package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/model"
)

type Option struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type StepFile struct {
	ID          string   `json:"id"`
	Order       int      `json:"order"`
	Type        string   `json:"type"`
	Content     string   `json:"content"`
	Options     []Option `json:"options"`
	AnswerKey   string   `json:"answer_key"`
	Explanation string   `json:"explanation"`
}

type ModuleFile struct {
	ID                      string     `json:"id"`
	VersionID               string     `json:"version_id"`
	Key                     string     `json:"key"`
	Version                 int        `json:"version"`
	Order                   int        `json:"order"`
	Title                   string     `json:"title"`
	SkillIDs                []string   `json:"skill_ids"`
	AssessmentCaseVersionID string     `json:"assessment_case_version_id"`
	Steps                   []StepFile `json:"steps"`
	Hash                    string     `json:"-"`
}

type CaseFile struct {
	ID            string          `json:"id"`
	VersionID     string          `json:"version_id"`
	Key           string          `json:"key"`
	Version       int             `json:"version"`
	Mode          string          `json:"mode"`
	Title         string          `json:"title"`
	Difficulty    string          `json:"difficulty"`
	SkillIDs      []string        `json:"skill_ids"`
	Briefing      string          `json:"briefing"`
	PrivateConfig json.RawMessage `json:"private_config"`
	MaxTurns      int             `json:"max_turns"`
	Rubric        json.RawMessage `json:"-"`
	Hash          string          `json:"-"`
}

type RubricFile struct {
	CaseKey  string `json:"case_key"`
	Version  int    `json:"version"`
	Criteria []struct {
		ID             string `json:"id"`
		Required       bool   `json:"required"`
		Behavior       string `json:"behavior"`
		SuccessExample string `json:"success_example"`
		FailureExample string `json:"failure_example"`
		Evidence       string `json:"evidence"`
	} `json:"criteria"`
}

type Data struct {
	Modules []ModuleFile
	Cases   []CaseFile
}

func Load(dir string) (Data, error) {
	var data Data

	caseFiles, err := filepath.Glob(filepath.Join(dir, "cases", "*.json"))
	if err != nil || len(caseFiles) == 0 {
		return data, fmt.Errorf("case files: %w", errors.Join(err, errors.New("no case files")))
	}

	for _, path := range caseFiles {
		var item CaseFile

		raw, err := readJSON(path, &item)
		if err != nil {
			return data, err
		}

		var rubric RubricFile

		rubricRaw, err := readJSON(filepath.Join(dir, "rubrics", item.Key+".json"), &rubric)
		if err != nil {
			return data, err
		}

		if rubric.CaseKey != item.Key || rubric.Version != item.Version || len(rubric.Criteria) != 3 {
			return data, fmt.Errorf("invalid rubric for %s", item.Key)
		}

		seen := make(map[string]bool)

		for _, criterion := range rubric.Criteria {
			if criterion.ID == "" || seen[criterion.ID] || criterion.Behavior == "" || criterion.Evidence == "" || criterion.SuccessExample == "" || criterion.FailureExample == "" {
				return data, fmt.Errorf("invalid criterion in %s", item.Key)
			}

			seen[criterion.ID] = true
		}

		if item.ID == "" || item.VersionID == "" || item.Key == "" || item.Version < 1 || item.Title == "" || item.Briefing == "" || item.MaxTurns < 1 || len(item.SkillIDs) == 0 || !json.Valid(item.PrivateConfig) || (item.Mode != "assessment" && item.Mode != "practice") {
			return data, fmt.Errorf("invalid case %s", path)
		}

		item.Rubric = rubricRaw
		item.Hash = hash(append(raw, rubricRaw...))
		data.Cases = append(data.Cases, item)
	}

	moduleFiles, err := filepath.Glob(filepath.Join(dir, "modules", "*.json"))
	if err != nil || len(moduleFiles) == 0 {
		return data, fmt.Errorf("module files: %w", errors.Join(err, errors.New("no module files")))
	}

	for _, path := range moduleFiles {
		var item ModuleFile

		raw, err := readJSON(path, &item)
		if err != nil {
			return data, err
		}

		if item.ID == "" || item.VersionID == "" || item.Key == "" || item.Version < 1 || item.Order < 1 || item.Title == "" || len(item.SkillIDs) == 0 || len(item.Steps) == 0 {
			return data, fmt.Errorf("invalid module %s", path)
		}

		foundCase := false

		for _, c := range data.Cases {
			if c.VersionID == item.AssessmentCaseVersionID && c.Mode == "assessment" {
				foundCase = true
			}
		}

		if !foundCase {
			return data, fmt.Errorf("module %s references missing assessment case", item.Key)
		}

		for i, step := range item.Steps {
			if step.ID == "" || step.Order != i+1 || step.Content == "" || step.Explanation == "" {
				return data, fmt.Errorf("invalid step in %s", item.Key)
			}

			switch step.Type {
			case "theory":
				words := len(strings.Fields(step.Content))
				if step.AnswerKey != "confirmed" || len(step.Options) != 0 || words < 60 || words > 120 {
					return data, fmt.Errorf("invalid theory step %s", step.ID)
				}

			case "choice":
				if len(step.Options) < 2 || !slices.ContainsFunc(step.Options, func(o Option) bool { return o.ID == step.AnswerKey }) {
					return data, fmt.Errorf("invalid choice step %s", step.ID)
				}

			default:
				return data, fmt.Errorf("unknown step type %s", step.Type)
			}
		}

		item.Hash = hash(raw)
		data.Modules = append(data.Modules, item)
	}

	slices.SortFunc(data.Modules, func(a, b ModuleFile) int { return a.Order - b.Order })

	for i, item := range data.Modules {
		if item.Order != i+1 {
			return data, fmt.Errorf("module order must be contiguous")
		}
	}

	return data, nil
}

