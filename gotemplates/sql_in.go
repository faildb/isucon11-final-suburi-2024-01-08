package gotemplates

import (
	"context"
	"github.com/jmoiron/sqlx"
)

func IN(ctx context.Context, db *sqlx.DB) {
	var levels = []int{4, 6, 7}
	query, args, err := sqlx.In("SELECT * FROM users WHERE level IN (?);", levels)

	query = db.Rebind(query)
	rows, err := db.SelectContext(ctx, query, args...)
}

func NamedIN(ctx context.Context, db *sqlx.DB) {
	arg := map[string]interface{}{
		"published": true,
		"authors":   []int{8, 19, 32, 44},
	}
	query, args, err := sqlx.Named("SELECT * FROM articles WHERE published=:published AND author_id IN (:authors)", arg)
	query, args, err := sqlx.In(query, args...)
	query = db.Rebind(query)
	db.SelectContext(ctx, query, args...)

}
