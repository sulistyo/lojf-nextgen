package handlers

import (
	"regexp"
	"testing"
)

var codeRE = regexp.MustCompile(`^REG-[0-9A-F]{8}$`)

// TestGenerateRegCode_Format verifies that generated codes match the expected
// REG-XXXXXXXX format (uppercase hex, exactly 8 digits).
func TestGenerateRegCode_Format(t *testing.T) {
	code := generateRegCode()
	if code == "" {
		t.Fatal("generateRegCode returned empty string")
	}
	if !codeRE.MatchString(code) {
		t.Errorf("code %q does not match REG-[0-9A-F]{8}", code)
	}
}

// TestGenerateRegCode_Unique generates 2000 codes and checks for collisions.
// With 32 bits of entropy the collision probability over 2000 draws is ~0.05%,
// so this would only flake in astronomically unlikely circumstances.
func TestGenerateRegCode_Unique(t *testing.T) {
	const n = 2000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		c := generateRegCode()
		if c == "" {
			t.Fatalf("generateRegCode returned empty string on iteration %d", i)
		}
		if _, dup := seen[c]; dup {
			t.Fatalf("duplicate code %q generated on iteration %d", c, i)
		}
		seen[c] = struct{}{}
	}
}
