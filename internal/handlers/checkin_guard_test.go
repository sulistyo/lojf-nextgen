package handlers

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lojf/nextgen/internal/models"
)

func init() {
	// Pin the signing key so token tests never touch the database.
	os.Setenv("SESSION_SECRET", "test-secret-do-not-use-in-production")
}

// jakartaMidnight builds a class date the way the app stores them.
func jakartaMidnight(offsetDays int) time.Time {
	n := time.Now().In(rosterLoc).AddDate(0, 0, offsetDays)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, rosterLoc)
}

func TestCampusOf(t *testing.T) {
	cases := map[string]string{
		"FJB Little Stars (Feast Jakarta Barat)": "FJB",
		"FJB - Stars Club (Feast Jakarta Barat)": "FJB", // the stray dashed variant
		"FJU Awesome Kids (Feast Jakarta Utara)": "FJU",
		"  FJU Stars Club":                       "FJU",
		"":                                       "",
	}
	for name, want := range cases {
		if got := CampusOf(name); got != want {
			t.Errorf("CampusOf(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestGuardCheckinRejectsEarlyCheckin replicates the 2026-08-07 incident:
// registrations 3123/3124 belonged to a class dated 2026-08-08 but were checked
// in the day before. Under the check-in role that must now fail.
func TestGuardCheckinRejectsEarlyCheckin(t *testing.T) {
	vol := &models.AdminUser{Role: models.RoleCheckin, Campus: "FJB"}
	tomorrow := models.Class{Name: "FJB Awesome Kids (Feast Jakarta Barat)", Date: jakartaMidnight(1)}

	if err := guardCheckin(vol, tomorrow); err != ErrCheckinNotToday {
		t.Fatalf("check-in a day early: got %v, want ErrCheckinNotToday", err)
	}
}

func TestGuardCheckinRejectsPastClass(t *testing.T) {
	vol := &models.AdminUser{Role: models.RoleCheckin, Campus: "FJB"}
	lastWeek := models.Class{Name: "FJB Awesome Kids (Feast Jakarta Barat)", Date: jakartaMidnight(-7)}

	if err := guardCheckin(vol, lastWeek); err != ErrCheckinNotToday {
		t.Fatalf("check-in for a past class: got %v, want ErrCheckinNotToday", err)
	}
}

func TestGuardCheckinRejectsOtherCampus(t *testing.T) {
	vol := &models.AdminUser{Role: models.RoleCheckin, Campus: "FJB"}
	other := models.Class{Name: "FJU Awesome Kids (Feast Jakarta Utara)", Date: jakartaMidnight(0)}

	if err := guardCheckin(vol, other); err != ErrCheckinWrongCampus {
		t.Fatalf("cross-campus check-in: got %v, want ErrCheckinWrongCampus", err)
	}
}

func TestGuardCheckinAllowsTodaySameCampus(t *testing.T) {
	vol := &models.AdminUser{Role: models.RoleCheckin, Campus: "FJB"}
	today := models.Class{Name: "FJB Little Stars (Feast Jakarta Barat)", Date: jakartaMidnight(0)}

	if err := guardCheckin(vol, today); err != nil {
		t.Fatalf("today at own campus should be allowed, got %v", err)
	}
}

// Admins keep the ability to fix records after the fact.
func TestGuardCheckinAdminBypassesBothFences(t *testing.T) {
	adm := &models.AdminUser{Role: models.RoleAdmin}
	future := models.Class{Name: "FJU Stars Club (Feast Jakarta Utara)", Date: jakartaMidnight(30)}

	if err := guardCheckin(adm, future); err != nil {
		t.Fatalf("admin should bypass the fences, got %v", err)
	}
}

// A check-in account with no campus set is allowed anywhere, but is still
// fenced to today.
func TestGuardCheckinEmptyCampusStillFencedToToday(t *testing.T) {
	vol := &models.AdminUser{Role: models.RoleCheckin, Campus: ""}

	today := models.Class{Name: "FJU Stars Club (Feast Jakarta Utara)", Date: jakartaMidnight(0)}
	if err := guardCheckin(vol, today); err != nil {
		t.Fatalf("empty campus should allow any campus today, got %v", err)
	}
	tomorrow := models.Class{Name: "FJU Stars Club (Feast Jakarta Utara)", Date: jakartaMidnight(1)}
	if err := guardCheckin(vol, tomorrow); err != ErrCheckinNotToday {
		t.Fatalf("empty campus must still be fenced to today, got %v", err)
	}
}

func TestSessionTokenRoundTrip(t *testing.T) {
	tok := signToken(42, time.Now().Add(time.Hour))
	id, ok := parseToken(tok)
	if !ok || id != 42 {
		t.Fatalf("round trip: got (%d, %v), want (42, true)", id, ok)
	}
}

func TestSessionTokenRejectsTamperedUserID(t *testing.T) {
	tok := signToken(42, time.Now().Add(time.Hour))
	parts := strings.SplitN(tok, ".", 2)
	forged := "9999." + parts[1] // swap the user id, keep the signature

	if _, ok := parseToken(forged); ok {
		t.Fatal("tampered token was accepted")
	}
}

func TestSessionTokenRejectsExpired(t *testing.T) {
	tok := signToken(42, time.Now().Add(-time.Minute))
	if _, ok := parseToken(tok); ok {
		t.Fatal("expired token was accepted")
	}
}

func TestSessionTokenRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "ok", "1.2", "1.2.3.4", "abc.def.ghi"} {
		if _, ok := parseToken(bad); ok {
			t.Fatalf("garbage token %q was accepted", bad)
		}
	}
}
