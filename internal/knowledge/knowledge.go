// Package knowledge implements a knowledge vault subsystem similar to artifacts:
// user can create vaults, upload files, list vaults and files, and read files.
// Each vault has an ID, description, and stores files addressably.
package knowledge

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
)

// KnowledgeVault is one addressable knowledge vault container.
type KnowledgeVault struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Pinned    bool      `json:"pinned"`
	Description string   `json:"description"`
}

// KnowledgeFile is a single file stored inside a knowledge vault.
type KnowledgeFile struct {
	VaultID   string    `json:"vaultId"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"sizeBytes"`
	UpdatedAt time.Time `json:"updatedAt"`
	MIMEType  string    `json:"mimeType"`
}

// Store persists knowledge vaults and their files under a root directory.
type Store struct {
	root string
	mu   sync.Mutex
}

// NewStore returns a store rooted at root.
func NewStore(root string) *Store {
	return &Store{root: root}
}

func newVaultID() string {
	var b [16]byte
	return "kv_" + hex.EncodeToString(b[:])
}

func validVaultID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (s *Store) vaultDir(id string) (string, error) {
	if !validVaultID(id) {
		return "", fmt.Errorf("invalid vault id %q", id)
	}
	return filepath.Join(s.root, id), nil
}

func (s *Store) metaPath(id string) string { return filepath.Join(s.root, id+".json") }

func newVault(name, description string) *KnowledgeVault {
	now := time.Now().UTC()
	return &KnowledgeVault{
		ID:        newVaultID(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Description: description,
	}
}

func (s *Store) writeVaultMeta(id string, vault *KnowledgeVault) error {
	if _, err := s.vaultDir(id); err != nil {
		return err
	}
	data, err := json.Marshal(vault)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(id), data, 0o600)
}

// loadVault returns nil when the vault does not exist.
func (s *Store) loadVault(id string) (*KnowledgeVault, error) {
	if _, err := s.vaultDir(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var vault KnowledgeVault
	if err := json.Unmarshal(data, &vault); err != nil {
		return nil, err
	}
	return &vault, nil
}

// CreateVault creates a new knowledge vault with a given name and description.
func (s *Store) CreateVault(name, description string) (*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := newVault(name, description)
	if err := s.writeVaultMeta(vault.ID, vault); err != nil {
		return nil, err
	}
	return vault, nil
}

// ListVaults returns all vaults ordered by creation time.
func (s *Store) ListVaults() ([]*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listVaultsLocked()
}

func (s *Store) listVaultsLocked() ([]*KnowledgeVault, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var vaults []*KnowledgeVault
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || entry.IsDir() {
			continue
		}
		vault, err := s.loadVault(strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		if vault == nil {
			continue
		}
		vaults = append(vaults, vault)
	}
	sort.Slice(vaults, func(i, j int) bool { return vaults[i].CreatedAt.Before(vaults[j].CreatedAt) })
	return vaults, nil
}

// DeleteVault removes a vault and all of its files.
func (s *Store) DeleteVault(vaultID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	contentDir, err := s.vaultDir(vaultID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(contentDir); err != nil {
		return err
	}
	return os.Remove(s.metaPath(vaultID))
}

// WriteFile creates or updates a file inside a knowledge vault and refreshes
// the vault's updated-at timestamp and size.
func (s *Store) WriteFile(vaultID, name, content string) (*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, fmt.Errorf("vault %q not found", vaultID)
	}
	vaultDir, err := s.vaultDir(vaultID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		return nil, err
	}
	target := filepath.Join(vaultDir, name)
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		return nil, err
	}
	return s.finishWriteLocked(vault)
}

func (s *Store) finishWriteLocked(vault *KnowledgeVault) (*KnowledgeVault, error) {
	vault.UpdatedAt = time.Now().UTC()
	if err := s.writeVaultMeta(vault.ID, vault); err != nil {
		return nil, err
	}
	return vault, nil
}

// ReadFile returns the content of a file inside a knowledge vault.
func (s *Store) ReadFile(vaultID, name string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vaultDir, err := s.vaultDir(vaultID)
	if err != nil {
		return "", false, err
	}
	target := filepath.Join(vaultDir, name)
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

// ListFiles returns the files of one vault ordered by path.
func (s *Store) ListFiles(vaultID string) ([]*KnowledgeFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.vaultDir(vaultID)
	if err != nil {
		return nil, err
	}
	files := []*KnowledgeFile{}
	if err := filepath.WalkDir(base, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, filePath)
		if err != nil {
			return err
		}
		mimeType := mime.TypeByExtension(path.Ext(rel))
		files = append(files, &KnowledgeFile{
			VaultID:   vaultID,
			Path:      filepath.ToSlash(rel),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().UTC(),
			MIMEType:  mimeType,
		})
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// textResult creates a tool execution result with a text output.
func textResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}

// Extension exposes the knowledge vault subsystem to the agent as tools.
func Extension(store *Store) *durable.Extension {
	return harness.DefineExtension(durable.Extension{
		Name: "knowledge",
		Tools: []*durable.ToolRegistration{
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name: "list_knowledge_vaults", Description: "List all knowledge vaults",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"": map[string]any{},
					}, "required": []any{""}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					vaults, err := store.ListVaults()
					if err != nil {
						return textResult(err.Error()), nil
					}
					items := []string{}
					for _, vault := range vaults {
						items = append(items, fmt.Sprintf("%s: %s", vault.ID, vault.Description))
					}
					return textResult(strings.Join(items, "\n")), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name: "list_knowledge_files", Description: "List files in a knowledge vault",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"vaultId": map[string]any{"type": "string"},
					}, "required": []any{"vaultId"}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					vaultId, _ := input["vaultId"].(string)
					files, err := store.ListFiles(vaultId)
					if err != nil {
						return textResult(err.Error()), nil
					}
					items := []string{}
					for _, file := range files {
						items = append(items, fmt.Sprintf("%s (%d bytes)", file.Path, file.SizeBytes))
					}
					return textResult(strings.Join(items, "\n")), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name: "read_knowledge_file", Description: "Read a file from a knowledge vault",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"vaultId": map[string]any{"type": "string"}, "filePath": map[string]any{"type": "string"},
					}, "required": []any{"vaultId", "filePath"}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					vaultId, _ := input["vaultId"].(string)
					filePath, _ := input["filePath"].(string)
					content, ok, err := store.ReadFile(vaultId, filePath)
					if err != nil {
						return textResult(err.Error()), nil
					}
					if !ok {
						return textResult("File not found"), nil
					}
					return textResult(content), nil
				},
			}),
		},
	})
}