package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	codebuddyauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codebuddy"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

type codeBuddyOAuthServiceDouble struct {
	authState *codebuddyauth.AuthState
	storage   *codebuddyauth.CodeBuddyTokenStorage
}

func (s *codeBuddyOAuthServiceDouble) FetchAuthState(context.Context) (*codebuddyauth.AuthState, error) {
	return s.authState, nil
}

func (s *codeBuddyOAuthServiceDouble) PollForToken(context.Context, string) (*codebuddyauth.CodeBuddyTokenStorage, error) {
	return s.storage, nil
}

func TestRequestCodeBuddyTokenStartsAndCompletesSession(t *testing.T) {
	store := newOAuthSessionStore(time.Minute)
	replaceOAuthSessionStoreForTest(t, store)

	handler := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	memoryStore := &memoryAuthStore{}
	handler.tokenStore = memoryStore
	persisted := make(chan struct{}, 1)
	handler.SetPostAuthPersistHook(func(context.Context, *coreauth.Auth) error {
		persisted <- struct{}{}
		return nil
	})

	service := &codeBuddyOAuthServiceDouble{
		authState: &codebuddyauth.AuthState{
			State:   "codebuddy-state",
			AuthURL: "https://www.codebuddy.cn/login/?platform=CLI&state=codebuddy-state",
		},
		storage: &codebuddyauth.CodeBuddyTokenStorage{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			UserID:       "user-123",
			Domain:       "www.codebuddy.cn",
			ExpiresIn:    3600,
			Type:         "codebuddy",
		},
	}

	router := gin.New()
	router.GET("/codebuddy-auth-url", func(c *gin.Context) {
		handler.requestCodeBuddyToken(c, service)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/codebuddy-auth-url", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Status string `json:"status"`
		URL    string `json:"url"`
		State  string `json:"state"`
	}
	if errDecode := json.Unmarshal(recorder.Body.Bytes(), &response); errDecode != nil {
		t.Fatalf("decode start response: %v", errDecode)
	}
	if response.Status != "ok" || response.State != "codebuddy-state" || response.URL != service.authState.AuthURL {
		t.Fatalf("unexpected start response: %#v", response)
	}

	select {
	case <-persisted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CodeBuddy credential persistence")
	}

	items, errList := memoryStore.List(context.Background())
	if errList != nil {
		t.Fatalf("list saved credentials: %v", errList)
	}
	if len(items) != 1 || items[0].Provider != "codebuddy" || items[0].ID != "codebuddy-user-123.json" {
		t.Fatalf("unexpected saved credentials: %#v", items)
	}
	_, _, _, _, completed, ok := GetOAuthSessionDetails("codebuddy-state")
	if !ok || !completed {
		t.Fatalf("CodeBuddy session completed/ok = %t/%t, want true/true", completed, ok)
	}
}
