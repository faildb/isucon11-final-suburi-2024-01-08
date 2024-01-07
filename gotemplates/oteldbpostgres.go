package gotemplates

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/jmoiron/sqlx"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	_ "github.com/mackee/pgx-replaced"
)

func GetDBNoOtel() (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%v/%s?sslmode=disable",
		GetEnv("DB_USER", "isucon"),
		GetEnv("DB_PASS", "isucon"),
		GetEnv("DB_HOSTNAME", "127.0.0.1"),
		GetEnv("DB_PORT", "5432"),
		GetEnv("DB_DATABASE", "isucon"),
	)

	tmpDB, err := sql.Open(
		"pgx-replaced",
		dsn,
	)
	if err != nil {
		return nil, err
	}

	WaitDB(tmpDB)

	tmpDB.SetMaxOpenConns(50)
	tmpDB.SetConnMaxLifetime(5 * time.Minute)

	return sqlx.NewDb(tmpDB, "pgx"), nil
}

func GetDB() (*sqlx.DB, error) {
	if GetEnv("OTEL_SDK_DISABLED", "false") == "true" {
		return GetDBNoOtel()
	}

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%v/%s?sslmode=disable",
		GetEnv("DB_USER", "isucon"),
		GetEnv("DB_PASS", "isucon"),
		GetEnv("DB_HOSTNAME", "127.0.0.1"),
		GetEnv("DB_PORT", "5432"),
		GetEnv("DB_DATABASE", "isucon"),
	)

	tmpDB, err := otelsql.Open(
		"pgx-replaced",
		dsn,
		otelsql.WithAttributes(
			semconv.DBSystemPostgreSQL,
		),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			Ping:                 false,
			RowsNext:             false,
			DisableErrSkip:       false,
			DisableQuery:         false,
			OmitConnResetSession: true,
			OmitConnPrepare:      true,
			OmitConnQuery:        false,
			OmitRows:             true,
			OmitConnectorConnect: false,
		}),
	)
	if err != nil {
		return nil, err
	}

	WaitDB(tmpDB)

	tmpDB.SetMaxOpenConns(50)
	tmpDB.SetConnMaxLifetime(5 * time.Minute)

	return sqlx.NewDb(tmpDB, "pgx"), nil
}

func WaitDB(db *sql.DB) {
	for {
		err := db.Ping()
		if err == nil {
			break
		}
		log.Println(fmt.Errorf("failed to ping DB on start up. retrying...: %w", err))
		time.Sleep(time.Second * 1)
	}
	log.Println("Succeeded to connect db!")
}
