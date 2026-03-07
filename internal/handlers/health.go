package handlers

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
)

func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"svc":  "lojf-nextgen",
		"note": "healthy",
	})
}

func Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := map[string]any{
		"service":  "lojf-nextgen",
		"version":  "dev",
		"revision": "unknown",
		"dirty":    false,
		"builtAt":  "unknown",
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		resp["version"] = bi.Main.Version
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if s.Value != "" {
					resp["revision"] = s.Value
				}
			case "vcs.time":
				if s.Value != "" {
					resp["builtAt"] = s.Value
				}
			case "vcs.modified":
				resp["dirty"] = (s.Value == "true")
			}
		}
	}

	_ = json.NewEncoder(w).Encode(resp)
}
