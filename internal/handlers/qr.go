package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func QR(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}
	// ensure code exists
	var reg models.Registration
	if err := db.Conn().Where("code = ?", code).First(&reg).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// Encode a URL so scanning opens check-in directly
	url := "http://" + r.Host + "/checkin?code=" + code

	png, err := qrcode.Encode(url, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "failed to generate qr", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}