package handlers

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
)

// BuildVersion is injected at build time via -ldflags "-X ...handlers.BuildVersion=<hash>"
var BuildVersion = "dev"

func resolvedVersion() string {
	if BuildVersion != "" && BuildVersion != "dev" {
		return BuildVersion
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				if len(s.Value) >= 7 {
					return s.Value[:7]
				}
				return s.Value
			}
		}
	}
	return BuildVersion
}

func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"svc":     "lojf-nextgen",
		"version": resolvedVersion(),
	})
}

func Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := map[string]any{
		"service":  "lojf-nextgen",
		"version":  resolvedVersion(),
		"revision": "unknown",
		"dirty":    false,
		"builtAt":  "unknown",
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
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
