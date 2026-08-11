package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	traeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/trae"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var traeFileComponentPattern = regexp.MustCompile(`[^a-zA-Z0-9._@-]+`)

// TraeAuthenticator supports Trae CLI PAT login and legacy IDE storage import.
type TraeAuthenticator struct{}

// NewTraeAuthenticator constructs a Trae authenticator.
func NewTraeAuthenticator() Authenticator {
	return &TraeAuthenticator{}
}

func (TraeAuthenticator) Provider() string { return traeauth.Provider }

func (TraeAuthenticator) RefreshLead() *time.Duration {
	lead := time.Minute
	return &lead
}

// Login exchanges a Trae CLI PAT by default. Supplying metadata.path keeps the
// existing IDE storage.json import flow available for compatibility.
func (TraeAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("trae: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}
	path := strings.TrimSpace(opts.Metadata["path"])
	if path != "" {
		return loginTraeIDE(path, strings.TrimSpace(opts.Metadata["edition"]))
	}

	pat := strings.TrimSpace(opts.Metadata["pat"])
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("TRAECLI_PERSONAL_ACCESS_TOKEN"))
	}
	if pat == "" && opts.Prompt != nil {
		value, errPrompt := opts.Prompt("Trae CLI personal access token: ")
		if errPrompt != nil {
			return nil, fmt.Errorf("trae: read personal access token: %w", errPrompt)
		}
		pat = strings.TrimSpace(value)
	}
	if pat == "" {
		return nil, fmt.Errorf("trae: personal access token is required")
	}
	baseURL := strings.TrimSpace(opts.Metadata["base_url"])
	storage, err := traeauth.ExchangePAT(ctx, cfg, pat, baseURL)
	if err != nil {
		return nil, err
	}
	return newTraeAuthRecord(storage, "Trae CLI", "cli"), nil
}

func loginTraeIDE(path, edition string) (*coreauth.Auth, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("trae: resolve storage path: %w", err)
	}
	storage, err := traeauth.ImportStorageFile(absPath, edition)
	if err != nil {
		return nil, err
	}
	label := strings.TrimSpace(storage.Email)
	if label == "" {
		label = strings.TrimSpace(storage.Username)
	}
	if label == "" {
		label = strings.TrimSpace(storage.UserID)
	}
	return newTraeAuthRecord(storage, label, "ide"), nil
}

func newTraeAuthRecord(storage *traeauth.TokenStorage, label, fallback string) *coreauth.Auth {
	if storage == nil {
		return nil
	}
	storage.AuthKind = traeauth.NormalizeAuthKind(storage.AuthKind, storage.PersonalAccessToken)
	if strings.TrimSpace(storage.LastRefresh) == "" {
		storage.LastRefresh = time.Now().UTC().Format(time.RFC3339)
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = fmt.Sprintf("%s-%d", fallback, time.Now().UnixMilli())
	}
	fileComponent := strings.Trim(traeFileComponentPattern.ReplaceAllString(label, "-"), "-.")
	if storage.UsesCLIRawChat() {
		sum := sha256.Sum256([]byte(storage.PersonalAccessToken))
		fileComponent = fmt.Sprintf("cli-%x", sum[:6])
	}
	if fileComponent == "" {
		fileComponent = fmt.Sprintf("user-%d", time.Now().UnixMilli())
	}
	fileName := fmt.Sprintf("trae-%s.json", fileComponent)
	metadata := map[string]any{
		"type":                  traeauth.Provider,
		"auth_kind":             storage.AuthKind,
		"edition":               storage.Edition,
		"token":                 storage.Token,
		"access_token":          storage.Token,
		"personal_access_token": storage.PersonalAccessToken,
		"refresh_token":         storage.RefreshToken,
		"user_id":               storage.UserID,
		"email":                 storage.Email,
		"username":              storage.Username,
		"expired":               storage.ExpiredAt,
		"refresh_expired":       storage.RefreshExpiredAt,
		"last_refresh":          storage.LastRefresh,
		"auth_base_url":         storage.AuthBaseURL,
		"chat_base_url":         storage.ChatBaseURL,
	}
	return &coreauth.Auth{
		ID:       fileName,
		Provider: traeauth.Provider,
		FileName: fileName,
		Label:    label,
		Storage:  storage,
		Metadata: metadata,
		Attributes: map[string]string{
			"auth_kind": storage.AuthKind,
			"base_url":  storage.ChatBaseURL,
		},
	}
}
