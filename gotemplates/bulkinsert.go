package gotemplates

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"strings"
)

type Event struct {
	At    int64
	Name  string
	Value int64
}

func BulkInsert(ctx context.Context, db *sqlx.DB, buf []Event) error {
	query := "INSERT INTO eventlog(at, name, value) VALUES"
	values := []interface{}{}

	placeholders := make([]string, 0, len(buf))
	for _, event := range buf {
		placeholders = append(placeholders, "(?, ?, ?)")
		values = append(values, event.At, event.Name, event.Value)
	}
	query += strings.Join(placeholders, ",")

	_, err := db.ExecContext(ctx, query, values...)
	if err !=nil {
		return fmt.Errorf("failed to exec builk insert: %w", err)
	}

	return nil
}
