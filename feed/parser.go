// Package feed handles fetching and parsing iCal (.ics) event feeds.
package feed

import (
	"bufio"
	"context"
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
func FetchAndParse(ctx context.Context, url string) ([]Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
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
		// Also extract TZID if present (e.g., "DTSTART;TZID=America/New_York").
		baseKey := key
		var tzid string
		if idx := strings.IndexByte(key, ';'); idx >= 0 {
			params := key[idx+1:]
			baseKey = key[:idx]
			for _, param := range strings.Split(params, ";") {
				if strings.HasPrefix(param, "TZID=") {
					tzid = param[5:]
				}
			}
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
			current.Date = parseICalDate(value, tzid)
		case "DTEND":
			current.DateEnd = parseICalDate(value, tzid)
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
// Handles: \\ -> backslash, \n -> newline, \, -> comma, \; -> semicolon.
func unescapeICal(s string) string {
	// Replace \\\\ first using a placeholder to avoid double-replacement.
	// e.g., input "\\n" should become "\n" (literal), not a newline.
	s = strings.ReplaceAll(s, "\\\\", "\x00")
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\x00", "\\")
	return strings.TrimSpace(s)
}

// parseICalDate parses an iCal date or datetime value.
// Supports: 20260215T080000Z, 20260215T080000, 20260215, with optional TZID.
func parseICalDate(s string, tzid string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	// If the value ends with Z, it's UTC regardless of any TZID.
	if strings.HasSuffix(s, "Z") {
		if t, err := time.Parse("20060102T150405Z", s); err == nil {
			return t
		}
		return time.Time{}
	}

	// Load timezone if TZID was specified.
	var loc *time.Location
	if tzid != "" {
		var err error
		loc, err = time.LoadLocation(tzid)
		if err != nil {
			loc = nil // Fall back to UTC on invalid TZID.
		}
	}

	formats := []string{
		"20060102T150405", // Local datetime
		"20060102",        // Date only
	}

	for _, f := range formats {
		if loc != nil {
			if t, err := time.ParseInLocation(f, s, loc); err == nil {
				return t
			}
		} else {
			if t, err := time.Parse(f, s); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}
