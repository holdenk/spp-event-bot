package matrix

import (
	"strings"
	"testing"
	"time"

	"github.com/sparklingpinkpandas/spp-event-bot/feed"
)

func TestFormatReminder(t *testing.T) {
	ev := feed.Event{
		Title:    "Saturday Ride",
		Link:     "https://example.com/ride",
		Date:     time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		DateEnd:  time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		Location: "Coffee Shop",
	}

	plain, html := formatReminder(ev)

	if !strings.Contains(plain, "Reminder: Today!") {
		t.Error("plain text should contain 'Reminder: Today!'")
	}
	if !strings.Contains(plain, "Saturday Ride") {
		t.Error("plain text should contain event title")
	}
	if !strings.Contains(plain, "10:00 AM - 1:00 PM") {
		t.Errorf("plain text should contain time range, got: %s", plain)
	}
	if !strings.Contains(plain, "Coffee Shop") {
		t.Error("plain text should contain location")
	}
	if !strings.Contains(html, "Reminder: Today!") {
		t.Error("html should contain 'Reminder: Today!'")
	}
	if !strings.Contains(html, "https://example.com/ride") {
		t.Error("html should contain event link")
	}
}

func TestFormatReminderNoLink(t *testing.T) {
	ev := feed.Event{
		Title: "Simple Event",
		Date:  time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
	}

	plain, html := formatReminder(ev)

	if !strings.Contains(plain, "Simple Event") {
		t.Error("plain text should contain title")
	}
	if !strings.Contains(html, "<b>Simple Event</b>") {
		t.Error("html should have bold title without link")
	}
}

func TestFormatEventOutput(t *testing.T) {
	ev := feed.Event{
		Title:       "Test Event",
		Link:        "https://example.com",
		Date:        time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		DateEnd:     time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		Location:    "Park",
		Description: "A fun event",
	}

	plain, html := formatEvent(ev)

	if !strings.Contains(plain, "Test Event") {
		t.Error("plain should contain title")
	}
	if !strings.Contains(plain, "Park") {
		t.Error("plain should contain location")
	}
	if !strings.Contains(html, "https://example.com") {
		t.Error("html should contain link")
	}
}
