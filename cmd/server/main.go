package main

import (
	"log"
	"net/http"
	"os"

	"github.com/lojf/nextgen/internal/bot"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/web"
)

func main() {
	// Init DB (creates nextgen.db in working dir)
	if err := db.Init(); err != nil {
		log.Fatalf("db init: %v", err)
	}
	bot.StartReminderLoop()

	r := web.Router()

	addr := getEnv("ADDR", ":8080")
	log.Printf("LOJF NextGen listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
