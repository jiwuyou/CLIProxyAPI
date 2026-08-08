package trae

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExchangePAT(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != exchangeTokenPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if _, ok := r.Header["X-Cloudide-Token"]; !ok {
			t.Fatal("X-Cloudide-Token header is missing")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["RefreshToken"] != "trae-lt-test" || len(body) != 1 {
			t.Fatalf("unexpected request body: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"Data": map[string]any{"Token": "jwt-test", "TokenExpireAt": expiresAt},
		})
	}))
	defer server.Close()

	storage, err := ExchangePAT(context.Background(), nil, "trae-lt-test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if storage.AuthKind != AuthKindPAT || storage.Token != "jwt-test" || storage.PersonalAccessToken != "trae-lt-test" {
		t.Fatalf("unexpected storage: %#v", storage)
	}
	if storage.AuthBaseURL != server.URL || storage.ChatBaseURL != server.URL || storage.ExpiredAt == "" {
		t.Fatalf("unexpected endpoints or expiry: %#v", storage)
	}
}

func TestRefreshPATUsesPersonalAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"Data": map[string]any{"Token": "jwt-new", "TokenExpireAt": time.Now().Add(time.Hour).UnixMilli()},
		})
	}))
	defer server.Close()

	updated, err := Refresh(context.Background(), nil, &TokenStorage{
		Type: Provider, AuthKind: AuthKindPAT, Token: "jwt-old",
		PersonalAccessToken: "trae-lt-test", AuthBaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Token != "jwt-new" || updated.PersonalAccessToken != "trae-lt-test" || !updated.UsesCLIRawChat() {
		t.Fatalf("unexpected refreshed storage: %#v", updated)
	}
}
