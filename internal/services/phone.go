package services

import (
	"errors"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	"regexp"
	"strings"
	"unicode"
)

var (
	reLetters = regexp.MustCompile(`[A-Za-z]`)
	// Only allow digits, spaces, +, -, (, )
	reAllowed = regexp.MustCompile(`^[0-9+\-\s\(\)]+$`)
	// E.164-ish: + followed by 8..15 digits (no leading 0 after +)
	reE164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

// NormPhone normalizes phone numbers to +E.164-like formats used in the app.
// Rules: strip spaces/dashes/parens; 00.. -> +..; 62.. -> +62..; 0.. -> +62..; ensure leading +
func NormPhone(p string) string {
	s := strings.TrimSpace(p)

	if s == "" {
		return ""
	}
	if reLetters.MatchString(s) {
		return ""
	}
	if !reAllowed.MatchString(s) {
		return ""
	}

	// strip separators
	repl := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "\n", "", "\r", "")
	s = repl.Replace(s)

	// 00.. -> +..
	if strings.HasPrefix(s, "00") {
		s = "+" + s[2:]
	}
	// 62.. (no plus) -> +62..
	if strings.HasPrefix(s, "62") && !strings.HasPrefix(s, "+") {
		s = "+" + s
	}
	// 0.. (Indo local) -> +62..
	if strings.HasPrefix(s, "0") {
		s = "+62" + s[1:]
	}
	// ensure leading +
	if !strings.HasPrefix(s, "+") {
		s = "+" + s
	}
	return s
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func altPhones(p string) []string {
	out := []string{}
	n := NormPhone(p)
	raw := strings.TrimSpace(p)

	out = append(out, n)
	if raw != n && raw != "" {
		out = append(out, raw)
	}
	if strings.HasPrefix(n, "+62") && len(n) > 3 {
		out = append(out, "0"+n[3:])  // 0811...
		out = append(out, "62"+n[1:]) // 62811...
	}
	if strings.HasPrefix(n, "0") && len(n) > 1 {
		out = append(out, "+62"+n[1:]) // +62811...
	}
	if strings.HasPrefix(n, "+") && len(n) > 1 {
		out = append(out, n[1:]) // drop plus
	}
	return out
}

// FindParentByAny tries multiple normalized variants and a digits-only SQL compare.
func FindParentByAny(phone string) (*models.Parent, error) {
	var parent models.Parent

	// exact matches over variants
	for _, cand := range altPhones(phone) {
		if err := db.Conn().Where("phone = ?", cand).First(&parent).Error; err == nil {
			return &parent, nil
		}
	}

	// fallback: digits-only compare in SQL (ignore +, spaces, -, ())
	inDigits := digitsOnly(phone)
	if inDigits != "" {
		q := `
			REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(phone,'+',''),' ',''),'-',''),'(',''),')','')
		`
		if err := db.Conn().Where(q+" = ?", inDigits).First(&parent).Error; err == nil {
			return &parent, nil
		}
	}

	return nil, errors.New("parent not found")
}
