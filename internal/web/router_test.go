package web

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/lojf/nextgen/internal/db"
)

// TestMain changes the working directory to the project root before any tests
// run, so that template paths like "templates/layouts/*.tmpl" resolve correctly.
func TestMain(m *testing.M) {
	for {
		if _, err := os.Stat("go.mod"); err == nil {
			break
		}
		if err := os.Chdir(".."); err != nil {
			log.Fatalf("could not find project root: %v", err)
		}
	}
	os.Exit(m.Run())
}

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

// TestRouterTemplatePreload verifies that Router() pre-builds all templates at
// startup. template.Must (called in every factory function) panics on any
// missing or unparseable template file, which would immediately fail this test.
func TestRouterTemplatePreload(t *testing.T) {
	if err := db.Init(); err != nil {
		t.Fatalf("db init: %v", err)
	}
	// If any template file is absent or broken, Router() panics here.
	_ = Router()
}
