package qoder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestQoderTokenStoragePersistsLifecycleAndGenericMetadata(t *testing.T) {
	storage := &QoderTokenStorage{
		Token:        "token",
		RefreshToken: "refresh-token",
		ExpireTime:   1786449600000,
		Expired:      "2026-08-11T12:00:00Z",
		LastRefresh:  "2026-08-11T11:00:00Z",
	}
	storage.SetMetadata(map[string]any{"disabled": true})

	path := filepath.Join(t.TempDir(), "qoder.json")
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
	if saved["disabled"] != true {
		t.Fatalf("disabled = %#v, want true", saved["disabled"])
	}
	if saved["expired"] != storage.Expired || saved["last_refresh"] != storage.LastRefresh {
		t.Fatalf("lifecycle metadata was not preserved: %#v", saved)
	}
}
