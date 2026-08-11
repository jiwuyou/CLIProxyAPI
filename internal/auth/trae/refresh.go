package trae

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
)

const exchangeTokenPath = "/cloudide/api/v3/trae/oauth/ExchangeToken"

// ExchangePAT exchanges a Trae CLI personal access token for a Cloud IDE JWT.
func ExchangePAT(ctx context.Context, cfg *config.Config, pat, baseURL string) (*TokenStorage, error) {
	pat = strings.TrimSpace(pat)
	if pat == "" {
		return nil, fmt.Errorf("trae: personal access token is missing")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultCLIBaseURL
	}
	body, err := json.Marshal(map[string]string{"RefreshToken": pat})
	if err != nil {
		return nil, fmt.Errorf("trae: encode PAT exchange request: %w", err)
	}
	raw, err := exchangeToken(ctx, cfg, baseURL, body, true)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("trae: parse PAT exchange response: %w", err)
	}
	if code, ok := numberValue(envelope["code"]); ok && code != 0 {
		message := firstString(envelope, "message", "msg", "Message")
		if message == "" {
			message = "unknown error"
		}
		return nil, fmt.Errorf("trae: PAT exchange failed with code %d: %s", code, message)
	}
	result := nestedMap(envelope, "Data", "data", "Result", "result")
	token := firstString(result, "Token", "token", "AccessToken", "accessToken", "access_token")
	if token == "" {
		return nil, fmt.Errorf("trae: PAT exchange response did not contain Data.Token")
	}
	storage := &TokenStorage{
		Type:                Provider,
		AuthKind:            AuthKindPAT,
		Edition:             EditionCN,
		Token:               token,
		PersonalAccessToken: pat,
		ExpiredAt:           normalizeExpiryValue(firstValue(result, "TokenExpireAt", "tokenExpireAt", "expiredAt", "expired")),
		AuthBaseURL:         baseURL,
		ChatBaseURL:         baseURL,
	}
	storage.LastRefresh = time.Now().UTC().Format(time.RFC3339)
	return storage, nil
}

// Refresh obtains a fresh token for either a CLI PAT or an imported IDE credential.
func Refresh(ctx context.Context, cfg *config.Config, storage *TokenStorage) (*TokenStorage, error) {
	if storage == nil {
		return nil, fmt.Errorf("trae: credential is nil")
	}
	if storage.UsesCLIRawChat() {
		updated, err := ExchangePAT(ctx, cfg, storage.PersonalAccessToken, storage.AuthBaseURL)
		if err != nil {
			return nil, err
		}
		updated.Metadata = storage.Metadata
		return updated, nil
	}
	if strings.TrimSpace(storage.RefreshToken) == "" {
		return nil, fmt.Errorf("trae: refresh token is missing")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(storage.AuthBaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAuthBaseURL(storage.Edition)
	}
	body, err := json.Marshal(map[string]string{
		"ClientID":     "ono9krqynydwx5",
		"RefreshToken": storage.RefreshToken,
		"ClientSecret": "-",
		"UserID":       storage.UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("trae: encode refresh request: %w", err)
	}
	raw, err := exchangeToken(ctx, cfg, baseURL, body, false)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("trae: parse refresh response: %w", err)
	}
	result := nestedMap(envelope, "Result", "result", "Data", "data")
	token := firstString(result, "Token", "token", "AccessToken", "accessToken", "access_token")
	if token == "" {
		return nil, fmt.Errorf("trae: refresh response did not contain a token")
	}
	updated := *storage
	updated.AuthKind = AuthKindIDE
	updated.Token = token
	if refreshToken := firstString(result, "RefreshToken", "refreshToken", "refresh_token"); refreshToken != "" {
		updated.RefreshToken = refreshToken
	}
	if expired := firstValue(result, "TokenExpireAt", "tokenExpireAt", "expiredAt", "expired"); expired != nil {
		updated.ExpiredAt = normalizeExpiryValue(expired)
	}
	if refreshExpired := firstValue(result, "RefreshTokenExpireAt", "refreshTokenExpireAt", "refreshExpiredAt"); refreshExpired != nil {
		updated.RefreshExpiredAt = normalizeExpiryValue(refreshExpired)
	}
	updated.LastRefresh = time.Now().UTC().Format(time.RFC3339)
	return &updated, nil
}

func exchangeToken(ctx context.Context, cfg *config.Config, baseURL string, body []byte, pat bool) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+exchangeTokenPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("trae: create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if pat {
		req.Header["X-Cloudide-Token"] = []string{""}
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if cfg != nil {
		client = util.SetProxy(&cfg.SDKConfig, client)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trae: token exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trae: read token exchange response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("trae: token exchange returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func nestedMap(value map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if nested, ok := value[key].(map[string]any); ok {
			return nested
		}
	}
	return value
}

func firstValue(value map[string]any, keys ...string) any {
	for _, key := range keys {
		if candidate, ok := value[key]; ok && candidate != nil {
			return candidate
		}
	}
	return nil
}

func numberValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	default:
		return 0, false
	}
}

func normalizeExpiryValue(value any) string {
	if value == nil {
		return ""
	}
	if epoch, ok := numberValue(value); ok {
		return normalizeExpiry(fmt.Sprintf("%d", epoch))
	}
	return normalizeExpiry(fmt.Sprint(value))
}

func normalizeExpiry(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	var epoch int64
	if _, err := fmt.Sscan(value, &epoch); err == nil {
		if epoch > 1_000_000_000_000 {
			epoch /= 1000
		}
		if epoch > 0 {
			return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
		}
	}
	return value
}
