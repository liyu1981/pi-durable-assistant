package attachments

import (
	"encoding/base64"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func input(name, mime string, data []byte) map[string]any {
	return map[string]any{"name": name, "mimeType": mime, "data": base64.StdEncoding.EncodeToString(data)}
}

func TestParseTextAndImage(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}, []byte("rest")...)
	files, err := Parse([]any{input("notes.md", "text/markdown", []byte("hello\n世界")), input("pic.png", "image/png", png)})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Kind != "text" || files[1].Kind != "image" {
		t.Fatalf("unexpected files: %+v", files)
	}
	if files[1].MimeType != "image/png" {
		t.Fatalf("unexpected mime: %s", files[1].MimeType)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	if _, err := Parse([]any{input("a.exe", "application/octet-stream", []byte("MZ"))}); err == nil {
		t.Fatal("unsupported type must be rejected")
	}
	if _, err := Parse([]any{input("a.png", "image/png", []byte("not png"))}); err == nil {
		t.Fatal("image signature mismatch must be rejected")
	}
	if _, err := Parse(make([]any, MaxFiles+1)); err == nil {
		t.Fatal("too many files must be rejected")
	}
}

func TestBuildAndSplitUserContent(t *testing.T) {
	content := BuildUserContent("look", []File{{Kind: "text", Name: "a.txt", MimeType: "text/plain", Size: 2, Text: "hi"}})
	blocks, ok := content.(ai.UserContentBlocks)
	if !ok {
		t.Fatalf("unexpected content type %T", content)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected two blocks, got %d", len(blocks))
	}
	text, files := SplitUserContent(blocks)
	if text != "look" || len(files) != 1 || files[0].Name != "a.txt" {
		t.Fatalf("roundtrip failed: %q %+v", text, files)
	}
}
