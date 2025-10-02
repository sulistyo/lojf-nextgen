package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/lojf/nextgen/internal/bot"
)

func TelegramWebhook(w http.ResponseWriter, r *http.Request) {
	// Simple secret check: /tg/webhook?secret=...
	if r.URL.Query().Get("secret") != os.Getenv("TG_WEBHOOK_SECRET") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)

	var up bot.Update
	if err := json.Unmarshal(b, &up); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	bot.NewDispatcher().Handle(&up)
	w.WriteHeader(200)
	w.Write([]byte("ok"))
}
