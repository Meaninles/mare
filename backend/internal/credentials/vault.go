package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var credentialRefPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type StoredCredential struct {
	Ref       string    `json:"ref"`
	Provider  string    `json:"provider"`
	Secret    string    `json:"secret"`
	Hint      string    `json:"hint,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Vault struct {
	root string
}

func NewVault(root string) (*Vault, error) {
	resolvedRoot, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(resolvedRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create credential vault directory: %w", err)
	}
	return &Vault{root: resolvedRoot}, nil
}

func (vault *Vault) Root() string {
	if vault == nil {
		return ""
	}
	return vault.root
}

func (vault *Vault) Put(provider, existingRef, secret, hint string) (StoredCredential, error) {
	if vault == nil {
		return StoredCredential{}, errors.New("credential vault is not configured")
	}

	normalizedSecret := strings.TrimSpace(secret)
	if normalizedSecret == "" {
		return StoredCredential{}, errors.New("credential secret is required")
	}

	ref := strings.TrimSpace(existingRef)
	if ref == "" {
		ref = "cred_" + uuid.NewString()
	}
	if !credentialRefPattern.MatchString(ref) {
		return StoredCredential{}, fmt.Errorf("invalid credential ref: %s", ref)
	}

	now := time.Now().UTC()
	createdAt := now
	if existing, err := vault.load(ref); err == nil {
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, os.ErrNotExist) {
		return StoredCredential{}, err
	}

	record := StoredCredential{
		Ref:       ref,
		Provider:  strings.TrimSpace(provider),
		Secret:    normalizedSecret,
		Hint:      strings.TrimSpace(hint),
		CreatedAt: createdAt,
		UpdatedAt: now,
	}

	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return StoredCredential{}, fmt.Errorf("marshal credential record: %w", err)
	}

	if err := os.WriteFile(vault.filePath(ref), payload, 0o600); err != nil {
		return StoredCredential{}, fmt.Errorf("write credential record: %w", err)
	}

	return record, nil
}

func (vault *Vault) Resolve(ref string) (StoredCredential, error) {
	if vault == nil {
		return StoredCredential{}, errors.New("credential vault is not configured")
	}

	normalizedRef := strings.TrimSpace(ref)
	if normalizedRef == "" {
		return StoredCredential{}, errors.New("credential ref is required")
	}

	return vault.load(normalizedRef)
}

func (vault *Vault) Delete(ref string) error {
	if vault == nil {
		return errors.New("credential vault is not configured")
	}

	normalizedRef := strings.TrimSpace(ref)
	if normalizedRef == "" {
		return errors.New("credential ref is required")
	}
	if !credentialRefPattern.MatchString(normalizedRef) {
		return fmt.Errorf("invalid credential ref: %s", normalizedRef)
	}

	if err := os.Remove(vault.filePath(normalizedRef)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete credential record: %w", err)
	}
	return nil
}

func (vault *Vault) load(ref string) (StoredCredential, error) {
	if !credentialRefPattern.MatchString(ref) {
		return StoredCredential{}, fmt.Errorf("invalid credential ref: %s", ref)
	}

	payload, err := os.ReadFile(vault.filePath(ref))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StoredCredential{}, os.ErrNotExist
		}
		return StoredCredential{}, fmt.Errorf("read credential record: %w", err)
	}

	var record StoredCredential
	if err := json.Unmarshal(payload, &record); err != nil {
		return StoredCredential{}, fmt.Errorf("decode credential record: %w", err)
	}
	return record, nil
}

func (vault *Vault) filePath(ref string) string {
	return filepath.Join(vault.root, ref+".json")
}

func resolveRoot(root string) (string, error) {
	if trimmed := strings.TrimSpace(root); trimmed != "" {
		return filepath.Clean(trimmed), nil
	}

	if configured := strings.TrimSpace(os.Getenv("MAM_CREDENTIAL_VAULT_DIR")); configured != "" {
		return filepath.Clean(configured), nil
	}

	configRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configRoot) == "" {
		return filepath.Join(".", "data", "local-state", "credentials"), nil
	}

	return filepath.Join(configRoot, "mam", "local-state", "credentials"), nil
}
