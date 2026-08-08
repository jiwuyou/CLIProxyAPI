// Package trae provides CLI PAT login, IDE credential import, and token refresh support for Trae.
package trae

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
)

const (
	Provider = "trae"

	EditionCN     = "cn"
	EditionSG     = "sg"
	EditionSoloCN = "solo"
	EditionSoloSG = "solo-sg"

	AuthKindIDE = "ide"
	AuthKindPAT = "pat"

	DefaultChatBaseCN = "https://trae-api-cn.mchost.guru"
	DefaultChatBaseSG = "https://a0ai-api-sg.byteintlapi.com"
	DefaultAuthBaseCN = "https://api.trae.cn"
	DefaultAuthBaseSG = "https://api-sg-central.trae.ai"

	DefaultCLIBaseURL = "https://api.enterprise.trae.cn"
)

// TokenStorage is the persisted Trae credential record.
type TokenStorage struct {
	Type                string         `json:"type"`
	AuthKind            string         `json:"auth_kind,omitempty"`
	Edition             string         `json:"edition"`
	Token               string         `json:"token"`
	PersonalAccessToken string         `json:"personal_access_token,omitempty"`
	RefreshToken        string         `json:"refresh_token,omitempty"`
	UserID              string         `json:"user_id,omitempty"`
	Email               string         `json:"email,omitempty"`
	Username            string         `json:"username,omitempty"`
	ExpiredAt           string         `json:"expired,omitempty"`
	RefreshExpiredAt    string         `json:"refresh_expired,omitempty"`
	AuthBaseURL         string         `json:"auth_base_url,omitempty"`
	ChatBaseURL         string         `json:"chat_base_url,omitempty"`
	Metadata            map[string]any `json:"-"`
}

// SetMetadata preserves generic auth-file fields such as disabled and headers.
func (s *TokenStorage) SetMetadata(metadata map[string]any) {
	if s == nil {
		return
	}
	s.Metadata = metadata
}

// SaveTokenToFile writes a Trae credential with owner-only permissions.
func (s *TokenStorage) SaveTokenToFile(path string) error {
	if s == nil {
		return fmt.Errorf("trae: token storage is nil")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("trae: credential path is empty")
	}
	misc.LogSavingCredentials(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("trae: create credential directory: %w", err)
	}

	payload := map[string]any{
		"type":                  Provider,
		"auth_kind":             NormalizeAuthKind(s.AuthKind, s.PersonalAccessToken),
		"edition":               NormalizeEdition(s.Edition),
		"token":                 s.Token,
		"personal_access_token": s.PersonalAccessToken,
		"refresh_token":         s.RefreshToken,
		"user_id":               s.UserID,
		"email":                 s.Email,
		"username":              s.Username,
		"expired":               s.ExpiredAt,
		"refresh_expired":       s.RefreshExpiredAt,
		"auth_base_url":         s.AuthBaseURL,
		"chat_base_url":         s.ChatBaseURL,
	}
	for key, value := range s.Metadata {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("trae: marshal credential: %w", err)
	}
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("trae: write credential: %w", err)
	}
	return nil
}

// NormalizeAuthKind canonicalizes a Trae credential source. Older credential
// files did not include auth_kind, so they continue to default to IDE mode.
func NormalizeAuthKind(authKind, personalAccessToken string) string {
	if strings.EqualFold(strings.TrimSpace(authKind), AuthKindPAT) || strings.TrimSpace(personalAccessToken) != "" {
		return AuthKindPAT
	}
	return AuthKindIDE
}

// UsesCLIRawChat reports whether the credential should use Trae CLI's raw-chat transport.
func (s *TokenStorage) UsesCLIRawChat() bool {
	return s != nil && NormalizeAuthKind(s.AuthKind, s.PersonalAccessToken) == AuthKindPAT
}

// NormalizeEdition canonicalizes a Trae edition identifier.
func NormalizeEdition(edition string) string {
	switch strings.ToLower(strings.TrimSpace(edition)) {
	case EditionSG:
		return EditionSG
	case EditionSoloCN, "solo-cn":
		return EditionSoloCN
	case EditionSoloSG, "sg-solo":
		return EditionSoloSG
	default:
		return EditionCN
	}
}

// DefaultChatBaseURL returns the edition-specific chat endpoint.
func DefaultChatBaseURL(edition string) string {
	switch NormalizeEdition(edition) {
	case EditionSG, EditionSoloSG:
		return DefaultChatBaseSG
	default:
		return DefaultChatBaseCN
	}
}

// DefaultAuthBaseURL returns the edition-specific OAuth endpoint.
func DefaultAuthBaseURL(edition string) string {
	switch NormalizeEdition(edition) {
	case EditionSG, EditionSoloSG:
		return DefaultAuthBaseSG
	default:
		return DefaultAuthBaseCN
	}
}
