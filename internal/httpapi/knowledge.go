package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/liyu1981/pi-durable-assistant/internal/knowledge"
)

// maxKnowledgeUpload bounds one uploaded file. Base64 encoding inflates the
// request body by roughly a third, so the JSON limit is larger than the limit
// applied to the decoded bytes.
const (
	maxKnowledgeUploadBody = 24 << 20
	maxKnowledgeUploadSize = 16 << 20
)

func knowledgeVaultView(vault *knowledge.KnowledgeVault) map[string]any {
	return map[string]any{
		"id":          vault.ID,
		"name":        vault.Name,
		"description": vault.Description,
		"createdAt":   vault.CreatedAt,
		"updatedAt":   vault.UpdatedAt,
		"fileCount":   vault.FileCount,
		"sizeBytes":   vault.SizeBytes,
	}
}

func knowledgeFileView(file *knowledge.KnowledgeFile) map[string]any {
	return map[string]any{
		"vaultId":   file.VaultID,
		"path":      file.Path,
		"sizeBytes": file.SizeBytes,
		"updatedAt": file.UpdatedAt,
		"mimeType":  file.MIMEType,
	}
}

// writeKnowledgeError maps store errors onto HTTP responses.
func writeKnowledgeError(w http.ResponseWriter, err error) {
	if errors.Is(err, knowledge.ErrVaultNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

// listKnowledgeVaults returns every vault with its file totals.
func (s *Server) listKnowledgeVaults(w http.ResponseWriter, r *http.Request) {
	vaults, err := s.Runtime.Knowledge.ListVaults(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(vaults))
	for _, vault := range vaults {
		items = append(items, knowledgeVaultView(vault))
	}
	writeJSON(w, http.StatusOK, map[string]any{"vaults": items})
}

// createKnowledgeVault creates a vault from a name and description.
func (s *Server) createKnowledgeVault(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	name, _ := body["name"].(string)
	description, _ := body["description"].(string)
	vault, err := s.Runtime.Knowledge.CreateVault(r.Context(), name, description)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"vault": knowledgeVaultView(vault)})
}

// updateKnowledgeVault renames a vault and/or replaces its description.
func (s *Server) updateKnowledgeVault(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	name, _ := body["name"].(string)
	description, _ := body["description"].(string)
	vault, err := s.Runtime.Knowledge.UpdateVault(r.Context(), r.PathValue("id"), name, description)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vault": knowledgeVaultView(vault)})
}

// deleteKnowledgeVault removes a vault together with all of its files.
func (s *Server) deleteKnowledgeVault(w http.ResponseWriter, r *http.Request) {
	if err := s.Runtime.Knowledge.DeleteVault(r.Context(), r.PathValue("id")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// listKnowledgeFiles lists the files of one vault.
func (s *Server) listKnowledgeFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.Runtime.Knowledge.ListFiles(r.Context(), r.PathValue("id"))
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(files))
	for _, file := range files {
		items = append(items, knowledgeFileView(file))
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": items})
}

// readKnowledgeFile returns one file as text and/or base64 so the browser can
// preview both text and binary uploads.
func (s *Server) readKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("path")
	data, ok, err := s.Runtime.Knowledge.ReadFile(r.Context(), id, name)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "knowledge file not found")
		return
	}
	payload := map[string]any{
		"vaultId": id, "path": name, "sizeBytes": len(data),
		"mimeType": knowledge.MIMEType(name), "isText": knowledge.IsText(data),
	}
	if knowledge.IsText(data) {
		payload["content"] = string(data)
	} else {
		payload["contentBase64"] = base64.StdEncoding.EncodeToString(data)
	}
	writeJSON(w, http.StatusOK, payload)
}

// writeKnowledgeFile stores one uploaded file. The body carries either `content`
// (UTF-8 text) or `base64` (arbitrary bytes).
func (s *Server) writeKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, maxKnowledgeUploadBody)
	if err != nil {
		handleError(w, err)
		return
	}
	var data []byte
	switch {
	case body["base64"] != nil:
		encoded, ok := body["base64"].(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "base64 must be a string")
			return
		}
		data, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			writeError(w, http.StatusBadRequest, "base64 content is not valid")
			return
		}
	case body["content"] != nil:
		text, ok := body["content"].(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "content must be a string")
			return
		}
		data = []byte(text)
	default:
		writeError(w, http.StatusBadRequest, "content or base64 is required")
		return
	}
	if len(data) > maxKnowledgeUploadSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file is too large")
		return
	}
	vault, err := s.Runtime.Knowledge.WriteFile(r.Context(), r.PathValue("id"), r.PathValue("path"), data)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vault": knowledgeVaultView(vault)})
}

// deleteKnowledgeFile removes one file from a vault.
func (s *Server) deleteKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	if err := s.Runtime.Knowledge.DeleteFile(r.Context(), r.PathValue("id"), r.PathValue("path")); err != nil {
		writeKnowledgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// rawKnowledgeFile serves the stored bytes for browser preview.
func (s *Server) rawKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	data, ok, err := s.Runtime.Knowledge.ReadFile(r.Context(), r.PathValue("id"), name)
	if err != nil {
		writeKnowledgeError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "knowledge file not found")
		return
	}
	contentType := artifactContentType(name)
	// Uploaded HTML must never run in this origin: the bytes are user content.
	if strings.HasPrefix(contentType, "text/html") {
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("content-type", contentType)
	w.Header().Set("content-disposition", "inline; filename=\""+safeFilename(name)+"\"")
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("x-content-type-options", "nosniff")
	w.Write(data)
}
