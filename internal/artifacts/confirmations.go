package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Confirmation is a pending human-in-the-loop request from the agent to pin
// or unpin an artifact.
type Confirmation struct {
	ID         string     `json:"id"`
	ArtifactID string     `json:"artifactId"`
	Action     string     `json:"action"`
	Context    string     `json:"context"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

// ConfirmationStore persists pending confirmation requests in a dedicated
// store, separate from the durable task graph.
type ConfirmationStore struct {
	path string
	mu   sync.Mutex
}

// NewConfirmationStore returns a confirmation store at path.
func NewConfirmationStore(path string) *ConfirmationStore {
	return &ConfirmationStore{path: path}
}

func (c *ConfirmationStore) loadLocked() ([]*Confirmation, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Confirmation{}, nil
		}
		return nil, err
	}
	var items []*Confirmation
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *ConfirmationStore) saveLocked(items []*Confirmation) error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o600)
}

// Propose records a new pending confirmation request.
func (c *ConfirmationStore) Propose(ctx context.Context, artifactID, action, contextText string) (*Confirmation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if action != "pin" && action != "unpin" {
		return nil, fmt.Errorf("invalid confirmation action %q", action)
	}
	items, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ArtifactID == artifactID && item.Action == action && item.Status == "pending" {
			return item, nil
		}
	}
	item := &Confirmation{
		ID:         newArtifactID(),
		ArtifactID: artifactID,
		Action:     action,
		Context:    contextText,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
	items = append(items, item)
	if err := c.saveLocked(items); err != nil {
		return nil, err
	}
	return item, nil
}

// List returns all confirmations, newest first.
func (c *ConfirmationStore) List(ctx context.Context) ([]*Confirmation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	items, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// Get returns one confirmation by id.
func (c *ConfirmationStore) Get(ctx context.Context, id string) (*Confirmation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	items, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

// Resolve approves or rejects a pending confirmation.
func (c *ConfirmationStore) Resolve(ctx context.Context, id, status string) (*Confirmation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if status != "approved" && status != "rejected" {
		return nil, fmt.Errorf("invalid confirmation status %q", status)
	}
	items, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.Status != "pending" {
			return nil, errors.New("confirmation already resolved")
		}
		item.Status = status
		now := time.Now().UTC()
		item.ResolvedAt = &now
		if err := c.saveLocked(items); err != nil {
			return nil, err
		}
		return item, nil
	}
	return nil, nil
}
