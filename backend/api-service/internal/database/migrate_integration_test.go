package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRefreshTokenMigrationUpgradesExistingSchema(t *testing.T) {
	baseDSN := os.Getenv("BE3_TEST_DSN")
	if baseDSN == "" {
		t.Skip("set BE3_TEST_DSN to a disposable PostgreSQL database")
	}

	adminDB, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	schema := fmt.Sprintf("be3_upgrade_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatal(err)
	}

	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`CREATE TABLE schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`).Error; err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"001_initial.sql", "002_published_content_guards.sql"} {
		sql, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}

		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}

		if err := db.Exec("INSERT INTO schema_migrations(version) VALUES (?)", name).Error; err != nil {
			t.Fatal(err)
		}
	}

	if db.Migrator().HasTable("refresh_tokens") {
		t.Fatal("refresh_tokens unexpectedly exists before upgrade")
	}

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	if !db.Migrator().HasTable("refresh_tokens") {
		t.Fatal("refresh_tokens was not created by the upgrade")
	}

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
}
