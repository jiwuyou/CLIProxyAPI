package trae

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportStorageFilePlaintextInfersEdition(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "Trae Solo", "User", "globalStorage", "storage.json")
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o700); err != nil {
		t.Fatal(err)
	}
	authValue := map[string]any{
		"token":        "ide-token",
		"refreshToken": "refresh-token",
		"userId":       "user-1",
		"account": map[string]any{
			"email": "user@example.com",
		},
	}
	storage := map[string]any{storageAuthKey: authValue}
	raw, err := json.Marshal(storage)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(storagePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	credential, err := ImportStorageFile(storagePath, "")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Edition != EditionSoloSG {
		t.Fatalf("Edition = %q, want %q", credential.Edition, EditionSoloSG)
	}
	if credential.Token != "ide-token" || credential.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected tokens: %#v", credential)
	}
	if credential.Email != "user@example.com" {
		t.Fatalf("Email = %q", credential.Email)
	}
	if credential.ChatBaseURL != DefaultChatBaseSG || credential.AuthBaseURL != DefaultAuthBaseSG {
		t.Fatalf("unexpected SG endpoints: chat=%q auth=%q", credential.ChatBaseURL, credential.AuthBaseURL)
	}
}

func TestDecryptStorageValueRoundTrip(t *testing.T) {
	plain := []byte(`{"token":"encrypted-token","userId":"user-2"}`)
	for _, private := range []bool{false, true} {
		private := private
		t.Run(map[bool]string{false: "public", true: "private"}[private], func(t *testing.T) {
			encoded := encryptStorageValueForTest(t, plain, private)
			decrypted, err := DecryptStorageValue(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decrypted, plain) {
				t.Fatalf("decrypted = %q, want %q", decrypted, plain)
			}
		})
	}
}

func encryptStorageValueForTest(t *testing.T, plain []byte, private bool) string {
	t.Helper()
	randomBytes := bytes.Repeat([]byte{0x42}, 32)
	key, iv := deriveStorageKey(randomBytes, private)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512(plain)
	payload := append(append([]byte(nil), hash[:]...), plain...)
	padding := aes.BlockSize - len(payload)%aes.BlockSize
	payload = append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...)
	encrypted := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, payload)
	header := []byte{0x74, 0x63, 0x05, 0x10, 0x00, 0x00}
	if private {
		header = []byte{18, 57, 32, 32, 2, 3}
	}
	blob := append(append(append([]byte(nil), header...), randomBytes...), encrypted...)
	return base64.StdEncoding.EncodeToString(blob)
}
