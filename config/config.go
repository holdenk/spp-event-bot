// Package config loads and validates application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values for the event bot.
type Config struct {
	// FeedURL is the URL of the RSS/Atom event feed to poll.
	FeedURL string

	// PollInterval is how often to poll the feed for new events.
	PollInterval time.Duration

	// MatrixHomeserver is the base URL of the Matrix homeserver.
	MatrixHomeserver string

	// MatrixAccessToken is the access token for authenticating with Matrix.
	MatrixAccessToken string

	// MatrixRoomID is the Matrix room to post events to.
	MatrixRoomID string

	// DataDir is the directory for persisting state (e.g., last-seen timestamp).
	DataDir string
}

// DefaultPollInterval is the default polling interval if none is specified.
const DefaultPollInterval = 900 // 15 minutes in seconds

// DefaultDataDir is the default directory for persisted state.
const DefaultDataDir = "/data"

// Load reads configuration from environment variables and applies defaults.
func Load() *Config {
	pollSeconds, err := strconv.Atoi(os.Getenv("POLL_INTERVAL"))
	if err != nil || pollSeconds <= 0 {
		pollSeconds = DefaultPollInterval
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = DefaultDataDir
	}

	return &Config{
		FeedURL:           os.Getenv("FEED_URL"),
		PollInterval:      time.Duration(pollSeconds) * time.Second,
		MatrixHomeserver:  os.Getenv("MATRIX_HOMESERVER"),
		MatrixAccessToken: os.Getenv("MATRIX_ACCESS_TOKEN"),
		MatrixRoomID:      os.Getenv("MATRIX_ROOM_ID"),
		DataDir:           dataDir,
	}
}

// Validate checks that all required configuration values are present.
// Returns an error describing the first missing field, or nil if valid.
func (c *Config) Validate() error {
	if c.FeedURL == "" {
		return fmt.Errorf("FEED_URL is required")
	}
	if c.MatrixHomeserver == "" {
		return fmt.Errorf("MATRIX_HOMESERVER is required")
	}
	if c.MatrixAccessToken == "" {
		return fmt.Errorf("MATRIX_ACCESS_TOKEN is required")
	}
	if c.MatrixRoomID == "" {
		return fmt.Errorf("MATRIX_ROOM_ID is required")
	}
	return nil
}

// RedactedToken returns the access token with most characters replaced for safe logging.
func (c *Config) RedactedToken() string {
	t := c.MatrixAccessToken
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "..." + t[len(t)-4:]
}
