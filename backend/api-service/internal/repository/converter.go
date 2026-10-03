package repository

import (
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
)

func StringToUUID(userID string) (pgtype.UUID, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("get uuid: %w", err)
	}

	return pgtype.UUID{
		Bytes: parsedUserID,
		Valid: true,
	}, nil
}
