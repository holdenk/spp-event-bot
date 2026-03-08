package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/sparklingpinkpandas/spp-event-bot/config"
	"github.com/sparklingpinkpandas/spp-event-bot/feed"
	"github.com/sparklingpinkpandas/spp-event-bot/matrix"
)

const stateFile = "event-state.json"

// EventRecord tracks the state of a single event by GUID.
type EventRecord struct {
	Title     string    `json:"title"`
	Date      time.Time `json:"date"`
	Published bool      `json:"published"`
	Reminded  bool      `json:"reminded"`
}

// EventState holds all tracked event records, keyed by GUID.
type EventState struct {
	Events map[string]*EventRecord `json:"events"`
}

func newEventState() *EventState {
	return &EventState{Events: make(map[string]*EventRecord)}
}

func main() {
	// Set up structured logging.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Load and validate configuration.
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting spp-event-bot",
		"feed_url", cfg.FeedURL,
		"poll_interval", cfg.PollInterval.String(),
		"homeserver", cfg.MatrixHomeserver,
		"room_id", cfg.MatrixRoomID,
		"access_token", cfg.RedactedToken(),
		"data_dir", cfg.DataDir,
	)

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("failed to create data directory", "path", cfg.DataDir, "error", err)
		os.Exit(1)
	}

	// Create Matrix client.
	mxClient, err := matrix.NewClient(cfg.MatrixHomeserver, cfg.MatrixAccessToken)
	if err != nil {
		slog.Error("failed to create matrix client", "error", err)
		os.Exit(1)
	}
	slog.Info("matrix client initialized")

	// Load event state from disk.
	state := loadState(cfg.DataDir)
	slog.Info("loaded event state", "tracked_events", len(state.Events))

	// Set up graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Run the poll loop.
	pollLoop(ctx, cfg, mxClient, state, time.Now)

	slog.Info("spp-event-bot stopped")
}

// pollLoop fetches the feed on a timer and posts new events to Matrix.
func pollLoop(ctx context.Context, cfg *config.Config, mxClient *matrix.Client, state *EventState, now func() time.Time) {
	// Do an immediate first poll.
	poll(ctx, cfg, mxClient, state, now)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("poll loop shutting down")
			return
		case <-ticker.C:
			poll(ctx, cfg, mxClient, state, now)
		}
	}
}

// poll performs a single fetch-parse-post cycle.
func poll(ctx context.Context, cfg *config.Config, mxClient *matrix.Client, state *EventState, now func() time.Time) {
	slog.Info("polling feed", "url", cfg.FeedURL)

	events, err := feed.FetchAndParse(ctx, cfg.FeedURL)
	if err != nil {
		slog.Error("failed to fetch/parse feed", "error", err)
		return
	}

	slog.Info("fetched events from feed", "count", len(events))

	currentTime := now()

	// Filter to only future events that haven't been published yet.
	var newEvents []feed.Event
	for _, ev := range events {
		if ev.Date.IsZero() || ev.GUID == "" {
			continue
		}
		if !ev.Date.After(currentTime) {
			continue
		}
		rec, exists := state.Events[ev.GUID]
		if !exists || !rec.Published {
			newEvents = append(newEvents, ev)
		}
	}

	if len(newEvents) > 0 {
		slog.Info("found new future events", "count", len(newEvents))

		// Sort events oldest-first.
		sort.Slice(newEvents, func(i, j int) bool {
			return newEvents[i].Date.Before(newEvents[j].Date)
		})

		// Post new events oldest-first.
		posted := 0
		for _, ev := range newEvents {
			if ctx.Err() != nil {
				slog.Info("context cancelled, stopping event posting")
				break
			}

			slog.Info("posting event", "title", ev.Title, "date", ev.Date, "guid", ev.GUID)

			if err := mxClient.PostEvent(ctx, cfg.MatrixRoomID, ev); err != nil {
				slog.Error("failed to post event", "title", ev.Title, "error", err)
				// Track as not published so we retry next poll.
				if _, exists := state.Events[ev.GUID]; !exists {
					state.Events[ev.GUID] = &EventRecord{
						Title:     ev.Title,
						Date:      ev.Date,
						Published: false,
						Reminded:  false,
					}
				}
				continue
			}
			posted++

			state.Events[ev.GUID] = &EventRecord{
				Title:     ev.Title,
				Date:      ev.Date,
				Published: true,
				Reminded:  false,
			}
		}

		slog.Info("posted events", "count", posted)
	} else {
		slog.Info("no new future events found")
	}

	// Send day-of reminders for events happening today that were published.
	todayStart := startOfDay(currentTime)
	todayEnd := todayStart.Add(24 * time.Hour)

	reminded := 0
	for _, ev := range events {
		if ev.Date.IsZero() || ev.GUID == "" {
			continue
		}

		rec, exists := state.Events[ev.GUID]
		if !exists || !rec.Published || rec.Reminded {
			continue
		}

		if !ev.Date.Before(todayStart) && ev.Date.Before(todayEnd) {
			if ctx.Err() != nil {
				break
			}

			slog.Info("sending day-of reminder", "title", ev.Title, "date", ev.Date, "guid", ev.GUID)

			if err := mxClient.PostReminder(ctx, cfg.MatrixRoomID, ev); err != nil {
				slog.Error("failed to send reminder", "title", ev.Title, "error", err)
				continue
			}

			rec.Reminded = true
			reminded++
		}
	}

	if reminded > 0 {
		slog.Info("sent reminders", "count", reminded)
	}

	// Prune events that are more than 7 days in the past.
	pruneThreshold := currentTime.AddDate(0, 0, -7)
	for guid, rec := range state.Events {
		if rec.Date.Before(pruneThreshold) {
			delete(state.Events, guid)
		}
	}

	saveState(cfg.DataDir, state)
}

// startOfDay returns the start of the day (midnight) for the given time in its location.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// loadState reads the event state from disk.
// Returns a new empty state if the file doesn't exist or can't be parsed.
func loadState(dataDir string) *EventState {
	path := filepath.Join(dataDir, stateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read state file", "path", path, "error", err)
		}
		return newEventState()
	}

	var state EventState
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("failed to parse state file", "error", err)
		return newEventState()
	}

	if state.Events == nil {
		state.Events = make(map[string]*EventRecord)
	}

	return &state
}

// saveState writes the event state to disk atomically.
func saveState(dataDir string, state *EventState) {
	path := filepath.Join(dataDir, stateFile)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		slog.Error("failed to marshal state", "error", err)
		return
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		slog.Error("failed to write state temp file", "path", tmpPath, "error", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		slog.Error("failed to rename state file", "from", tmpPath, "to", path, "error", err)
		return
	}

	slog.Info("saved event state", "tracked_events", len(state.Events))
}
