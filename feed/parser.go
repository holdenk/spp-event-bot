// Package feed handles fetching and parsing RSS 2.0 and Atom XML event feeds.
package feed

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Event represents a single event parsed from a feed.
type Event struct {
	Title       string
	Description string
	Link        string
	Date        time.Time
	Location    string
	GUID        string
}

// --- RSS 2.0 structures ---

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
	// Some feeds use <location>, <ev:location>, or <georss:featureName> for location.
	Location string `xml:"location"`
}

// --- Atom structures ---

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	Links   []atomLink `xml:"link"`
	Updated string     `xml:"updated"`
	ID      string     `xml:"id"`
	// Atom doesn't have a standard location field; try <georss:featureName>.
	Location string `xml:"featureName"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// Parse parses raw XML data as either an RSS 2.0 or Atom feed and returns events.
func Parse(data []byte) ([]Event, error) {
	// Try RSS 2.0 first.
	events, err := parseRSS(data)
	if err == nil {
		return events, nil
	}

	// Try Atom.
	events, err = parseAtom(data)
	if err == nil {
		return events, nil
	}

	return nil, fmt.Errorf("failed to parse feed as RSS 2.0 or Atom: %w", err)
}

// FetchAndParse fetches a feed from the given URL via HTTP GET and parses it.
func FetchAndParse(url string) ([]Event, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "spp-event-bot/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

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

func parseRSS(data []byte) ([]Event, error) {
	var rss rssRoot
	if err := xml.Unmarshal(data, &rss); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		date := parseDate(item.PubDate)

		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		events = append(events, Event{
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(stripHTML(item.Description)),
			Link:        strings.TrimSpace(item.Link),
			Date:        date,
			Location:    strings.TrimSpace(item.Location),
			GUID:        guid,
		})
	}

	return events, nil
}

func parseAtom(data []byte) ([]Event, error) {
	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		date := parseDate(entry.Updated)

		// Prefer the alternate link; fall back to the first link.
		link := ""
		for _, l := range entry.Links {
			if l.Rel == "alternate" || l.Rel == "" {
				link = l.Href
				break
			}
		}
		if link == "" && len(entry.Links) > 0 {
			link = entry.Links[0].Href
		}

		description := entry.Summary
		if description == "" {
			description = entry.Content
		}

		events = append(events, Event{
			Title:       strings.TrimSpace(entry.Title),
			Description: strings.TrimSpace(stripHTML(description)),
			Link:        strings.TrimSpace(link),
			Date:        date,
			Location:    strings.TrimSpace(entry.Location),
			GUID:        entry.ID,
		})
	}

	return events, nil
}

// parseDate attempts to parse a date string in several common feed formats.
func parseDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	formats := []string{
		time.RFC1123Z,                // RSS standard: Mon, 02 Jan 2006 15:04:05 -0700
		time.RFC1123,                 // Mon, 02 Jan 2006 15:04:05 MST
		time.RFC3339,                 // Atom standard: 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05Z",      // Atom without offset
		"2006-01-02T15:04:05-07:00", // Atom variant
		"2006-01-02 15:04:05",       // Common format
		"2006-01-02",                // Date only
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"02 Jan 2006 15:04:05 -0700",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t
		}
	}

	return time.Time{}
}

// stripHTML removes HTML tags from a string for plain-text display.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}
