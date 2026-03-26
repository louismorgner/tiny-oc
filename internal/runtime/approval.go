package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type PendingApprovalRequest struct {
	ID          string            `json:"id"`
	SessionID   string            `json:"session_id"`
	Agent       string            `json:"agent"`
	Integration string            `json:"integration"`
	Action      string            `json:"action"`
	Target      string            `json:"target"`
	Params      map[string]string `json:"params,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

type PendingApprovalResponse struct {
	Decision string `json:"decision"` // allow, allow_always, deny
}

func WritePendingApproval(workspace, sessionID string, req PendingApprovalRequest) (string, error) {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}

	dir := filepath.Join(workspace, ".toc", "sessions", sessionID, "pending_approvals")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, req.ID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return "", err
	}
	return req.ID, nil
}

func WaitForPendingApproval(workspace, sessionID, approvalID string, timeout time.Duration) (*PendingApprovalResponse, error) {
	responsePath := filepath.Join(workspace, ".toc", "sessions", sessionID, "pending_approvals", approvalID+".response.json")
	deadline := time.Now().Add(timeout)

	// TODO: switch this to fsnotify when approval volume grows; polling is fine
	// for v1 but wastes wakeups when many agents wait concurrently.
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(responsePath)
		if err == nil {
			var resp PendingApprovalResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				return nil, err
			}
			return &resp, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		time.Sleep(250 * time.Millisecond)
	}

	return nil, fmt.Errorf("approval timed out after %s", timeout)
}
