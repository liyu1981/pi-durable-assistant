// Package attachments validates browser uploads and maps them to model content.
package attachments

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/ai"
)

const (
	MaxFiles     = 5
	MaxImageSize = 5 * 1024 * 1024
	MaxTextSize  = 200 * 1024
	MaxTotalSize = 10 * 1024 * 1024
)

var textExtensions = map[string]bool{
	"txt": true, "md": true, "markdown": true, "csv": true, "tsv": true, "json": true, "jsonl": true,
	"yaml": true, "yml": true, "xml": true, "html": true, "htm": true, "css": true, "js": true,
	"mjs": true, "cjs": true, "ts": true, "tsx": true, "jsx": true, "py": true, "go": true,
	"rs": true, "java": true, "kt": true, "swift": true, "c": true, "h": true, "cpp": true,
	"hpp": true, "cs": true, "rb": true, "php": true, "sh": true, "zsh": true, "bash": true,
	"sql": true, "toml": true, "ini": true, "conf": true, "log": true, "srt": true, "vtt": true,
}

var base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
var markPattern = regexp.MustCompile(`^<附件 name="([^"]*)" type="([^"]*)" size="(\d+)"(/?)>`)

// File is a validated upload.
type File struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Data     string `json:"data,omitempty"`
	Text     string `json:"text,omitempty"`
	Index    *int   `json:"index,omitempty"`
	URL      string `json:"url,omitempty"`
}

func cleanName(raw any) (string, error) {
	name, _ := raw.(string)
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`"<>`, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	if name == "" {
		return "", errors.New("附件缺少文件名")
	}
	return name, nil
}

func decode(data any, name string) ([]byte, string, error) {
	s, ok := data.(string)
	if !ok || s == "" || len(s)%4 != 0 || !base64Pattern.MatchString(s) {
		return nil, "", fmt.Errorf("附件「%s」不是合法的 base64 数据", name)
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, "", fmt.Errorf("附件「%s」不是合法的 base64 数据", name)
	}
	return raw, s, nil
}

func imageMatches(mime string, b []byte) bool {
	switch mime {
	case "image/png":
		return len(b) > 8 && bytes.Equal(b[:4], []byte{0x89, 'P', 'N', 'G'})
	case "image/jpeg":
		return len(b) > 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
	case "image/gif":
		return len(b) > 6 && string(b[:4]) == "GIF8"
	case "image/webp":
		return len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	}
	return false
}

// Parse validates the raw JSON attachment list.
func Parse(raw any) ([]File, error) {
	if raw == nil {
		return []File{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("attachments 必须是数组")
	}
	if len(items) > MaxFiles {
		return nil, fmt.Errorf("一次最多上传 %d 个附件", MaxFiles)
	}
	files := make([]File, 0, len(items))
	total := 0
	for _, item := range items {
		obj, _ := item.(map[string]any)
		name, err := cleanName(obj["name"])
		if err != nil {
			return nil, err
		}
		mime, _ := obj["mimeType"].(string)
		mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
		rawBytes, encoded, err := decode(obj["data"], name)
		if err != nil {
			return nil, err
		}
		total += len(rawBytes)
		if total > MaxTotalSize {
			return nil, errors.New("附件总大小超过 10MB")
		}
		if imageMatches(mime, rawBytes) {
			if len(rawBytes) > MaxImageSize {
				return nil, fmt.Errorf("图片「%s」超过 5MB", name)
			}
			files = append(files, File{Kind: "image", Name: name, MimeType: mime, Size: len(rawBytes), Data: encoded})
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		if strings.HasPrefix(mime, "text/") || textExtensions[ext] {
			if len(rawBytes) > MaxTextSize {
				return nil, fmt.Errorf("文本文件「%s」超过 200KB", name)
			}
			if !utf8.Valid(rawBytes) || bytes.Contains(rawBytes, []byte{0}) {
				return nil, fmt.Errorf("「%s」不是 UTF-8 文本", name)
			}
			textMime := mime
			if !strings.HasPrefix(mime, "text/") {
				textMime = "text/plain"
			}
			files = append(files, File{Kind: "text", Name: name, MimeType: textMime, Size: len(rawBytes), Text: string(rawBytes)})
			continue
		}
		return nil, fmt.Errorf("暂不支持「%s」这种文件，目前只支持图片（PNG/JPEG/WebP/GIF）和文本类文件", name)
	}
	return files, nil
}

// BuildUserContent creates a model user-content value from text and files.
func BuildUserContent(text string, files []File) ai.UserContent {
	if len(files) == 0 {
		return ai.UserText(text)
	}
	parts := ai.UserContentBlocks{}
	if text != "" {
		parts = append(parts, ai.TextContent{Text: text})
	}
	for _, file := range files {
		head := fmt.Sprintf(`<附件 name="%s" type="%s" size="%d"`, file.Name, file.MimeType, file.Size)
		if file.Kind == "text" {
			parts = append(parts, ai.TextContent{Text: head + ">\n" + file.Text + "\n</附件>"})
		} else {
			parts = append(parts, ai.TextContent{Text: head + "/>"}, ai.ImageContent{Data: file.Data, MimeType: file.MimeType})
		}
	}
	return parts
}

// SplitUserContent recovers plain text and attachment metadata from stored user content.
func SplitUserContent(content any) (string, []File) {
	switch value := content.(type) {
	case string:
		return value, []File{}
	case ai.UserText:
		return string(value), []File{}
	case ai.UserContentBlocks:
		texts := []string{}
		files := []File{}
		imageIndex := 0
		for _, part := range value {
			if _, ok := part.(ai.ImageContent); ok {
				if len(files) > 0 && files[len(files)-1].Kind == "image" && files[len(files)-1].Index == nil {
					files[len(files)-1].Index = &imageIndex
				}
				imageIndex++
				continue
			}
			contentBlock, ok := part.(ai.TextContent)
			if !ok {
				continue
			}
			matches := markPattern.FindStringSubmatch(contentBlock.Text)
			if matches == nil {
				texts = append(texts, contentBlock.Text)
				continue
			}
			size, _ := strconv.Atoi(matches[3])
			file := File{Name: matches[1], MimeType: matches[2], Size: size}
			if matches[4] == "/" {
				file.Kind = "image"
			} else {
				file.Kind = "text"
			}
			files = append(files, file)
		}
		return strings.Join(texts, ""), files
	default:
		return "", []File{}
	}
}

// ImageAt retrieves the image part at index from stored user content.
func ImageAt(content any, index int) *ai.ImageContent {
	parts, ok := content.(ai.UserContentBlocks)
	if !ok {
		return nil
	}
	at := 0
	for _, part := range parts {
		if image, ok := part.(ai.ImageContent); ok {
			if at == index {
				imageCopy := image
				return &imageCopy
			}
			at++
		}
	}
	return nil
}
