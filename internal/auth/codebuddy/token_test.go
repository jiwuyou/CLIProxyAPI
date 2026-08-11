package codebuddy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCodeBuddyTokenStorageLifecycleAndMetadataPersistence(t *testing.T) {
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	storage := &CodeBuddyTokenStorage{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
		UserID:       "user-1",
	}
	storage.MarkRefreshed(now)
	storage.SetMetadata(map[string]any{"disabled": true, "note": "keep-me"})

	if storage.LastRefresh != now.Format(time.RFC3339) {
		t.Fatalf("LastRefresh = %q, want %q", storage.LastRefresh, now.Format(time.RFC3339))
	}
	wantExpired := now.Add(time.Hour).Format(time.RFC3339)
	if storage.Expired != wantExpired {
		t.Fatalf("Expired = %q, want %q", storage.Expired, wantExpired)
	}

	path := filepath.Join(t.TempDir(), "codebuddy.json")
	if err := storage.SaveTokenToFile(path); err != nil {
		t.Fatalf("SaveTokenToFile() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	var saved map[string]any
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode token file: %v", err)
	}
	if saved["disabled"] != true || saved["note"] != "keep-me" {
		t.Fatalf("generic metadata was not preserved: %#v", saved)
	}
	if saved["expired"] != wantExpired || saved["last_refresh"] != now.Format(time.RFC3339) {
		t.Fatalf("lifecycle metadata was not preserved: %#v", saved)
	}
}
