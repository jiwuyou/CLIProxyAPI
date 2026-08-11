// Package codebuddy provides authentication and token management functionality
// for CodeBuddy AI services. It handles OAuth2 token storage, serialization,
// and retrieval for maintaining authenticated sessions with the CodeBuddy API.
package codebuddy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
)

// CodeBuddyTokenStorage stores OAuth token information for CodeBuddy API authentication.
// It maintains compatibility with the existing auth system while adding CodeBuddy-specific fields
// for managing access tokens and user account information.
type CodeBuddyTokenStorage struct {
	// AccessToken is the OAuth2 access token used for authenticating API requests.
	AccessToken string `json:"access_token"`
	// RefreshToken is the OAuth2 refresh token used to obtain new access tokens.
	RefreshToken string `json:"refresh_token"`
	// ExpiresIn is the number of seconds until the access token expires.
	ExpiresIn int64 `json:"expires_in"`
	// Expired is the absolute access-token expiration timestamp.
	Expired string `json:"expired,omitempty"`
	// RefreshExpiresIn is the number of seconds until the refresh token expires.
	RefreshExpiresIn int64 `json:"refresh_expires_in,omitempty"`
	// TokenType is the type of token, typically "bearer".
	TokenType string `json:"token_type"`
	// Domain is the CodeBuddy service domain/region.
	Domain string `json:"domain"`
	// UserID is the user ID associated with this token.
	UserID string `json:"user_id"`
	// Type indicates the authentication provider type, always "codebuddy" for this storage.
	Type string `json:"type"`
	// LastRefresh is the timestamp of the most recent successful token acquisition.
	LastRefresh string `json:"last_refresh,omitempty"`

	// Metadata holds generic auth-file fields that are flattened during serialization.
	Metadata map[string]any `json:"-"`
}

// SetMetadata preserves generic auth-file fields such as disabled and headers.
func (s *CodeBuddyTokenStorage) SetMetadata(metadata map[string]any) {
	if s == nil {
		return
	}
	s.Metadata = metadata
}

// MarkRefreshed records the absolute expiry and successful acquisition time.
func (s *CodeBuddyTokenStorage) MarkRefreshed(now time.Time) {
	if s == nil {
		return
	}
	now = now.UTC()
	s.LastRefresh = now.Format(time.RFC3339)
	if s.ExpiresIn > 0 {
		s.Expired = now.Add(time.Duration(s.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
}

// SaveTokenToFile serializes the CodeBuddy token storage to a JSON file.
// This method creates the necessary directory structure and writes the token
// data in JSON format to the specified file path for persistent storage.
//
// Parameters:
//   - authFilePath: The full path where the token file should be saved
//
// Returns:
//   - error: An error if the operation fails, nil otherwise
func (s *CodeBuddyTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	s.Type = "codebuddy"
	if err := os.MkdirAll(filepath.Dir(authFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, errMerge := misc.MergeMetadata(s, s.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}

	f, err := os.OpenFile(authFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if err = json.NewEncoder(f).Encode(data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}
