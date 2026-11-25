// Package knowledge implements read-only, allowlisted search over an Obsidian vault.
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
)

const (
	maxResults = 12
	maxRead    = 12_000
)

// AllowedRoots returns the configured vault root and allowlisted relative folders.
func AllowedRoots(root string, allow []string) []string {
	if root == "" {
		return nil
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	if len(allow) == 0 {
		return []string{cleanRoot}
	}
	roots := make([]string, 0, len(allow))
	for _, rel := range allow {
		roots = append(roots, filepath.Join(cleanRoot, filepath.Clean(rel)))
	}
	return roots
}

func resolve(roots []string, rel string) (string, error) {
	if len(roots) == 0 {
		return "", errors.New("知识库未启用：请设置 OBSIDIAN_ROOT")
	}
	clean := filepath.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", errors.New("路径不合法")
	}
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		candidate := filepath.Join(absRoot, clean)
		relCheck, err := filepath.Rel(absRoot, candidate)
		if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("路径不在允许范围内")
}

// Match is one search hit.
type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Search walks the allowlisted roots and returns case-insensitive text hits.
func Search(roots []string, query string, includeHistorical bool) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Match{}, nil
	}
	if len([]rune(query)) > 200 {
		return nil, errors.New("查询词太长")
	}
	needle := strings.ToLower(query)
	hits := []Match{}
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || len(hits) >= maxResults {
				return nil
			}
			if entry.IsDir() {
				name := entry.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" && ext != ".txt" && ext != ".org" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 2_000_000 {
				return nil
			}
			for i, line := range strings.Split(string(data), "\n") {
				if !strings.Contains(strings.ToLower(line), needle) {
					continue
				}
				if !includeHistorical && strings.Contains(line, "归档") {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, Match{Path: filepath.ToSlash(rel), Line: i + 1, Text: strings.TrimSpace(line)})
				if len(hits) >= maxResults {
					break
				}
			}
			return nil
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Path == hits[j].Path {
			return hits[i].Line < hits[j].Line
		}
		return hits[i].Path < hits[j].Path
	})
	return hits, nil
}

// Read returns a small, line-numbered slice of an allowlisted file.
func Read(roots []string, rel string, line int) (string, error) {
	path, err := resolve(roots, rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("文件不存在或不可读")
	}
	if !utf8.Valid(data) {
		return "", errors.New("文件不是 UTF-8 文本")
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 {
		line = 1
	}
	if line > len(lines) {
		line = len(lines)
	}
	end := line + 80
	if end > len(lines) {
		end = len(lines)
	}
	out := []string{}
	for i := line - 1; i < end; i++ {
		out = append(out, fmt.Sprintf("%d\t%s", i+1, lines[i]))
	}
	result := strings.Join(out, "\n")
	if len(result) > maxRead {
		result = result[:maxRead]
	}
	return result, nil
}

// Status describes the knowledge configuration for the UI.
func Status(root string) map[string]any {
	if root == "" {
		return map[string]any{"enabled": false, "reason": "未配置 OBSIDIAN_ROOT"}
	}
	return map[string]any{"enabled": true, "rootConfigured": true}
}

// Extension exposes knowledge search/read tools to the agent.
var Extension = harness.DefineExtension(durable.Extension{
	Name: "knowledge",
	Tools: []*durable.ToolRegistration{
		harness.DefineTool(durable.ToolRegistration{
			ToolSchema: ai.ToolSchema{
				Name: "search_knowledge", Description: "在用户允许的笔记库中搜索，返回路径和行号",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"query": map[string]any{"type": "string"}, "includeHistorical": map[string]any{"type": "boolean"},
				}, "required": []any{"query"}},
			},
			Replay: durable.ReplaySafe,
			Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
				input, _ := args.(map[string]any)
				query, _ := input["query"].(string)
				historical, _ := input["includeHistorical"].(bool)
				hits, err := Search(AllowedRoots(allowedRoot(), allowedAllow()), query, historical)
				if err != nil {
					return textResult(err.Error()), nil
				}
				if len(hits) == 0 {
					return textResult("没有找到相关内容。"), nil
				}
				lines := []string{}
				for _, hit := range hits {
					lines = append(lines, fmt.Sprintf("%s:%d  %s", hit.Path, hit.Line, hit.Text))
				}
				return textResult(strings.Join(lines, "\n")), nil
			},
		}),
		harness.DefineTool(durable.ToolRegistration{
			ToolSchema: ai.ToolSchema{
				Name: "read_knowledge", Description: "读取用户笔记库中一个文件的指定行（只读）",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"path": map[string]any{"type": "string"}, "line": map[string]any{"type": "number"},
				}, "required": []any{"path"}},
			},
			Replay: durable.ReplaySafe,
			Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
				input, _ := args.(map[string]any)
				path, _ := input["path"].(string)
				line := 1
				if value, ok := input["line"].(float64); ok {
					line = int(value)
				}
				content, err := Read(AllowedRoots(allowedRoot(), allowedAllow()), path, line)
				if err != nil {
					return textResult(err.Error()), nil
				}
				return textResult(content), nil
			},
		}),
	},
})

func allowedRoot() string    { return os.Getenv("OBSIDIAN_ROOT") }
func allowedAllow() []string { return nil }

func textResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}
