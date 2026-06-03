package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

// TokenStore persists OAuth tokens between SDK invocations.
//
// Load returns (nil, nil) when no token has been stored — i.e. ErrNotFound
// is reserved for "the storage backend itself failed" and is rarely useful;
// callers should treat a nil token as "needs login".
type TokenStore interface {
	Load(ctx context.Context) (*oauth2.Token, error)
	Save(ctx context.Context, tok *oauth2.Token) error
	Clear(ctx context.Context) error
}

// ErrNoStoredToken is returned by Load if no token is in the backing store.
var ErrNoStoredToken = errors.New("auth: no stored token")

// FileTokenStore persists tokens as JSON under a file path. Mode 0600.
//
// DefaultFilePath returns the canonical location: $XDG_CONFIG_HOME/swsrs/
// credentials.json on Linux/BSD, ~/Library/Application Support/swsrs/... on
// macOS, %AppData%\swsrs\... on Windows.
type FileTokenStore struct {
	Path string
}

// DefaultFilePath returns the OS-appropriate default credentials path.
func DefaultFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("auth: locate user config dir: %w", err)
	}
	return filepath.Join(dir, "swsrs", "credentials.json"), nil
}

// path returns the effective path (s.Path or the default).
func (s *FileTokenStore) path() (string, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	return DefaultFilePath()
}

// Load reads and returns the stored token. Returns (nil, ErrNoStoredToken)
// if the file doesn't exist.
func (s *FileTokenStore) Load(ctx context.Context) (*oauth2.Token, error) {
	p, err := s.path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoStoredToken
		}
		return nil, fmt.Errorf("auth: read token file: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("auth: decode token file: %w", err)
	}
	return &tok, nil
}

// Save writes the token to disk with mode 0600. Parent directory is created
// if needed.
func (s *FileTokenStore) Save(ctx context.Context, tok *oauth2.Token) error {
	p, err := s.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("auth: mkdir token dir: %w", err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file in the same dir then rename — atomic, and never
	// leaves a half-written credentials file on disk.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".credentials-*.json")
	if err != nil {
		return fmt.Errorf("auth: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeds
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("auth: write token: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("auth: chmod token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}

// Clear deletes the stored token. Idempotent — missing file is not an error.
func (s *FileTokenStore) Clear(ctx context.Context) error {
	p, err := s.path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("auth: remove token: %w", err)
	}
	return nil
}
