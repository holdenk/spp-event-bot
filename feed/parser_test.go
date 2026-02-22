package feed

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>SPP Rides</title>
    <item>
      <title>Saturday Morning Ride</title>
      <description>Join us for a chill 30-mile ride through the hills.</description>
      <link>https://example.com/rides/saturday</link>
      <pubDate>Sat, 15 Feb 2026 08:00:00 -0800</pubDate>
      <guid>ride-001</guid>
      <location>Coffee Shop Parking Lot</location>
    </item>
    <item>
      <title>Tuesday Evening Spin</title>
      <description>&lt;p&gt;Easy pace, &lt;b&gt;everyone&lt;/b&gt; welcome.&lt;/p&gt;</description>
      <link>https://example.com/rides/tuesday</link>
      <pubDate>Tue, 18 Feb 2026 17:30:00 -0800</pubDate>
      <guid>ride-002</guid>
    </item>
  </channel>
</rss>`

const testAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>SPP Events</title>
  <entry>
    <title>Group Dinner</title>
    <summary>Monthly dinner at the usual place.</summary>
    <link href="https://example.com/events/dinner" rel="alternate"/>
    <updated>2026-02-20T19:00:00Z</updated>
    <id>event-dinner-001</id>
  </entry>
  <entry>
    <title>Park Hangout</title>
    <content>Bring snacks and games.</content>
    <link href="https://example.com/events/park"/>
    <updated>2026-02-22T14:00:00Z</updated>
    <id>event-park-001</id>
  </entry>
</feed>`

func TestParseRSS(t *testing.T) {
	events, err := Parse([]byte(testRSS))
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
	if e.GUID != "ride-001" {
		t.Errorf("guid = %q, want %q", e.GUID, "ride-001")
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

	// Second event should have HTML stripped from description.
	e2 := events[1]
	if e2.Description != "Easy pace, everyone welcome." {
		t.Errorf("html-stripped description = %q, want %q", e2.Description, "Easy pace, everyone welcome.")
	}
}

func TestParseAtom(t *testing.T) {
	events, err := Parse([]byte(testAtom))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	e := events[0]
	if e.Title != "Group Dinner" {
		t.Errorf("title = %q, want %q", e.Title, "Group Dinner")
	}
	if e.Link != "https://example.com/events/dinner" {
		t.Errorf("link = %q, want %q", e.Link, "https://example.com/events/dinner")
	}
	if e.GUID != "event-dinner-001" {
		t.Errorf("guid = %q, want %q", e.GUID, "event-dinner-001")
	}
	if e.Description != "Monthly dinner at the usual place." {
		t.Errorf("description = %q", e.Description)
	}

	// Second entry uses <content> instead of <summary>.
	e2 := events[1]
	if e2.Description != "Bring snacks and games." {
		t.Errorf("content-as-description = %q, want %q", e2.Description, "Bring snacks and games.")
	}
}

func TestParseEmptyFeed(t *testing.T) {
	rss := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`
	events, err := Parse([]byte(rss))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestParseInvalidXML(t *testing.T) {
	_, err := Parse([]byte("this is not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

func TestParseDateFormats(t *testing.T) {
	tests := []struct {
		input string
		want  string // expected date in YYYY-MM-DD format, or "" for zero
	}{
		{"Sat, 15 Feb 2026 08:00:00 -0800", "2026-02-15"},
		{"2026-02-20T19:00:00Z", "2026-02-20"},
		{"2026-02-20T19:00:00+05:30", "2026-02-20"},
		{"2026-02-20 19:00:00", "2026-02-20"},
		{"2026-02-20", "2026-02-20"},
		{"", ""},
		{"not a date", ""},
	}

	for _, tt := range tests {
		got := parseDate(tt.input)
		if tt.want == "" {
			if !got.IsZero() {
				t.Errorf("parseDate(%q) = %v, want zero", tt.input, got)
			}
			continue
		}
		gotStr := got.Format("2006-01-02")
		if gotStr != tt.want {
			t.Errorf("parseDate(%q) = %s, want %s", tt.input, gotStr, tt.want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"<p>hello</p>", "hello"},
		{"<b>bold</b> and <i>italic</i>", "bold and italic"},
		{"no tags here", "no tags here"},
		{"<a href=\"x\">link</a>", "link"},
		{"", ""},
	}

	for _, tt := range tests {
		got := stripHTML(tt.input)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFetchAndParse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(testRSS))
	}))
	defer server.Close()

	events, err := FetchAndParse(server.URL)
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

	_, err := FetchAndParse(server.URL)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestRSSGUIDFallback(t *testing.T) {
	// When GUID is empty, it should fall back to Link.
	rss := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>No GUID Event</title>
      <link>https://example.com/fallback</link>
      <pubDate>Mon, 01 Jan 2026 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	events, err := Parse([]byte(rss))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].GUID != "https://example.com/fallback" {
		t.Errorf("guid = %q, want link fallback %q", events[0].GUID, "https://example.com/fallback")
	}
}

func TestAtomLinkSelection(t *testing.T) {
	// Should prefer rel="alternate", fall back to first link.
	atom := `<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Multi Link</title>
    <link href="https://example.com/self" rel="self"/>
    <link href="https://example.com/alternate" rel="alternate"/>
    <updated>2026-01-01T00:00:00Z</updated>
    <id>multi-link-001</id>
  </entry>
</feed>`

	events, err := Parse([]byte(atom))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Link != "https://example.com/alternate" {
		t.Errorf("link = %q, want alternate link", events[0].Link)
	}
}

func TestEventDateSorting(t *testing.T) {
	// Verify events maintain feed order (not sorted by date).
	rss := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Later Event</title>
      <pubDate>Wed, 25 Feb 2026 10:00:00 +0000</pubDate>
      <guid>later</guid>
    </item>
    <item>
      <title>Earlier Event</title>
      <pubDate>Mon, 23 Feb 2026 10:00:00 +0000</pubDate>
      <guid>earlier</guid>
    </item>
  </channel>
</rss>`

	events, err := Parse([]byte(rss))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Events should be in feed order.
	if events[0].Title != "Later Event" {
		t.Errorf("first event = %q, want %q", events[0].Title, "Later Event")
	}
	if events[1].Title != "Earlier Event" {
		t.Errorf("second event = %q, want %q", events[1].Title, "Earlier Event")
	}

	// Verify dates are actually parsed correctly.
	if !events[0].Date.After(events[1].Date) {
		t.Error("later event date should be after earlier event date")
	}
	_ = time.Now() // use the time import
}
