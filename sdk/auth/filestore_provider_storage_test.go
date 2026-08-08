package auth

import (
	"os"
	"path/filepath"
	"testing"

	qoderauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/qoder"
	traeauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth/trae"
)

func TestFileTokenStoreReadAuthFileRestoresProviderStorage(t *testing.T) {
	tests := []struct {
		name       string
		fileName   string
		payload    string
		assertType func(*testing.T, any)
	}{
		{
			name:     "qoder",
			fileName: "qoder-user.json",
			payload:  `{"type":"qoder","token":"qoder-token","user_id":"user-1"}`,
			assertType: func(t *testing.T, storage any) {
				t.Helper()
				credential, ok := storage.(*qoderauth.QoderTokenStorage)
				if !ok || credential.Token != "qoder-token" {
					t.Fatalf("unexpected Qoder storage: %#v", storage)
				}
			},
		},
		{
			name:     "trae",
			fileName: "trae-user.json",
			payload:  `{"type":"trae","edition":"sg","token":"trae-token","user_id":"user-2"}`,
			assertType: func(t *testing.T, storage any) {
				t.Helper()
				credential, ok := storage.(*traeauth.TokenStorage)
				if !ok || credential.Token != "trae-token" || credential.Edition != traeauth.EditionSG {
					t.Fatalf("unexpected Trae storage: %#v", storage)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			path := filepath.Join(baseDir, test.fileName)
			if err := os.WriteFile(path, []byte(test.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			store := NewFileTokenStore()
			auth, err := store.readAuthFile(path, baseDir)
			if err != nil {
				t.Fatal(err)
			}
			test.assertType(t, auth.Storage)
		})
	}
}
