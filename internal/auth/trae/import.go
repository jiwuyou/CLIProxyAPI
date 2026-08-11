package trae

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const storageAuthKey = "iCubeAuthInfo://icube.cloudide"

var (
	saltA = [64]byte{82, 9, 106, 213, 48, 54, 165, 56, 191, 64, 163, 158, 129, 243, 215, 251, 124, 227, 57, 130, 155, 47, 255, 135, 52, 142, 67, 68, 196, 222, 233, 203, 84, 123, 148, 50, 166, 194, 35, 61, 238, 76, 149, 11, 66, 250, 195, 78, 8, 46, 161, 102, 40, 217, 36, 178, 118, 91, 162, 73, 109, 139, 209, 37}
	saltB = [64]byte{31, 221, 168, 51, 136, 7, 199, 49, 177, 18, 16, 89, 39, 128, 236, 95, 96, 81, 127, 169, 25, 181, 74, 13, 45, 229, 122, 159, 147, 201, 156, 239, 160, 224, 59, 77, 174, 42, 245, 176, 200, 235, 187, 60, 131, 83, 153, 97, 23, 43, 4, 126, 186, 119, 214, 38, 225, 105, 20, 99, 85, 33, 12, 125}
	saltC = [64]byte{191, 192, 216, 250, 122, 246, 220, 97, 31, 254, 98, 27, 8, 72, 71, 176, 135, 99, 96, 18, 127, 101, 203, 104, 211, 102, 191, 125, 37, 72, 150, 156, 51, 229, 121, 35, 17, 153, 141, 177, 110, 131, 150, 128, 172, 255, 254, 6, 18, 140, 55, 62, 236, 249, 135, 64, 135, 12, 117, 4, 89, 149, 168, 209}
	saltD = [64]byte{246, 204, 26, 232, 232, 70, 129, 109, 223, 146, 169, 242, 23, 241, 105, 145, 50, 196, 165, 42, 254, 120, 3, 54, 244, 207, 209, 85, 53, 6, 138, 106, 175, 148, 31, 204, 186, 186, 165, 182, 87, 142, 49, 10, 39, 110, 26, 154, 86, 56, 173, 125, 18, 64, 198, 225, 99, 99, 83, 82, 191, 134, 76, 170}
)

// ImportStorageFile reads Trae's VS Code-style storage.json and returns a
// normalized credential. SG credentials may be plaintext; the other editions
// use Trae's tc envelope.
func ImportStorageFile(path, edition string) (*TokenStorage, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trae: storage.json path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("trae: read %s: %w", path, err)
	}

	var storage map[string]json.RawMessage
	if err = json.Unmarshal(raw, &storage); err != nil {
		return nil, fmt.Errorf("trae: parse storage.json: %w", err)
	}
	authRaw, ok := storage[storageAuthKey]
	if !ok {
		return nil, fmt.Errorf("trae: %q is missing from storage.json", storageAuthKey)
	}

	var encoded string
	if err = json.Unmarshal(authRaw, &encoded); err != nil {
		if bytes.HasPrefix(bytes.TrimSpace(authRaw), []byte("{")) {
			encoded = string(authRaw)
		} else {
			return nil, fmt.Errorf("trae: auth storage value is not a string or object")
		}
	}

	plain := []byte(strings.TrimSpace(encoded))
	if !bytes.HasPrefix(plain, []byte("{")) {
		decrypted, errDecrypt := DecryptStorageValue(encoded)
		if errDecrypt != nil {
			return nil, errDecrypt
		}
		plain = decrypted
	}
	credential, err := ParseCredentialJSON(plain, edition)
	if err != nil {
		return nil, err
	}
	if credential.Edition == "" {
		credential.Edition = InferEdition(path)
	}
	credential.Edition = NormalizeEdition(credential.Edition)
	if credential.ChatBaseURL == "" {
		credential.ChatBaseURL = DefaultChatBaseURL(credential.Edition)
	}
	if credential.AuthBaseURL == "" {
		credential.AuthBaseURL = DefaultAuthBaseURL(credential.Edition)
	}
	return credential, nil
}

