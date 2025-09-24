package handlers

import (
	"strings"
	"unicode"
	"fmt"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// normPhone produces a permissive E.164-like number:
// - trims spaces
// - removes spaces, hyphens, parentheses
// - "00" prefix -> "+" (international)
// - "+<digits>" kept as-is
// - "0xxxxxxxxx" -> assume Indonesia local -> "+62" + rest
// - "<digits>" with no + or leading 0 -> prefix "+" (assume already CC)
func normPhone(p string) string {
	p = strings.TrimSpace(p)
	// strip common separators
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	p = replacer.Replace(p)
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "00") {
		return "+" + p[2:]
	}
	if strings.HasPrefix(p, "+") {
		return "+" + strings.TrimPrefix(p, "+")
	}
	if strings.HasPrefix(p, "0") {
		// Indonesia local to international
		return "+62" + p[1:]
	}
	// bare digits: treat as already including CC, add '+'
	return "+" + p
}

// strip all non-digits (for fuzzy match)
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// alternative forms we can try (handles +62… / 0… / bare 62… / drop '+')
func altPhones(p string) []string {
	var out []string
	n := normPhone(p)           // e.g. +62811…
	raw := strings.TrimSpace(p) // whatever came in

	out = append(out, n)
	if raw != n {
		out = append(out, raw)
	}
	if strings.HasPrefix(n, "+62") && len(n) > 3 {
		out = append(out, "0"+n[3:])  // 0811…
		out = append(out, "62"+n[1:]) // 62811…
	}
	if strings.HasPrefix(n, "0") && len(n) > 1 {
		out = append(out, "+62"+n[1:]) // +62811…
	}
	if strings.HasPrefix(n, "+") && len(n) > 1 {
		out = append(out, n[1:]) // drop plus: 62811…
	}
	return out
}

// Try multiple strategies to find a parent by phone.
func findParentByAny(phone string) (*models.Parent, error) {
	candidates := altPhones(phone)
	var parent models.Parent

	// 1) Try exact matches over candidates
	for _, cand := range candidates {
		if err := db.Conn().Where("phone = ?", cand).First(&parent).Error; err == nil {
			return &parent, nil
		}
	}

	// 2) Fallback: digits-only compare in SQL (ignores +, spaces, -, ())
	inDigits := digitsOnly(phone)
	if inDigits != "" {
		// REPLACE(REPLACE...) to normalize stored phone on the fly
		q := `
			REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(phone,'+',''),' ',''),'-',''),'(',''),')','')
		`
		if err := db.Conn().
			Where(q+" = ?", inDigits).
			First(&parent).Error; err == nil {
			return &parent, nil
		}
	}

	return nil, fmt.Errorf("parent not found for phone")
}