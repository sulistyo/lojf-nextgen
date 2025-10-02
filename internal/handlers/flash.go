// internal/handlers/flash.go
package handlers

import (
	"net/http"
	"strings"
)

type Flash struct {
	Kind string // "ok" or "error"
	Text string
}

var okText = map[string]string{
	"saved":         "Saved.",
	"child_saved":   "Child saved.",
	"child_deleted": "Child deleted.",
	"registered":    "Registration completed.",
	"checked_in":    "Checked in.",
	"canceled":      "Registration canceled.",
	"linked":        "Telegram linked.",
	"unlinked":      "Telegram has been unlinked.",
}

var errText = map[string]string{
	"missing":             "Name and phone are required.",
	"invalid_email":       "Invalid email address.",
	"email_in_use":        "That email is already used by another account.",
	"class_not_found":     "Class not found.",
	"no_upcoming_classes": "No upcoming classes.",
	"already_registered":  "This child is already registered for this class.",
	"same_day_conflict":   "This child is already registered for another class that day.",
	"invalid_code":        "Invalid or missing code.",
	"code_not_found":      "Code not found.",
	"invalid_checkin":     "Code is not eligible for check-in.",
	"already_checkedin":   "Already checked in.",
	"has_future":          "Cannot delete: parent has upcoming registrations. Cancel them first.",
}

// MakeFlash reads query params and/or explicit strings to build a Flash.
// Supports both new (?ok= / ?error=) and legacy (?msg= / ?err=) parameters.
func MakeFlash(r *http.Request, errStr, msgStr string) *Flash {
	q := r.URL.Query()

	// Accept both new and legacy keys
	errRaw := strings.TrimSpace(q.Get("error"))
	if errRaw == "" {
		errRaw = strings.TrimSpace(q.Get("err"))
	}
	okRaw := strings.TrimSpace(q.Get("ok"))
	if okRaw == "" {
		okRaw = strings.TrimSpace(q.Get("msg"))
	}

	if errRaw != "" {
		key := strings.ToLower(errRaw)
		if t, ok := errText[key]; ok {
			return &Flash{Kind: "error", Text: t}
		}
		return &Flash{Kind: "error", Text: errRaw}
	}
	if okRaw != "" {
		key := strings.ToLower(okRaw)
		if t, ok := okText[key]; ok {
			return &Flash{Kind: "ok", Text: t}
		}
		return &Flash{Kind: "ok", Text: okRaw}
	}

	// Fallback to handler-provided messages
	if errStr != "" {
		return &Flash{Kind: "error", Text: errStr}
	}
	if msgStr != "" {
		return &Flash{Kind: "ok", Text: msgStr}
	}
	return nil
}
