package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultMaxTotalBytes is 4GB: the default global limit for unpinned
	// artifact content.
	DefaultMaxTotalBytes int64 = 4 * 1024 * 1024 * 1024
	// EnvMaxTotalBytes overrides the default limit.
	EnvMaxTotalBytes = "ARTIFACT_MAX_TOTAL_BYTES"
)

// MaxTotalBytesFromEnv parses the configured artifact size limit. It accepts
// plain byte counts and the suffixes KB, MB and GB (case-insensitive).
func MaxTotalBytesFromEnv(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultMaxTotalBytes, nil
	}
	multiplier := int64(1)
	for suffix, factor := range map[string]int64{"KB": 1024, "MB": 1024 * 1024, "GB": 1024 * 1024 * 1024} {
		if strings.HasSuffix(strings.ToUpper(value), suffix) {
			multiplier = factor
			value = strings.TrimSpace(value[:len(value)-len(suffix)])
			break
		}
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number < 0 {
		return 0, errInvalidLimit(value)
	}
	return number * multiplier, nil
}

type invalidLimitError struct{ value string }

func (e *invalidLimitError) Error() string { return "invalid artifact size limit: " + e.value }
func errInvalidLimit(value string) error   { return &invalidLimitError{value: value} }

// ConfiguredMaxTotalBytes reads ARTIFACT_MAX_TOTAL_BYTES, falling back to the
// default when unset.
func ConfiguredMaxTotalBytes() (int64, error) {
	return MaxTotalBytesFromEnv(os.Getenv(EnvMaxTotalBytes))
}

// Sweep removes unpinned artifacts, least-recently-used first, until the total
// size of unpinned artifacts is within limit. It returns the removed IDs.
func (s *Store) Sweep(ctx context.Context, limit int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweepLocked(limit)
}

func (s *Store) sweepLocked(limit int64) ([]string, error) {
	artifacts, err := s.listArtifactsLocked()
	if err != nil {
		return nil, err
	}
	candidates := make([]*Artifact, 0, len(artifacts))
	var total int64
	for _, artifact := range artifacts {
		if artifact.Pinned {
			continue
		}
		candidates = append(candidates, artifact)
		total += artifact.SizeBytes
	}
	if total <= limit {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].UpdatedAt.Before(candidates[j].UpdatedAt)
	})
	removed := []string{}
	for _, artifact := range candidates {
		if total <= limit {
			break
		}
		if err := os.RemoveAll(filepath.Join(s.root, artifact.ID)); err != nil {
			return removed, err
		}
		if err := os.Remove(s.metaPath(artifact.ID)); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		total -= artifact.SizeBytes
		removed = append(removed, artifact.ID)
	}
	return removed, nil
}

// SweepIfNeeded reads the configured limit and sweeps. It is called after
// writes and on startup so the policy is enforced without a background timer.
func (s *Store) SweepIfNeeded(ctx context.Context) ([]string, error) {
	limit, err := ConfiguredMaxTotalBytes()
	if err != nil {
		return nil, err
	}
	return s.Sweep(ctx, limit)
}

// Touch marks an artifact as recently used, refreshing its LRU timestamp.
func (s *Store) Touch(ctx context.Context, artifactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.touchLocked(artifactID)
}

func (s *Store) touchLocked(artifactID string) error {
	artifact, err := s.loadArtifact(artifactID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return nil
	}
	artifact.UpdatedAt = time.Now().UTC()
	return s.writeArtifactMeta(artifactID, artifact)
}
