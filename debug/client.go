// SPDX-License-Identifier: Apache-2.0

// Package debug provides an HTTP client for the Mendix runtime debugger protocol.
package debug

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client communicates with the Mendix runtime debugger endpoint.
type Client struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
}

// NewClient creates a debugger client for the given Mendix app URL and password.
// The password is base64-encoded for the X-Debugger-Authentication header.
func NewClient(appURL, debuggerPassword string) *Client {
	return &Client{
		baseURL:    appURL,
		authHeader: base64.StdEncoding.EncodeToString([]byte(debuggerPassword)),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Response is the envelope returned by all debugger actions.
type Response struct {
	Result  json.RawMessage `json:"result"`
	Status  int             `json:"status"`
	Message string          `json:"message,omitempty"`
}

// DebuggerError represents a non-zero status response from the Mendix debugger.
type DebuggerError struct {
	Status  int
	Message string
}

func (e *DebuggerError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("debugger error (status %d): %s", e.Status, e.Message)
	}
	return fmt.Sprintf("debugger error (status %d)", e.Status)
}

// IsNotFound returns true if the error indicates the debug_id no longer exists
// (microflow execution completed).
func (e *DebuggerError) IsNotFound() bool {
	return e.Status == 2 && strings.Contains(e.Message, "Could not find")
}

// SessionResult is returned by start_session.
type SessionResult struct {
	SessionToken    string            `json:"session_token"`
	RuntimeVersion  string            `json:"runtime_version"`
	ProjectID       string            `json:"project_id"`
	PausedMicroflows []PausedMicroflow `json:"paused_microflows"`
}

// PausedMicroflow describes a microflow that is paused at a breakpoint.
type PausedMicroflow struct {
	DebugID       string              `json:"debug_id"`
	MicroflowID   string              `json:"microflow_id"`
	MicroflowName string              `json:"microflow_name"`
	ObjectID      string              `json:"object_id"`
	ObjectName    string              `json:"object_name"`
	Variables     map[string]Variable `json:"variables"`
}

// Variable represents a variable in a paused microflow's scope.
type Variable struct {
	Type            string `json:"type"`
	Value           any    `json:"value,omitempty"`
	ID              string `json:"id,omitempty"`
	State           string `json:"state,omitempty"`
	Entity          string `json:"entity,omitempty"`
	EnumerationName string `json:"enumeration_name,omitempty"`
}

// PollResult is returned by poll_events.
type PollResult struct {
	Events []Event `json:"events"`
}

// Event is a debugger event (e.g., paused_microflow).
type Event struct {
	Type string          `json:"type"`
	Data PausedMicroflow `json:"data"`
}

func (c *Client) send(action, sessionToken string, params map[string]any) (*Response, error) {
	body := map[string]any{
		"action": action,
		"params": params,
	}
	if sessionToken != "" {
		body["session_token"] = sessionToken
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/debugger/", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-Debugger-Authentication", c.authHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Mendix runtime at %s — is the app running? (%w)", c.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (HTTP 401) — check your debugger password")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d from %s: %s", resp.StatusCode, c.baseURL, string(respBody))
	}

	var result Response
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Status != 0 {
		return nil, &DebuggerError{Status: result.Status, Message: result.Message}
	}

	return &result, nil
}

// StartSession starts a new debugger session.
func (c *Client) StartSession() (*SessionResult, error) {
	resp, err := c.send("start_session", "", map[string]any{"breakpoints": []any{}})
	if err != nil {
		return nil, err
	}
	var result SessionResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse session result: %w", err)
	}
	return &result, nil
}

// StopSession stops the current debugger session.
func (c *Client) StopSession(sessionToken string) error {
	_, err := c.send("stop_session", sessionToken, map[string]any{})
	return err
}

// AddBreakpoint sets a breakpoint on a specific action within a microflow.
func (c *Client) AddBreakpoint(sessionToken, microflowName, objectID, condition string) error {
	_, err := c.send("add_breakpoint", sessionToken, map[string]any{
		"microflow_name": microflowName,
		"object_id":      objectID,
		"condition":      condition,
	})
	return err
}

// RemoveBreakpoint removes a breakpoint.
func (c *Client) RemoveBreakpoint(sessionToken, objectID string) error {
	_, err := c.send("remove_breakpoint", sessionToken, map[string]any{
		"object_id": objectID,
	})
	return err
}

// PollEvents long-polls for debugger events. The server typically holds the
// connection for ~25 seconds before returning empty results.
func (c *Client) PollEvents(sessionToken string, timeout time.Duration) (*PollResult, error) {
	saved := c.httpClient.Timeout
	c.httpClient.Timeout = timeout + 5*time.Second
	defer func() { c.httpClient.Timeout = saved }()

	resp, err := c.send("poll_events", sessionToken, map[string]any{})
	if err != nil {
		return nil, err
	}
	var result PollResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse poll result: %w", err)
	}
	return &result, nil
}

// Continue resumes a single paused microflow.
func (c *Client) Continue(sessionToken, debugID string) error {
	_, err := c.send("continue", sessionToken, map[string]any{
		"debug_id": debugID,
	})
	return err
}

// ContinueAll resumes all paused microflows.
func (c *Client) ContinueAll(sessionToken string) error {
	_, err := c.send("continue_all", sessionToken, map[string]any{})
	return err
}

// StepOver steps to the next action in the current microflow.
func (c *Client) StepOver(sessionToken, debugID string) error {
	_, err := c.send("step_over", sessionToken, map[string]any{
		"debug_id": debugID,
	})
	return err
}

// StepInto steps into a sub-microflow or loop.
func (c *Client) StepInto(sessionToken, debugID string) error {
	_, err := c.send("step_into", sessionToken, map[string]any{
		"debug_id": debugID,
	})
	return err
}

// StepOut steps out of the current sub-microflow or loop.
func (c *Client) StepOut(sessionToken, debugID string) error {
	_, err := c.send("step_out", sessionToken, map[string]any{
		"debug_id": debugID,
	})
	return err
}
