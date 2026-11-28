// Package knowledge implements the user's knowledge vault subsystem: vaults are
// addressable, described containers that hold arbitrary uploaded files, which
// the agent can list and read. Only the user's UI creates and deletes vaults;
// the agent side is read-only.
package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
)

const (
	maxNameLen        = 120
	maxDescriptionLen = 2000
	maxReadBytes      = 200_000
)

// ErrVaultNotFound is returned when a vault id does not exist.
var ErrVaultNotFound = errors.New("knowledge vault not found")

// KnowledgeVault is one addressable knowledge vault container.
type KnowledgeVault struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	FileCount   int       `json:"fileCount"`
	SizeBytes   int64     `json:"sizeBytes"`
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
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("kv_%d", time.Now().UnixNano())
	}
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

// safePath resolves a user-supplied file path inside one vault. Absolute paths,
// escaping paths and NUL/backslash separators are rejected so a request cannot
// read or write outside the vault folder.
func (s *Store) safePath(vaultID, name string) (string, error) {
	base, err := s.vaultDir(vaultID)
	if err != nil {
		return "", err
	}
	if name == "" || name == "." || strings.ContainsAny(name, "\x00\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid file path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid file path %q", name)
	}
	target := filepath.Join(base, filepath.FromSlash(clean))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file path %q", name)
	}
	return target, nil
}

// MIMEType guesses a content type from the file extension.
func MIMEType(name string) string {
	if value := mime.TypeByExtension(path.Ext(name)); value != "" {
		return value
	}
	return "application/octet-stream"
}

// IsText reports whether the bytes are valid UTF-8 and have no NUL byte, which
// is how the UI decides between a text preview and a binary preview.
func IsText(data []byte) bool {
	return utf8.Valid(data) && !strings.ContainsRune(string(data), 0)
}

func newVault(name, description string) *KnowledgeVault {
	now := time.Now().UTC()
	return &KnowledgeVault{
		ID:          newVaultID(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NormalizeFields trims and validates the user-editable vault fields.
func NormalizeFields(name, description string) (string, string, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return "", "", errors.New("name is required")
	}
	if len([]rune(name)) > maxNameLen {
		return "", "", fmt.Errorf("name must be at most %d characters", maxNameLen)
	}
	if len([]rune(description)) > maxDescriptionLen {
		return "", "", fmt.Errorf("description must be at most %d characters", maxDescriptionLen)
	}
	return name, description, nil
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

// CreateVault creates a new knowledge vault.
func (s *Store) CreateVault(ctx context.Context, name, description string) (*KnowledgeVault, error) {
	cleanName, cleanDescription, err := NormalizeFields(name, description)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := newVault(cleanName, cleanDescription)
	if err := s.writeVaultMeta(vault.ID, vault); err != nil {
		return nil, err
	}
	return vault, nil
}

// UpdateVault renames a vault and/or replaces its description.
func (s *Store) UpdateVault(ctx context.Context, vaultID, name, description string) (*KnowledgeVault, error) {
	cleanName, cleanDescription, err := NormalizeFields(name, description)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, ErrVaultNotFound
	}
	vault.Name = cleanName
	vault.Description = cleanDescription
	vault.UpdatedAt = time.Now().UTC()
	if err := s.writeVaultMeta(vaultID, vault); err != nil {
		return nil, err
	}
	return vault, nil
}

// GetVault returns one vault, or ErrVaultNotFound.
func (s *Store) GetVault(ctx context.Context, vaultID string) (*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, ErrVaultNotFound
	}
	s.fillSizeLocked(vault)
	return vault, nil
}

// ListVaults returns all vaults ordered by creation time, with file totals.
func (s *Store) ListVaults(ctx context.Context) ([]*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vaults, err := s.listVaultsLocked()
	if err != nil {
		return nil, err
	}
	for _, vault := range vaults {
		s.fillSizeLocked(vault)
	}
	return vaults, nil
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

// fillSizeLocked stores file count and byte total on the vault. The values are
// computed for display and recomputed on every read, so a missing folder is a
// zero total rather than an error.
func (s *Store) fillSizeLocked(vault *KnowledgeVault) {
	base, err := s.vaultDir(vault.ID)
	if err != nil {
		return
	}
	var count int
	var size int64
	_ = filepath.WalkDir(base, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		count++
		size += info.Size()
		return nil
	})
	vault.FileCount = count
	vault.SizeBytes = size
}

// DeleteVault removes a vault and all of its files.
func (s *Store) DeleteVault(ctx context.Context, vaultID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	contentDir, err := s.vaultDir(vaultID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(s.metaPath(vaultID)); err != nil {
		if os.IsNotExist(err) {
			return ErrVaultNotFound
		}
		return err
	}
	if err := os.RemoveAll(contentDir); err != nil {
		return err
	}
	return os.Remove(s.metaPath(vaultID))
}

// WriteFile creates or updates a file inside a vault, creating parent folders
// for nested paths, and refreshes the vault's updated-at timestamp.
func (s *Store) WriteFile(ctx context.Context, vaultID, name string, content []byte) (*KnowledgeVault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, ErrVaultNotFound
	}
	target, err := s.safePath(vaultID, name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return nil, err
	}
	return s.finishWriteLocked(vault)
}

func (s *Store) finishWriteLocked(vault *KnowledgeVault) (*KnowledgeVault, error) {
	vault.UpdatedAt = time.Now().UTC()
	if err := s.writeVaultMeta(vault.ID, vault); err != nil {
		return nil, err
	}
	s.fillSizeLocked(vault)
	return vault, nil
}

// ReadFile returns the bytes of one file. ok is false when the file is missing.
func (s *Store) ReadFile(ctx context.Context, vaultID, name string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := s.safePath(vaultID, name)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// DeleteFile removes one file from a vault.
func (s *Store) DeleteFile(ctx context.Context, vaultID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return err
	}
	if vault == nil {
		return ErrVaultNotFound
	}
	target, err := s.safePath(vaultID, name)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file %q not found", name)
		}
		return err
	}
	_, err = s.finishWriteLocked(vault)
	return err
}

