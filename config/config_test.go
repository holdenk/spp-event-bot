package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear all relevant env vars.
	for _, k := range []string{"FEED_URL", "POLL_INTERVAL", "MATRIX_HOMESERVER", "MATRIX_ACCESS_TOKEN", "MATRIX_ROOM_ID", "DATA_DIR"} {
		os.Unsetenv(k)
	}

	cfg := Load()

	if cfg.PollInterval != 900*time.Second {
		t.Errorf("default PollInterval = %v, want %v", cfg.PollInterval, 900*time.Second)
	}
	if cfg.DataDir != "/data" {
		t.Errorf("default DataDir = %q, want %q", cfg.DataDir, "/data")
	}
	if cfg.FeedURL != "" {
		t.Errorf("FeedURL should be empty, got %q", cfg.FeedURL)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("FEED_URL", "https://example.com/feed.xml")
	os.Setenv("POLL_INTERVAL", "300")
	os.Setenv("MATRIX_HOMESERVER", "https://matrix.example.com")
	os.Setenv("MATRIX_ACCESS_TOKEN", "syt_test_token_123")
	os.Setenv("MATRIX_ROOM_ID", "!room:example.com")
	os.Setenv("DATA_DIR", "/tmp/botdata")
	defer func() {
		for _, k := range []string{"FEED_URL", "POLL_INTERVAL", "MATRIX_HOMESERVER", "MATRIX_ACCESS_TOKEN", "MATRIX_ROOM_ID", "DATA_DIR"} {
			os.Unsetenv(k)
		}
	}()

	cfg := Load()

	if cfg.FeedURL != "https://example.com/feed.xml" {
		t.Errorf("FeedURL = %q", cfg.FeedURL)
	}
	if cfg.PollInterval != 300*time.Second {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 300*time.Second)
	}
	if cfg.MatrixHomeserver != "https://matrix.example.com" {
		t.Errorf("MatrixHomeserver = %q", cfg.MatrixHomeserver)
	}
	if cfg.MatrixAccessToken != "syt_test_token_123" {
		t.Errorf("MatrixAccessToken = %q", cfg.MatrixAccessToken)
	}
	if cfg.MatrixRoomID != "!room:example.com" {
		t.Errorf("MatrixRoomID = %q", cfg.MatrixRoomID)
	}
	if cfg.DataDir != "/tmp/botdata" {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
}

func TestLoadInvalidPollInterval(t *testing.T) {
	os.Setenv("POLL_INTERVAL", "not-a-number")
	defer os.Unsetenv("POLL_INTERVAL")

	cfg := Load()
	if cfg.PollInterval != 900*time.Second {
		t.Errorf("invalid POLL_INTERVAL should use default, got %v", cfg.PollInterval)
	}
}

func TestLoadNegativePollInterval(t *testing.T) {
	os.Setenv("POLL_INTERVAL", "-10")
	defer os.Unsetenv("POLL_INTERVAL")

	cfg := Load()
	if cfg.PollInterval != 900*time.Second {
		t.Errorf("negative POLL_INTERVAL should use default, got %v", cfg.PollInterval)
	}
}

func TestValidateAllPresent(t *testing.T) {
	cfg := &Config{
		FeedURL:           "https://example.com/feed.xml",
		MatrixHomeserver:  "https://matrix.example.com",
		MatrixAccessToken: "token",
		MatrixRoomID:      "!room:example.com",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidateMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing feed url",
			cfg:     Config{MatrixHomeserver: "h", MatrixAccessToken: "t", MatrixRoomID: "r"},
			wantErr: "FEED_URL",
		},
		{
			name:    "missing homeserver",
			cfg:     Config{FeedURL: "f", MatrixAccessToken: "t", MatrixRoomID: "r"},
			wantErr: "MATRIX_HOMESERVER",
		},
		{
			name:    "missing access token",
			cfg:     Config{FeedURL: "f", MatrixHomeserver: "h", MatrixRoomID: "r"},
			wantErr: "MATRIX_ACCESS_TOKEN",
		},
		{
			name:    "missing room id",
			cfg:     Config{FeedURL: "f", MatrixHomeserver: "h", MatrixAccessToken: "t"},
			wantErr: "MATRIX_ROOM_ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if got := err.Error(); got != tt.wantErr+" is required" {
				t.Errorf("error = %q, want to contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestRedactedToken(t *testing.T) {
	tests := []struct {
		token, want string
	}{
		{"syt_abcdefghijklmnop_12345678", "syt_...5678"},
		{"short", "****"},
		{"12345678", "****"},
		{"123456789", "1234...6789"},
		{"", "****"},
	}

	for _, tt := range tests {
		cfg := &Config{MatrixAccessToken: tt.token}
		got := cfg.RedactedToken()
		if got != tt.want {
			t.Errorf("RedactedToken(%q) = %q, want %q", tt.token, got, tt.want)
		}
	}
}
