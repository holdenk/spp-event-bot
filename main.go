package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/sparklingpinkpandas/spp-event-bot/config"
	"github.com/sparklingpinkpandas/spp-event-bot/feed"
	"github.com/sparklingpinkpandas/spp-event-bot/matrix"
)

const lastSeenFile = "last-seen.txt"

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

	// Load last-seen timestamp from disk.
	lastSeen := loadLastSeen(cfg.DataDir)
	slog.Info("loaded last-seen timestamp", "last_seen", lastSeen)

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
	pollLoop(ctx, cfg, mxClient, &lastSeen)

	slog.Info("spp-event-bot stopped")
}

// pollLoop fetches the feed on a timer and posts new events to Matrix.
func pollLoop(ctx context.Context, cfg *config.Config, mxClient *matrix.Client, lastSeen *time.Time) {
	// Do an immediate first poll.
	poll(ctx, cfg, mxClient, lastSeen)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("poll loop shutting down")
			return
		case <-ticker.C:
			poll(ctx, cfg, mxClient, lastSeen)
		}
	}
}

// poll performs a single fetch-parse-post cycle.
func poll(ctx context.Context, cfg *config.Config, mxClient *matrix.Client, lastSeen *time.Time) {
	slog.Info("polling feed", "url", cfg.FeedURL)

	events, err := feed.FetchAndParse(ctx, cfg.FeedURL)
	if err != nil {
		slog.Error("failed to fetch/parse feed", "error", err)
		return
	}

	slog.Info("fetched events from feed", "count", len(events))

	// Filter to only new events (those with a date after lastSeen).
	var newEvents []feed.Event
	for _, ev := range events {
		// Skip undated events — we can only do date-based filtering.
		if ev.Date.IsZero() {
			continue
		}
		if ev.Date.After(*lastSeen) {
			newEvents = append(newEvents, ev)
		}
	}

	if len(newEvents) == 0 {
		slog.Info("no new events found")
		return
	}

	slog.Info("found new events", "count", len(newEvents))

	// Sort events oldest-first.
	sort.Slice(newEvents, func(i, j int) bool {
		return newEvents[i].Date.Before(newEvents[j].Date)
	})

	// Post new events oldest-first.
	latestDate := *lastSeen
	posted := 0
	for _, ev := range newEvents {
		// Check for cancellation between posts.
		if ctx.Err() != nil {
			slog.Info("context cancelled, stopping event posting")
			break
		}

		slog.Info("posting event", "title", ev.Title, "date", ev.Date, "guid", ev.GUID)

		if err := mxClient.PostEvent(ctx, cfg.MatrixRoomID, ev); err != nil {
			slog.Error("failed to post event", "title", ev.Title, "error", err)
			continue
		}
		posted++

		if ev.Date.After(latestDate) {
			latestDate = ev.Date
		}
	}

	slog.Info("posted events", "count", posted)

	// Update last-seen timestamp if we posted anything.
	if latestDate.After(*lastSeen) {
		*lastSeen = latestDate
		saveLastSeen(cfg.DataDir, *lastSeen)
	}
}

// loadLastSeen reads the last-seen timestamp from disk.
// Returns zero time if the file doesn't exist or can't be parsed.
func loadLastSeen(dataDir string) time.Time {
	path := filepath.Join(dataDir, lastSeenFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read last-seen file", "path", path, "error", err)
		}
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		slog.Warn("failed to parse last-seen timestamp", "raw", string(data), "error", err)
		return time.Time{}
	}

	return t
}

// saveLastSeen writes the last-seen timestamp to disk atomically.
func saveLastSeen(dataDir string, t time.Time) {
	path := filepath.Join(dataDir, lastSeenFile)
	content := t.Format(time.RFC3339) + "\n"

	// Write to a temp file and rename for atomicity.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		slog.Error("failed to write last-seen temp file", "path", tmpPath, "error", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		slog.Error("failed to rename last-seen file", "from", tmpPath, "to", path, "error", err)
		return
	}

	slog.Info("saved last-seen timestamp", "time", t.Format(time.RFC3339))
}

