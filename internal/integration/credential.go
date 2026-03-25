package integration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiny-oc/toc/internal/config"
)

const (
	keychainService = "toc-integrations"
	keychainAccount = "master-key"
	keySize         = 32 // AES-256
)

// Credential holds the authentication tokens for an integration.
type Credential struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// GetOrCreateMasterKey retrieves the master encryption key from the OS keychain,
// or creates a new one if none exists.
func GetOrCreateMasterKey() ([]byte, error) {
	key, err := getMasterKeyFromKeychain()
	if err == nil {
		return key, nil
	}

	// Generate a new key
	key = make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}

	if err := storeMasterKeyInKeychain(key); err != nil {
		return nil, fmt.Errorf("failed to store master key in keychain: %w", err)
	}

	return key, nil
}

func getMasterKeyFromKeychain() ([]byte, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("key not found in keychain")
	}

	hexKey := strings.TrimSpace(string(out))
	return hexDecode(hexKey)
}

func storeMasterKeyInKeychain(key []byte) error {
	hexKey := hexEncode(key)
	cmd := exec.Command("security", "add-generic-password",
		"-s", keychainService,
		"-a", keychainAccount,
		"-w", hexKey,
		"-U", // update if exists
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to store in keychain: %w", err)
	}
	return nil
}

func hexEncode(data []byte) string {
	const hextable = "0123456789abcdef"
	buf := make([]byte, len(data)*2)
	for i, b := range data {
		buf[i*2] = hextable[b>>4]
		buf[i*2+1] = hextable[b&0x0f]
	}
	return string(buf)
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex string")
	}
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		high, ok1 := hexVal(s[i])
		low, ok2 := hexVal(s[i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex character")
		}
		result[i/2] = high<<4 | low
	}
	return result, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// Encrypt encrypts data using AES-256-GCM with the given key.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data using AES-256-GCM with the given key.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// StoreCredential encrypts and saves a credential for the given integration.
func StoreCredential(name string, cred *Credential) error {
	key, err := GetOrCreateMasterKey()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	encrypted, err := Encrypt(data, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	dir := filepath.Join(config.IntegrationsDir(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create integration directory: %w", err)
	}

	path := filepath.Join(dir, "credentials.enc")
	return os.WriteFile(path, encrypted, 0600)
}

// LoadCredential loads and decrypts a credential for the given integration.
func LoadCredential(name string) (*Credential, error) {
	key, err := GetOrCreateMasterKey()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(config.IntegrationsDir(), name, "credentials.enc")
	encrypted, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no credentials found for integration '%s'", name)
		}
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}

	data, err := Decrypt(encrypted, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &cred, nil
}

// LoadCredentialFromWorkspace loads a credential using an explicit workspace path.
// Used by runtime commands that run from a session dir.
func LoadCredentialFromWorkspace(workspace, name string) (*Credential, error) {
	key, err := GetOrCreateMasterKey()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(workspace, ".toc", "integrations", name, "credentials.enc")
	encrypted, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no credentials found for integration '%s'", name)
		}
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}

	data, err := Decrypt(encrypted, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &cred, nil
}

// StoreCredentialInWorkspace encrypts and saves a credential using an explicit workspace path.
func StoreCredentialInWorkspace(workspace, name string, cred *Credential) error {
	key, err := GetOrCreateMasterKey()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	encrypted, err := Encrypt(data, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	dir := filepath.Join(workspace, ".toc", "integrations", name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create integration directory: %w", err)
	}

	path := filepath.Join(dir, "credentials.enc")
	return os.WriteFile(path, encrypted, 0600)
}

// RemoveCredential deletes the credential file for an integration.
func RemoveCredential(name string) error {
	dir := filepath.Join(config.IntegrationsDir(), name)
	return os.RemoveAll(dir)
}

// CredentialExists checks if credentials are stored for the given integration.
func CredentialExists(name string) bool {
	path := filepath.Join(config.IntegrationsDir(), name, "credentials.enc")
	_, err := os.Stat(path)
	return err == nil
}

// CredentialExistsInWorkspace checks if credentials exist using an explicit workspace path.
func CredentialExistsInWorkspace(workspace, name string) bool {
	path := filepath.Join(workspace, ".toc", "integrations", name, "credentials.enc")
	_, err := os.Stat(path)
	return err == nil
}

// ListConfiguredIntegrations returns the names of integrations with stored credentials.
func ListConfiguredIntegrations() ([]string, error) {
	dir := config.IntegrationsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			credPath := filepath.Join(dir, entry.Name(), "credentials.enc")
			if _, err := os.Stat(credPath); err == nil {
				names = append(names, entry.Name())
			}
		}
	}
	return names, nil
}
