package gotemplates

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
)

func WaitDB(db *sqlx.DB) {
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
