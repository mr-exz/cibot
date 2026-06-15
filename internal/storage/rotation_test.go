package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := New(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, ctx
}

// addPerson creates a support person with no configured work days/hours, so the
// category is staffed every day and walk-forward returns the base slot person.
func addPerson(t *testing.T, db *DB, ctx context.Context, name string) int64 {
	t.Helper()
	id, err := db.AddSupportPersonFull(ctx, name, "@"+name, name, "", "", "")
	if err != nil {
		t.Fatalf("AddSupportPersonFull(%s): %v", name, err)
	}
	return id
}

func setupCategory(t *testing.T, db *DB, ctx context.Context, rotationType string, startDate string, names ...string) (int64, map[string]int64) {
	t.Helper()
	catID, err := db.AddCategoryWithTopic(ctx, "cat", "🛟", "TEAM", nil, nil)
	if err != nil {
		t.Fatalf("AddCategoryWithTopic: %v", err)
	}
	ids := map[string]int64{}
	for _, n := range names {
		pid := addPerson(t, db, ctx, n)
		ids[n] = pid
		if err := db.CreateInitialAssignment(ctx, catID, pid, rotationType, startDate); err != nil {
			t.Fatalf("CreateInitialAssignment(%s): %v", n, err)
		}
	}
	return catID, ids
}

func onDuty(t *testing.T, db *DB, ctx context.Context, catID int64, date time.Time) int64 {
	t.Helper()
	p, err := db.GetOnDutyPerson(ctx, catID, date)
	if err != nil {
		t.Fatalf("GetOnDutyPerson(%s): %v", dateKey(date), err)
	}
	return p.ID
}

func TestWeeklyRotationAdvancesAndFreezes(t *testing.T) {
	db, ctx := newTestDB(t)
	mon := mondayOf(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	catID, ids := setupCategory(t, db, ctx, "weekly", dateKey(mon), "alice", "bob", "carol")

	// Drive generation from week 0; current week + next week get materialized.
	if got := onDuty(t, db, ctx, catID, mon); got != ids["alice"] {
		t.Fatalf("week0: got %d, want alice(%d)", got, ids["alice"])
	}
	if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 7)); got != ids["bob"] {
		t.Fatalf("week1: got %d, want bob(%d)", got, ids["bob"])
	}

	// Sunday sweep one week later extends the schedule with the next turn.
	if err := db.GenerateAllRotations(ctx, mon.AddDate(0, 0, 7)); err != nil {
		t.Fatalf("GenerateAllRotations: %v", err)
	}
	if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 14)); got != ids["carol"] {
		t.Fatalf("week2: got %d, want carol(%d)", got, ids["carol"])
	}
	// Wraps back to alice on week 3.
	if err := db.GenerateAllRotations(ctx, mon.AddDate(0, 0, 14)); err != nil {
		t.Fatalf("GenerateAllRotations: %v", err)
	}
	if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 21)); got != ids["alice"] {
		t.Fatalf("week3: got %d, want alice(%d)", got, ids["alice"])
	}

	// Adding a person must NOT disturb already-decided weeks.
	dave := addPerson(t, db, ctx, "dave")
	if err := db.CreateInitialAssignment(ctx, catID, dave, "weekly", dateKey(mon)); err != nil {
		t.Fatalf("CreateInitialAssignment(dave): %v", err)
	}
	if err := db.EnsureRotationGenerated(ctx, catID, mon.AddDate(0, 0, 14)); err != nil {
		t.Fatalf("EnsureRotationGenerated: %v", err)
	}
	for week, want := range map[int]int64{0: ids["alice"], 1: ids["bob"], 2: ids["carol"], 3: ids["alice"]} {
		if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 7*week)); got != want {
			t.Fatalf("after add, week%d: got %d, want %d", week, got, want)
		}
	}
	// Dave joins the end of the cycle (highest id): the rotation continues
	// a→b→c→a→b→c→d, so weeks 4/5/6 are bob/carol/dave. Decided weeks 0-3 above
	// were not disturbed by the add.
	for _, sweep := range []int{21, 28, 35} {
		if err := db.GenerateAllRotations(ctx, mon.AddDate(0, 0, sweep)); err != nil {
			t.Fatalf("GenerateAllRotations: %v", err)
		}
	}
	for week, want := range map[int]int64{4: ids["bob"], 5: ids["carol"], 6: dave} {
		if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 7*week)); got != want {
			t.Fatalf("week%d: got %d, want %d", week, got, want)
		}
	}
}

func TestRemoveOnDutyPersonReassignsAndKeepsHistory(t *testing.T) {
	db, ctx := newTestDB(t)
	mon := mondayOf(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	catID, ids := setupCategory(t, db, ctx, "weekly", dateKey(mon), "alice", "bob", "carol")

	// Materialize from a mid-week point (Wednesday of week 0); alice is on duty.
	wed := mon.AddDate(0, 0, 2)
	if got := onDuty(t, db, ctx, catID, wed); got != ids["alice"] {
		t.Fatalf("wed: got %d, want alice(%d)", got, ids["alice"])
	}

	// Remove alice mid-week.
	if err := db.reassignAfterRemoval(ctx, catID, ids["alice"], wed); err != nil {
		t.Fatalf("reassignAfterRemoval: %v", err)
	}

	// Past days of the week stay as history (alice).
	if got, err := db.GetScheduledPerson(ctx, catID, mon); err != nil || got == nil || got.ID != ids["alice"] {
		t.Fatalf("history mon: got %v err %v, want alice(%d)", got, err, ids["alice"])
	}
	if got, err := db.GetScheduledPerson(ctx, catID, mon.AddDate(0, 0, 1)); err != nil || got == nil || got.ID != ids["alice"] {
		t.Fatalf("history tue: got %v err %v, want alice(%d)", got, err, ids["alice"])
	}
	// From the removal day forward, the next person in line (bob) is on duty.
	if got := onDuty(t, db, ctx, catID, wed); got != ids["bob"] {
		t.Fatalf("wed after removal: got %d, want bob(%d)", got, ids["bob"])
	}
	// Next week continues the rotation without alice (carol).
	if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, 7)); got != ids["carol"] {
		t.Fatalf("week1 after removal: got %d, want carol(%d)", got, ids["carol"])
	}
}

func TestDailyRotation(t *testing.T) {
	db, ctx := newTestDB(t)
	mon := mondayOf(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	catID, ids := setupCategory(t, db, ctx, "daily", dateKey(mon), "alice", "bob")

	want := []int64{ids["alice"], ids["bob"], ids["alice"], ids["bob"], ids["alice"]}
	for i, w := range want {
		if got := onDuty(t, db, ctx, catID, mon.AddDate(0, 0, i)); got != w {
			t.Fatalf("daily day%d: got %d, want %d", i, got, w)
		}
	}
}
