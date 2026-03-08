package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testICal = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//SPP//Events//EN
BEGIN:VEVENT
UID:ride-001@sparklingpinkpandas.com
DTSTART:20260215T160000Z
DTEND:20260215T190000Z
SUMMARY:Saturday Morning Ride
DESCRIPTION:Join us for a chill 30-mile ride through the hills.
LOCATION:Coffee Shop Parking Lot
URL:https://example.com/rides/saturday
END:VEVENT
BEGIN:VEVENT
UID:ride-002@sparklingpinkpandas.com
DTSTART:20260218T013000Z
DTEND:20260218T030000Z
SUMMARY:Tuesday Evening Spin
DESCRIPTION:Easy pace\, everyone welcome.\nBring lights!
LOCATION:City Park Entrance
END:VEVENT
END:VCALENDAR`

const testICalDateOnly = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:allday-001@example.com
DTSTART;VALUE=DATE:20260301
SUMMARY:All Day Event
DESCRIPTION:This event has no specific time.
END:VEVENT
END:VCALENDAR`

func TestParseICal(t *testing.T) {
	events, err := Parse([]byte(testICal))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	e := events[0]
	if e.Title != "Saturday Morning Ride" {
		t.Errorf("title = %q, want %q", e.Title, "Saturday Morning Ride")
	}
	if e.Link != "https://example.com/rides/saturday" {
		t.Errorf("link = %q, want %q", e.Link, "https://example.com/rides/saturday")
	}
	if e.GUID != "ride-001@sparklingpinkpandas.com" {
		t.Errorf("guid = %q, want %q", e.GUID, "ride-001@sparklingpinkpandas.com")
	}
	if e.Location != "Coffee Shop Parking Lot" {
		t.Errorf("location = %q, want %q", e.Location, "Coffee Shop Parking Lot")
	}
	if e.Date.IsZero() {
		t.Error("date should not be zero")
	}
	if e.Description != "Join us for a chill 30-mile ride through the hills." {
		t.Errorf("description = %q", e.Description)
	}
	expected := time.Date(2026, 2, 15, 16, 0, 0, 0, time.UTC)
	if !e.Date.Equal(expected) {
		t.Errorf("date = %v, want %v", e.Date, expected)
	}
	if e.DateEnd.IsZero() {
		t.Error("end date should not be zero")
	}
}

func TestParseICalEscaping(t *testing.T) {
	events, err := Parse([]byte(testICal))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	e := events[1]
	if e.Title != "Tuesday Evening Spin" {
		t.Errorf("title = %q, want %q", e.Title, "Tuesday Evening Spin")
	}
	// \, should become , and \n should become newline.
	want := "Easy pace, everyone welcome.\nBring lights!"
	if e.Description != want {
		t.Errorf("description = %q, want %q", e.Description, want)
	}
}

func TestParseICalDateOnly(t *testing.T) {
	events, err := Parse([]byte(testICalDateOnly))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Title != "All Day Event" {
		t.Errorf("title = %q", e.Title)
	}
	expected := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !e.Date.Equal(expected) {
		t.Errorf("date = %v, want %v", e.Date, expected)
	}
}

func TestParseEmptyCalendar(t *testing.T) {
	ical := "BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"
	events, err := Parse([]byte(ical))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestParseNoSummarySkipped(t *testing.T) {
	// Events without SUMMARY or UID should be skipped.
	ical := `BEGIN:VCALENDAR
BEGIN:VEVENT
DTSTART:20260101T000000Z
DESCRIPTION:No title event
END:VEVENT
END:VCALENDAR`

	events, err := Parse([]byte(ical))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events (no summary/uid), got %d", len(events))
	}
}

