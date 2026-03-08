package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartOfDay(t *testing.T) {
	ts := time.Date(2026, 3, 15, 14, 30, 45, 0, time.UTC)
	got := startOfDay(ts)
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("startOfDay(%v) = %v, want %v", ts, got, want)
	}
}

func TestLoadStateEmpty(t *testing.T) {
	dir := t.TempDir()
	state := loadState(dir)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Events) != 0 {
		t.Errorf("expected empty events map, got %d entries", len(state.Events))
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()

	state := newEventState()
	state.Events["test-uid-1"] = &EventRecord{
		Title:     "Test Event",
		Date:      time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Published: true,
		Reminded:  false,
	}
	state.Events["test-uid-2"] = &EventRecord{
		Title:     "Failed Event",
		Date:      time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC),
		Published: false,
		Reminded:  false,
	}

	saveState(dir, state)

	loaded := loadState(dir)
	if len(loaded.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(loaded.Events))
	}

	rec1 := loaded.Events["test-uid-1"]
	if rec1 == nil {
		t.Fatal("missing test-uid-1")
	}
	if !rec1.Published {
		t.Error("test-uid-1 should be published")
	}
	if rec1.Reminded {
		t.Error("test-uid-1 should not be reminded")
	}

	rec2 := loaded.Events["test-uid-2"]
	if rec2 == nil {
		t.Fatal("missing test-uid-2")
	}
	if rec2.Published {
		t.Error("test-uid-2 should not be published")
	}
}

func TestLoadStateCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, stateFile)
	if err := os.WriteFile(path, []byte("not valid json{"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := loadState(dir)
	if state == nil {
		t.Fatal("expected non-nil state on corrupt file")
	}
	if len(state.Events) != 0 {
		t.Errorf("expected empty events on corrupt file, got %d", len(state.Events))
	}
}

func TestStateFileFormat(t *testing.T) {
	dir := t.TempDir()

	state := newEventState()
	state.Events["uid-1"] = &EventRecord{
		Title:     "Event",
		Date:      time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Published: true,
		Reminded:  true,
	}

	saveState(dir, state)

	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}
	if _, ok := raw["events"]; !ok {
		t.Error("state file missing 'events' key")
	}
}

func TestNewEventState(t *testing.T) {
	state := newEventState()
	if state.Events == nil {
		t.Error("newEventState should have non-nil Events map")
	}
}