func readJSON(path string, target any) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	return raw, nil
}

func hash(raw []byte) string {
	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:])
}

func Seed(ctx context.Context, db *gorm.DB, data Data) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range data.Cases {
			base := model.Case{ID: item.ID, Key: item.Key}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&base).Error; err != nil {
				return fmt.Errorf("seed case %s: %w", item.Key, err)
			}

			skills, _ := json.Marshal(item.SkillIDs)
			version := model.CaseVersion{ID: item.VersionID, CaseID: item.ID, Version: item.Version, Mode: item.Mode, Title: item.Title, Difficulty: item.Difficulty, SkillIDs: skills, Briefing: item.Briefing, PrivateConfig: item.PrivateConfig, Rubric: item.Rubric, MaxTurns: item.MaxTurns, ContentHash: item.Hash, PublishedAt: time.Now().UTC()}
			if err := insertVersion(tx, &version, item.VersionID, item.Hash); err != nil {
				return fmt.Errorf("seed case version %s: %w", item.Key, err)
			}
		}

		for _, item := range data.Modules {
			base := model.Module{ID: item.ID, Key: item.Key}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&base).Error; err != nil {
				return fmt.Errorf("seed module %s: %w", item.Key, err)
			}

			skills, _ := json.Marshal(item.SkillIDs)
			version := model.ModuleVersion{ID: item.VersionID, ModuleID: item.ID, Version: item.Version, OrderIndex: item.Order, Title: item.Title, SkillIDs: skills, AssessmentCaseVersionID: item.AssessmentCaseVersionID, ContentHash: item.Hash, PublishedAt: time.Now().UTC()}
			if err := insertVersion(tx, &version, item.VersionID, item.Hash); err != nil {
				return fmt.Errorf("seed module version %s: %w", item.Key, err)
			}

			for _, step := range item.Steps {
				options, _ := json.Marshal(step.Options)
				row := model.Step{ID: step.ID, ModuleVersionID: item.VersionID, OrderIndex: step.Order, Type: step.Type, Content: step.Content, Options: options, AnswerKey: step.AnswerKey, Explanation: step.Explanation}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return fmt.Errorf("seed step %s: %w", step.ID, err)
				}
			}
		}

		return nil
	})
}

func insertVersion(tx *gorm.DB, row any, id, expectedHash string) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
		return err
	}

	var existing struct{ ContentHash string }

	if err := tx.Model(row).Select("content_hash").Where("id = ?", id).Take(&existing).Error; err != nil {
		return err
	}

	if existing.ContentHash != expectedHash {
		return fmt.Errorf("published version %s changed; create a new version", id)
	}

	return nil
}