func TestParseICalDateFormats(t *testing.T) {
	tests := []struct {
		input string
		want  string // YYYY-MM-DD or "" for zero
	}{
		{"20260215T160000Z", "2026-02-15"},
		{"20260215T160000", "2026-02-15"},
		{"20260215", "2026-02-15"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		got := parseICalDate(tt.input, "")
		if tt.want == "" {
			if !got.IsZero() {
				t.Errorf("parseICalDate(%q) = %v, want zero", tt.input, got)
			}
			continue
		}
		gotStr := got.Format("2006-01-02")
		if gotStr != tt.want {
			t.Errorf("parseICalDate(%q) = %s, want %s", tt.input, gotStr, tt.want)
		}
	}
}

func TestUnescapeICal(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`hello\, world`, "hello, world"},
		{`line1\nline2`, "line1\nline2"},
		{`line1\Nline2`, "line1\nline2"},
		{`semi\;colon`, "semi;colon"},
		{`back\\slash`, "back\\slash"},
		{"no escapes", "no escapes"},
		{"", ""},
	}

	for _, tt := range tests {
		got := unescapeICal(tt.input)
		if got != tt.want {
			t.Errorf("unescapeICal(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUnfoldLines(t *testing.T) {
	// RFC 5545: lines starting with space/tab are continuations.
	input := "DESCRIPTION:This is a long\n description that wraps\n\ttwo times"
	got := unfoldLines(input)
	want := "DESCRIPTION:This is a longdescription that wrapstwo times"
	if got != want {
		t.Errorf("unfoldLines = %q, want %q", got, want)
	}
}

func TestFetchAndParse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(testICal))
	}))
	defer server.Close()

	events, err := FetchAndParse(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestFetchAndParseHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchAndParse(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestFoldedICal(t *testing.T) {
	// Test a real-world iCal with folded lines.
	// Per RFC 5545, the leading space/tab on continuation lines is the fold
	// marker and is stripped. Content continues immediately.
	// "SUMMARY:Folded\r\n Event" unfolds to "SUMMARY:FoldedEvent"
	ical := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fold-test@example.com\r\nDTSTART:20260301T100000Z\r\nSUMMARY:Folded \r\n Event Title\r\nDESCRIPTION:This description is \r\n also folded across \r\n multiple lines.\r\nEND:VEVENT\r\nEND:VCALENDAR"

	events, err := Parse([]byte(ical))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	// The trailing space before fold + continuation gives "Folded Event Title"
	if events[0].Title != "Folded Event Title" {
		t.Errorf("title = %q, want %q", events[0].Title, "Folded Event Title")
	}
	want := "This description is also folded across multiple lines."
	if events[0].Description != want {
		t.Errorf("description = %q, want %q", events[0].Description, want)
	}
}

func TestMultipleEvents(t *testing.T) {
	ical := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:a@test
SUMMARY:Event A
DTSTART:20260301T100000Z
END:VEVENT
BEGIN:VEVENT
UID:b@test
SUMMARY:Event B
DTSTART:20260302T100000Z
END:VEVENT
BEGIN:VEVENT
UID:c@test
SUMMARY:Event C
DTSTART:20260303T100000Z
END:VEVENT
END:VCALENDAR`

	events, err := Parse([]byte(ical))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify order is preserved.
	titles := []string{"Event A", "Event B", "Event C"}
	for i, want := range titles {
		if events[i].Title != want {
			t.Errorf("events[%d].Title = %q, want %q", i, events[i].Title, want)
		}
	}
}

func TestUnescapeICalBackslash(t *testing.T) {
	// Input "\\n" is literal backslash + n in iCal. It should unescape to
	// a single backslash followed by 'n', NOT a newline.
	got := unescapeICal(`\\n`)
	want := `\n`
	if got != want {
		t.Errorf("unescapeICal(%q) = %q, want %q", `\\n`, got, want)
	}

	// Input "\\," should become "\,"
	got2 := unescapeICal(`\\,`)
	want2 := `\,`
	if got2 != want2 {
		t.Errorf("unescapeICal(%q) = %q, want %q", `\\,`, got2, want2)
	}
}

func TestParseICalTZID(t *testing.T) {
	ical := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:tz-test@example.com
DTSTART;TZID=America/New_York:20260215T160000
DTEND;TZID=America/New_York:20260215T190000
SUMMARY:East Coast Ride
END:VEVENT
END:VCALENDAR`

	events, err := Parse([]byte(ical))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ev := events[0]
	// 4pm Eastern = 9pm UTC (EST is UTC-5)
	wantUTC := time.Date(2026, 2, 15, 21, 0, 0, 0, time.UTC)
	if !ev.Date.UTC().Equal(wantUTC) {
		t.Errorf("date = %v (UTC: %v), want %v UTC", ev.Date, ev.Date.UTC(), wantUTC)
	}
	// 7pm Eastern = midnight UTC
	wantEndUTC := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	if !ev.DateEnd.UTC().Equal(wantEndUTC) {
		t.Errorf("dateEnd = %v (UTC: %v), want %v UTC", ev.DateEnd, ev.DateEnd.UTC(), wantEndUTC)
	}
}
