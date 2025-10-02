package services

import (
	"net/mail"
	"strings"
)

func NormEmail(s string) (string, bool) {
	e := strings.TrimSpace(strings.ToLower(s))
	if e == "" {
		return "", true // treat empty as ok/optional
	}
	_, err := mail.ParseAddress(e)
	return e, err == nil
}
