// SPDX-License-Identifier: Apache-2.0

package debug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Session holds the state of an active debugger session, persisted to disk
// so that subsequent CLI invocations can reuse the session without re-authenticating.
type Session struct {
	SessionToken   string    `json:"session_token"`
	AppURL         string    `json:"app_url"`
	AuthHeader     string    `json:"auth_header"`
	RuntimeVersion string    `json:"runtime_version,omitempty"`
	ProjectID      string    `json:"project_id,omitempty"`
	StartedAt      time.Time `json:"started_at"`
}

func sessionPath(projectDir string) string {
	return filepath.Join(projectDir, ".mxcli", "debug-session.json")
}

// SaveSession writes the session state to .mxcli/debug-session.json.
func SaveSession(projectDir string, s *Session) error {
	dir := filepath.Join(projectDir, ".mxcli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .mxcli dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	path := sessionPath(projectDir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

// LoadSession reads the session state from .mxcli/debug-session.json.
func LoadSession(projectDir string) (*Session, error) {
	path := sessionPath(projectDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no active debug session (run 'mxcli debug start' first)")
		}
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session file: %w", err)
	}
	return &s, nil
}

// ClearSession removes the session file.
func ClearSession(projectDir string) error {
	path := sessionPath(projectDir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session file: %w", err)
	}
	return nil
}

// ClientFromSession creates a debugger Client from a saved session.
func ClientFromSession(s *Session) *Client {
	c := &Client{
		baseURL:    s.AppURL,
		authHeader: s.AuthHeader,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	return c
}
