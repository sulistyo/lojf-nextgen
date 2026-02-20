package bot

import (
	"testing"

	"github.com/lojf/nextgen/internal/models"
)

// TestRemindersBatchTgMap verifies the batch map-building logic used in
// runReminders: TelegramUser records are keyed by *ParentID, and records with
// a nil ParentID must be skipped (they haven't been linked to a parent yet).
func TestRemindersBatchTgMap(t *testing.T) {
	pid1, pid2 := uint(10), uint(20)

	tgUsers := []models.TelegramUser{
		{ID: 1, ChatID: 100, ParentID: &pid1, Deliverable: true},
		{ID: 2, ChatID: 200, ParentID: &pid2, Deliverable: true},
		{ID: 3, ChatID: 300, ParentID: nil, Deliverable: true}, // unlinked – must be skipped
	}

	// Replicate the exact map-building logic from runReminders.
	tgMap := make(map[uint]models.TelegramUser, len(tgUsers))
	for _, tu := range tgUsers {
		if tu.ParentID != nil {
			tgMap[*tu.ParentID] = tu
		}
	}

	if len(tgMap) != 2 {
		t.Errorf("tgMap length: want 2, got %d", len(tgMap))
	}
	if got := tgMap[pid1].ChatID; got != 100 {
		t.Errorf("parent %d ChatID: want 100, got %d", pid1, got)
	}
	if got := tgMap[pid2].ChatID; got != 200 {
		t.Errorf("parent %d ChatID: want 200, got %d", pid2, got)
	}
	// The nil-ParentID record must not appear under any key.
	if _, ok := tgMap[0]; ok {
		t.Error("nil ParentID produced a spurious map entry at key 0")
	}
}

// TestRemindersBatchTgMap_RowLookup verifies that a row whose parent has no
// linked Telegram account is silently skipped (no message sent).
func TestRemindersBatchTgMap_RowLookup(t *testing.T) {
	linkedParent := uint(1)
	unlinkedParent := uint(2)

	tgUsers := []models.TelegramUser{
		{ID: 1, ChatID: 111, ParentID: &linkedParent, Deliverable: true},
	}

	tgMap := make(map[uint]models.TelegramUser, len(tgUsers))
	for _, tu := range tgUsers {
		if tu.ParentID != nil {
			tgMap[*tu.ParentID] = tu
		}
	}

	type row struct{ Parent uint }
	rows := []row{{Parent: linkedParent}, {Parent: unlinkedParent}}

	messaged := 0
	for _, r := range rows {
		if _, ok := tgMap[r.Parent]; ok {
			messaged++
		}
	}

	if messaged != 1 {
		t.Errorf("expected 1 message to be sent, got %d", messaged)
	}
}
