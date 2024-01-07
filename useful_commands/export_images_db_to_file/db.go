package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"

	// mysql
	_ "github.com/go-sql-driver/mysql"
	// postgres
	//_ "github.com/mackee/pgx-replaced"
)

func GetEnv(key, val string) string {
	if v := os.Getenv(key); v == "" {
		return val
	} else {
		return v
	}
}

func GetDBMysql() (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%v)/%s?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true",
		GetEnv("DB_USER", "isucon"),
		GetEnv("DB_PASS", "isucon"),
		GetEnv("DB_HOSTNAME", "127.0.0.1"),
		GetEnv("DB_PORT", "3306"),
		GetEnv("DB_DATABASE", "isucon"),
	)
	fmt.Printf("dsn: %s\n", dsn)

	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	WaitDB(db.DB)
	return db, err
}

func GetDBPostgres() (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%v/%s?sslmode=disable",
		GetEnv("DB_USER", "isucon"),
		GetEnv("DB_PASS", "isucon"),
		GetEnv("DB_HOSTNAME", "127.0.0.1"),
		GetEnv("DB_PORT", "5432"),
		GetEnv("DB_DATABASE", "isucon"),
	)
	fmt.Printf("dsn: %s\n", dsn)

	db, err := sqlx.Open("pgx-replaced", dsn)
	if err != nil {
		return nil, err
	}
	WaitDB(db.DB)
	return db, err
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
