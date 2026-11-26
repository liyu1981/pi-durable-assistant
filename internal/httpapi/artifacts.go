package httpapi

import (
	"context"
	"net/http"

	"github.com/liyu1981/pi-durable-assistant/internal/artifacts"
)

func artifactView(artifact *artifacts.Artifact) map[string]any {
	return map[string]any{
		"id":        artifact.ID,
		"createdAt": artifact.CreatedAt,
		"updatedAt": artifact.UpdatedAt,
		"pinned":    artifact.Pinned,
		"sizeBytes": artifact.SizeBytes,
	}
}

// listArtifacts returns all artifacts.
func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	artifacts, err := s.Runtime.Artifacts.ListArtifacts(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		items = append(items, artifactView(artifact))
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": items})
}

// createArtifact creates a new artifact.
func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	created, err := s.Runtime.Artifacts.CreateArtifact(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"artifact": artifactView(created)})
}

// getArtifact returns one artifact by id.
func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	artifact, err := s.findArtifact(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}
	if artifact == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact": artifactView(artifact)})
}

// deleteArtifact removes one artifact and its files.
func (s *Server) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	if err := s.Runtime.Artifacts.DeleteArtifact(r.Context(), r.PathValue("id")); err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// pinArtifact flags an artifact as pinned.
func (s *Server) pinArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.findArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	if artifact == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err := s.Runtime.Artifacts.SetPinned(r.Context(), artifact.ID, true); err != nil {
		handleError(w, err)
		return
	}
	updated, err := s.findArtifact(r.Context(), artifact.ID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact": artifactView(updated)})
}

// unpinArtifact clears the pinned flag of an artifact.
func (s *Server) unpinArtifact(w http.ResponseWriter, r *http.Request) {
	artifact, err := s.findArtifact(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	if artifact == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err := s.Runtime.Artifacts.SetPinned(r.Context(), artifact.ID, false); err != nil {
		handleError(w, err)
		return
	}
	updated, err := s.findArtifact(r.Context(), artifact.ID)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact": artifactView(updated)})
}

// listArtifactFiles lists the files in one artifact.
func (s *Server) listArtifactFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.Runtime.Artifacts.ListFiles(r.Context(), r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(files))
	for _, file := range files {
		items = append(items, map[string]any{
			"path": file.Path, "sizeBytes": file.SizeBytes, "updatedAt": file.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": items})
}

// readArtifactFile reads one file from an artifact by path.
func (s *Server) readArtifactFile(w http.ResponseWriter, r *http.Request) {
	data, ok, err := s.Runtime.Artifacts.ReadFile(r.Context(), r.PathValue("id"), r.PathValue("path"))
	if err != nil {
		handleError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "artifact file not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": string(data)})
}

// writeArtifactFile creates or updates one file in an artifact by path.
func (s *Server) writeArtifactFile(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 4*1024*1024)
	if err != nil {
		handleError(w, err)
		return
	}
	content, ok := body["content"].(string)
	if !ok {
		writeError(w, http.StatusBadRequest, "content must be a string")
		return
	}
	artifact, err := s.Runtime.Artifacts.WriteFile(r.Context(), r.PathValue("id"), r.PathValue("path"), []byte(content))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact": artifactView(artifact)})
}

// listArtifactConfirmations returns pending and resolved pin/unpin requests.
func (s *Server) listArtifactConfirmations(w http.ResponseWriter, r *http.Request) {
	items, err := s.Runtime.Confirmations.List(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmations": items})
}

// decideArtifactConfirmation approves or rejects a pending agent request.
// On approval the pin/unpin action is applied to the artifact.
func (s *Server) decideArtifactConfirmation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	decision := r.PathValue("decision")
	status := ""
	switch decision {
	case "approve":
		status = "approved"
	case "reject":
		status = "rejected"
	default:
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	item, err := s.Runtime.Confirmations.Resolve(r.Context(), id, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "confirmation not found")
		return
	}
	if status == "approved" {
		if err := s.Runtime.Artifacts.SetPinned(r.Context(), item.ArtifactID, item.Action == "pin"); err != nil {
			handleError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmation": item})
}

func (s *Server) findArtifact(ctx context.Context, id string) (*artifacts.Artifact, error) {
	artifacts, err := s.Runtime.Artifacts.ListArtifacts(ctx)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return artifact, nil
		}
	}
	return nil, nil
}
