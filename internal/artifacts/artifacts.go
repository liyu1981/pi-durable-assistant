// Package artifacts implements the durable file subsystem used by the agent:
// artifacts are addressable containers that hold files which can be created,
// read, updated, moved, pinned and cleaned up by policy.
package artifacts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Artifact is one addressable file container managed by the subsystem.
type Artifact struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Pinned    bool      `json:"pinned"`
	SizeBytes int64     `json:"sizeBytes"`
}

// ArtifactFile is a single file stored inside an artifact.
type ArtifactFile struct {
	ArtifactID string    `json:"artifactId"`
	Path       string    `json:"path"`
	Content    []byte    `json:"content"`
	SizeBytes  int64     `json:"sizeBytes"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Store persists artifacts and their files under a root directory.
type Store struct {
	root string
	mu   sync.Mutex
}

// NewStore returns a store rooted at root.
func NewStore(root string) *Store {
	return &Store{root: root}
}

func newArtifactID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("art_%d", time.Now().UnixNano())
	}
	return "art_" + hex.EncodeToString(b[:])
}

func validArtifactID(id string) bool {
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

// artifactDir validates an artifact ID and returns its content directory.
func (s *Store) artifactDir(id string) (string, error) {
	if !validArtifactID(id) {
		return "", fmt.Errorf("invalid artifact id %q", id)
	}
	return filepath.Join(s.root, id), nil
}

// safePath resolves a user-supplied file path inside one artifact. It rejects
// absolute paths, escaping paths and separators that could leave the folder.
func (s *Store) safePath(id, name string) (string, error) {
	base, err := s.artifactDir(id)
	if err != nil {
		return "", err
	}
	if name == "" || name == "." || strings.ContainsAny(name, "\x00\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid artifact path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid artifact path %q", name)
	}
	target := filepath.Join(base, filepath.FromSlash(clean))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid artifact path %q", name)
	}
	return target, nil
}

func (s *Store) metaPath(id string) string { return filepath.Join(s.root, id+".json") }

func (s *Store) CreateArtifact(ctx context.Context) (*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newArtifactID()
	now := time.Now().UTC()
	artifact := &Artifact{ID: id, CreatedAt: now, UpdatedAt: now}
	if err := s.writeArtifactMeta(id, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func (s *Store) writeArtifactMeta(id string, artifact *Artifact) error {
	if _, err := s.artifactDir(id); err != nil {
		return err
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(id), data, 0o600)
}

// loadArtifact returns nil when the artifact does not exist.
func (s *Store) loadArtifact(id string) (*Artifact, error) {
	if _, err := s.artifactDir(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

// WriteFile creates or updates a file inside an artifact and refreshes the
// artifact's updated-at timestamp and size.
func (s *Store) WriteFile(ctx context.Context, artifactID, name string, content []byte) (*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, err := s.loadArtifact(artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact %q not found", artifactID)
	}
	target, err := s.safePath(artifactID, name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return nil, err
	}
	return s.finishWriteLocked(artifact)
}

// MoveFile renames a file inside one artifact. An existing destination is
// overwritten. Both paths must stay inside the artifact folder.
func (s *Store) MoveFile(ctx context.Context, artifactID, from, to string) (*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, err := s.loadArtifact(artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact %q not found", artifactID)
	}
	source, err := s.safePath(artifactID, from)
	if err != nil {
		return nil, err
	}
	target, err := s.safePath(artifactID, to)
	if err != nil {
		return nil, err
	}
	if source == target {
		return artifact, nil
	}
	if _, err := os.Stat(source); err != nil {
		return nil, fmt.Errorf("source file %q not found: %w", from, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return nil, err
	}
	if err := os.Rename(source, target); err != nil {
		return nil, err
	}
	return s.finishWriteLocked(artifact)
}

func (s *Store) finishWriteLocked(artifact *Artifact) (*Artifact, error) {
	artifact.UpdatedAt = time.Now().UTC()
	artifact.SizeBytes = s.contentSize(artifact.ID)
	if err := s.writeArtifactMeta(artifact.ID, artifact); err != nil {
		return nil, err
	}
	limit, err := ConfiguredMaxTotalBytes()
	if err != nil {
		return nil, err
	}
	if _, err := s.sweepLocked(limit); err != nil {
		return nil, err
	}
	return artifact, nil
}

// ReadFile returns the content of a file inside an artifact. ok is false when
// the artifact or file does not exist.
func (s *Store) ReadFile(ctx context.Context, artifactID, name string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, err := s.safePath(artifactID, name)
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
	_ = s.touchLocked(artifactID)
	return data, true, nil
}

// DeleteArtifact removes an artifact and all of its files.
func (s *Store) DeleteArtifact(ctx context.Context, artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	contentDir, err := s.artifactDir(artifactID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(contentDir); err != nil {
		return err
	}
	return os.Remove(s.metaPath(artifactID))
}

// ListArtifacts returns all artifacts ordered by creation time.
func (s *Store) ListArtifacts(ctx context.Context) ([]*Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listArtifactsLocked()
}

func (s *Store) listArtifactsLocked() ([]*Artifact, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var artifacts []*Artifact
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || entry.IsDir() {
			continue
		}
		artifact, err := s.loadArtifact(strings.TrimSuffix(name, ".json"))
		if err != nil {
			return nil, err
		}
		if artifact == nil {
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].CreatedAt.Before(artifacts[j].CreatedAt) })
	return artifacts, nil
}

// ListFiles returns the files of one artifact ordered by path.
func (s *Store) ListFiles(ctx context.Context, artifactID string) ([]*ArtifactFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.artifactDir(artifactID)
	if err != nil {
		return nil, err
	}
	files := []*ArtifactFile{}
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
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, filePath)
		if err != nil {
			return err
		}
		files = append(files, &ArtifactFile{
			ArtifactID: artifactID,
			Path:       filepath.ToSlash(rel),
			Content:    content,
			SizeBytes:  info.Size(),
			UpdatedAt:  info.ModTime().UTC(),
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

// SetPinned flags an artifact as pinned or unpinned.
func (s *Store) SetPinned(ctx context.Context, artifactID string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, err := s.loadArtifact(artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return fmt.Errorf("artifact %q not found", artifactID)
	}
	artifact.Pinned = pinned
	artifact.UpdatedAt = time.Now().UTC()
	return s.writeArtifactMeta(artifactID, artifact)
}

// Size returns the total size of the files inside one artifact.
func (s *Store) Size(ctx context.Context, artifactID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.artifactDir(artifactID)
	if err != nil {
		return 0, err
	}
	return contentSize(base), nil
}

// Total returns the sum of the sizes of all artifacts. The LRU cleanup policy
// only considers unpinned artifacts and excludes pinned ones from the limit.
func (s *Store) Total(ctx context.Context) (int64, error) {
	artifacts, err := s.ListArtifacts(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, artifact := range artifacts {
		if artifact.Pinned {
			continue
		}
		total += artifact.SizeBytes
	}
	return total, nil
}

func (s *Store) contentSize(artifactID string) int64 {
	base, err := s.artifactDir(artifactID)
	if err != nil {
		return 0
	}
	return contentSize(base)
}

func contentSize(base string) int64 {
	var size int64
	_ = filepath.WalkDir(base, func(filePath string, entry os.DirEntry, err error) error {
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
		size += info.Size()
		return nil
	})
	return size
}
