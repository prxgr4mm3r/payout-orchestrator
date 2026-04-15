package pgtypeutil

import "github.com/jackc/pgx/v5/pgtype"

func ParseUUID(raw string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}
