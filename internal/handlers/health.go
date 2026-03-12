package handlers

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
)

func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	version := "dev"
	revision := "unknown"
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				revision = s.Value[:min(7, len(s.Value))]
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"svc":      "lojf-nextgen",
		"version":  version,
		"revision": revision,
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
