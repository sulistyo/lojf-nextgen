package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lojf/nextgen/internal/db"
)

func TestRouterHealthz(t *testing.T) {
	if err := db.Init(); err != nil {
		t.Fatalf("db init: %v", err)
	}
	r := Router()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
