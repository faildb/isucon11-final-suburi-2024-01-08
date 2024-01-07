package gotemplates

import (
	"github.com/jackc/pgconn"
)

func pgxIsDuplicateError(err error) bool {
	if perr, ok := err.(*pgconn.PgError); ok && perr.Code == "23505" { // duplicate entry
		return true
	}

	return false
}