// ListFiles returns the files of one vault ordered by path.
func (s *Store) ListFiles(ctx context.Context, vaultID string) ([]*KnowledgeFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listFilesLocked(vaultID)
}

func (s *Store) listFilesLocked(vaultID string) ([]*KnowledgeFile, error) {
	vault, err := s.loadVault(vaultID)
	if err != nil {
		return nil, err
	}
	if vault == nil {
		return nil, ErrVaultNotFound
	}
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
		files = append(files, &KnowledgeFile{
			VaultID:   vaultID,
			Path:      filepath.ToSlash(rel),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().UTC(),
			MIMEType:  MIMEType(rel),
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

func textResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}

// Extension exposes the knowledge vaults to the agent as read-only tools.
func Extension(store *Store) *durable.Extension {
	return harness.DefineExtension(durable.Extension{
		Name: "knowledge",
		Tools: []*durable.ToolRegistration{
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "list_knowledge_vaults",
					Description: "List the user's knowledge vaults with their id, name, description and file count",
					Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					vaults, err := store.ListVaults(ctx)
					if err != nil {
						return textResult(err.Error()), nil
					}
					if len(vaults) == 0 {
						return textResult("No knowledge vaults exist yet."), nil
					}
					lines := make([]string, 0, len(vaults))
					for _, vault := range vaults {
						description := vault.Description
						if description == "" {
							description = "(no description)"
						}
						lines = append(lines, fmt.Sprintf("%s  %s  files=%d  %s", vault.ID, vault.Name, vault.FileCount, description))
					}
					return textResult(strings.Join(lines, "\n")), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "list_knowledge_files",
					Description: "List the files inside one knowledge vault by vault id",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"vaultId": map[string]any{"type": "string", "description": "Knowledge vault id, for example kv_ab12"},
					}, "required": []any{"vaultId"}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					vaultID, _ := input["vaultId"].(string)
					if vaultID == "" {
						return textResult("vaultId is required"), nil
					}
					files, err := store.ListFiles(ctx, vaultID)
					if err != nil {
						return textResult(err.Error()), nil
					}
					if len(files) == 0 {
						return textResult("This vault has no files."), nil
					}
					lines := make([]string, 0, len(files))
					for _, file := range files {
						lines = append(lines, fmt.Sprintf("%s  %d bytes  %s", file.Path, file.SizeBytes, file.MIMEType))
					}
					return textResult(strings.Join(lines, "\n")), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "read_knowledge_file",
					Description: "Read one text file from a knowledge vault by vault id and file path",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"vaultId":  map[string]any{"type": "string", "description": "Knowledge vault id"},
						"filePath": map[string]any{"type": "string", "description": "Path inside the vault, for example notes/todo.md"},
					}, "required": []any{"vaultId", "filePath"}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					vaultID, _ := input["vaultId"].(string)
					filePath, _ := input["filePath"].(string)
					if vaultID == "" || filePath == "" {
						return textResult("vaultId and filePath are required"), nil
					}
					data, ok, err := store.ReadFile(ctx, vaultID, filePath)
					if err != nil {
						return textResult(err.Error()), nil
					}
					if !ok {
						return textResult("File not found."), nil
					}
					if !IsText(data) {
						return textResult(fmt.Sprintf("%s is a binary file (%s, %d bytes) and cannot be read as text.",
							filePath, MIMEType(filePath), len(data))), nil
					}
					if len(data) > maxReadBytes {
						return textResult(string(data[:maxReadBytes]) + "\n\n[truncated]"), nil
					}
					return textResult(string(data)), nil
				},
			}),
		},
	})
}