// ParseCredentialJSON normalizes decrypted Trae auth JSON.
func ParseCredentialJSON(raw []byte, edition string) (*TokenStorage, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("trae: parse decrypted credential: %w", err)
	}
	account, _ := value["account"].(map[string]any)
	parsedEdition := firstString(value, "edition", "Edition")
	credential := &TokenStorage{
		Type:             Provider,
		AuthKind:         AuthKindIDE,
		Edition:          parsedEdition,
		Token:            firstString(value, "token", "Token", "accessToken", "access_token"),
		RefreshToken:     firstString(value, "refreshToken", "refresh_token", "RefreshToken"),
		UserID:           firstString(value, "userId", "user_id", "UserID", "uid"),
		Email:            firstString(value, "email", "Email"),
		Username:         firstString(value, "username", "userName", "name"),
		ExpiredAt:        firstString(value, "expiredAt", "expired", "tokenExpireAt", "TokenExpireAt"),
		RefreshExpiredAt: firstString(value, "refreshExpiredAt", "refresh_expired", "RefreshTokenExpireAt"),
		LastRefresh:      firstString(value, "lastRefresh", "last_refresh", "lastRefreshedAt"),
		AuthBaseURL:      firstString(value, "host", "authBaseUrl", "auth_base_url"),
		ChatBaseURL:      firstString(value, "chatBaseUrl", "chat_base_url"),
	}
	if strings.TrimSpace(edition) != "" {
		credential.Edition = NormalizeEdition(edition)
	} else if credential.Edition != "" {
		credential.Edition = NormalizeEdition(credential.Edition)
	}
	if account != nil {
		if credential.Email == "" {
			credential.Email = firstString(account, "email", "Email")
		}
		if credential.Username == "" {
			credential.Username = firstString(account, "username", "userName", "name")
		}
	}
	if credential.Token == "" {
		return nil, fmt.Errorf("trae: imported credential does not contain a token")
	}
	return credential, nil
}

// InferEdition derives a likely edition from the storage path.
func InferEdition(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(path, "trae solo cn"):
		return EditionSoloCN
	case strings.Contains(path, "trae solo"):
		return EditionSoloSG
	case strings.Contains(path, "trae cn"):
		return EditionCN
	default:
		return EditionSG
	}
}

// DecryptStorageValue decrypts one base64-encoded Trae tc value.
func DecryptStorageValue(encoded string) ([]byte, error) {
	blob, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("trae: decode encrypted credential: %w", err)
	}
	if len(blob) < 38+aes.BlockSize {
		return nil, fmt.Errorf("trae: encrypted credential is too short")
	}

	private := false
	switch {
	case bytes.Equal(blob[:6], []byte{0x74, 0x63, 0x05, 0x10, 0x00, 0x00}):
	case bytes.Equal(blob[:6], []byte{18, 57, 32, 32, 2, 3}):
		private = true
	default:
		return nil, fmt.Errorf("trae: unknown encrypted credential header")
	}
	randomBytes := blob[6:38]
	encrypted := blob[38:]
	if len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("trae: invalid encrypted credential length")
	}

	key, iv := deriveStorageKey(randomBytes, private)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("trae: initialize credential cipher: %w", err)
	}
	decrypted := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, encrypted)
	decrypted, err = unpadPKCS7(decrypted, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("trae: decrypt credential: %w", err)
	}
	if len(decrypted) < sha512.Size {
		return nil, fmt.Errorf("trae: decrypted credential is too short")
	}
	storedHash := decrypted[:sha512.Size]
	plain := decrypted[sha512.Size:]
	computed := sha512.Sum512(plain)
	if !bytes.Equal(storedHash, computed[:]) {
		return nil, fmt.Errorf("trae: decrypted credential hash mismatch")
	}
	return plain, nil
}

func deriveStorageKey(randomBytes []byte, private bool) ([]byte, []byte) {
	first := sha512.Sum512(randomBytes)
	salt := make([]byte, sha512.Size)
	for i := range salt {
		if private {
			salt[i] = saltC[i] ^ saltD[i]
		} else {
			salt[i] = saltA[i] ^ saltB[i]
		}
	}
	combined := make([]byte, 0, sha512.Size*2)
	combined = append(combined, first[:]...)
	combined = append(combined, salt...)
	final := sha512.Sum512(combined)
	return append([]byte(nil), final[:16]...), append([]byte(nil), final[16:32]...)
}

func unpadPKCS7(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid PKCS#7 data length")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid PKCS#7 padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("invalid PKCS#7 padding")
		}
	}
	return data[:len(data)-padding], nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case json.Number:
			return typed.String()
		case float64:
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}
