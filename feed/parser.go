// Package feed handles fetching and parsing iCal (.ics) event feeds.
package feed

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Event represents a single event parsed from an iCal feed.
type Event struct {
	Title       string
	Description string
	Link        string
	Date        time.Time
	DateEnd     time.Time
	Location    string
	GUID        string
}

// Parse parses raw iCal data and returns events.
func Parse(data []byte) ([]Event, error) {
	return parseICal(string(data))
}

// FetchAndParse fetches an iCal feed from the given URL via HTTP GET and parses it.
func FetchAndParse(url string) ([]Event, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "spp-event-bot/1.0")
	req.Header.Set("Accept", "text/calendar, application/ics, text/plain")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading feed body: %w", err)
	}

	return Parse(body)
}

// parseICal parses iCalendar format text into events.
// Extracts VEVENT components with SUMMARY, DESCRIPTION, DTSTART, DTEND,
// LOCATION, URL, and UID properties.
func parseICal(data string) ([]Event, error) {
	// Unfold lines per RFC 5545: lines starting with a space or tab are
	// continuations of the previous line.
	data = unfoldLines(data)

	scanner := bufio.NewScanner(strings.NewReader(data))
	var events []Event
	var current *Event
	inEvent := false

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if line == "BEGIN:VEVENT" {
			inEvent = true
			current = &Event{}
			continue
		}

		if line == "END:VEVENT" && inEvent {
			inEvent = false
			if current.Title != "" || current.GUID != "" {
				events = append(events, *current)
			}
			current = nil
			continue
		}

		if !inEvent || current == nil {
			continue
		}

		// Parse property:value pairs. Properties may have parameters
		// (e.g., DTSTART;VALUE=DATE:20260215).
		key, value := splitProperty(line)

		// Strip parameters from key (e.g., "DTSTART;VALUE=DATE" -> "DTSTART").
		baseKey := key
		if idx := strings.IndexByte(key, ';'); idx >= 0 {
			baseKey = key[:idx]
		}

		switch baseKey {
		case "SUMMARY":
			current.Title = unescapeICal(value)
		case "DESCRIPTION":
			current.Description = unescapeICal(value)
		case "LOCATION":
			current.Location = unescapeICal(value)
		case "URL":
			current.Link = value
		case "UID":
			current.GUID = value
		case "DTSTART":
			current.Date = parseICalDate(value)
		case "DTEND":
			current.DateEnd = parseICalDate(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ical data: %w", err)
	}

	return events, nil
}

// splitProperty splits an iCal line into property name (with any parameters) and value.
// Example: "DTSTART;VALUE=DATE:20260215" -> ("DTSTART;VALUE=DATE", "20260215")
func splitProperty(line string) (string, string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	return line[:idx], line[idx+1:]
}

// unfoldLines handles RFC 5545 line folding: lines that start with a space
// or horizontal tab are continuations of the previous line.
func unfoldLines(s string) string {
	var b strings.Builder
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			// Continuation: append without the leading whitespace.
			b.WriteString(line[1:])
		} else {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
	}
	return b.String()
}

// unescapeICal unescapes iCal text values per RFC 5545.
// Handles: \n -> newline, \, -> comma, \; -> semicolon, \\ -> backslash.
func unescapeICal(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return strings.TrimSpace(s)
}

// parseICalDate parses an iCal date or datetime value.
// Supports: 20260215T080000Z, 20260215T080000, 20260215, with optional TZID.
func parseICalDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	formats := []string{
		"20060102T150405Z",  // UTC datetime
		"20060102T150405",   // Local datetime
		"20060102",          // Date only
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}

	return time.Time{}
}
