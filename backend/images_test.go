package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A 1x1 PNG.
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

func TestSafeDataPath(t *testing.T) {
	cfg := defaultConfig()
	for _, bad := range []string{"../bancada.yaml", "/etc/passwd", "images/../../x.png"} {
		if _, err := safeDataPath(cfg, bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
	if _, err := safeDataPath(cfg, "images/a/b.png"); err != nil {
		t.Fatal(err)
	}
}

func TestImageDetection(t *testing.T) {
	if !looksLikeImage("images/x/FOTO.JPG") || !looksLikeImage("https://x.com/a.png?s=1") || looksLikeImage("texto") {
		t.Fatal("looksLikeImage is wrong")
	}
	if mt, err := imageMediaType(tinyPNG); err != nil || mt != "image/png" {
		t.Fatalf("got %q %v", mt, err)
	}
	if _, err := imageMediaType([]byte("not an image")); err == nil {
		t.Fatal("text should not be an image")
	}
	cfg := defaultConfig()
	cfg.Paths.Datasets = t.TempDir()
	os.MkdirAll(filepath.Join(cfg.Paths.Datasets, "images"), 0o755)
	os.WriteFile(filepath.Join(cfg.Paths.Datasets, "images", "a.png"), tinyPNG, 0o644)
	ds := Dataset{Columns: []string{"imagem", "arquivo", "pergunta"}, Rows: []map[string]string{
		{"imagem": "images/a.png", "arquivo": "a.png", "pergunta": "o que é?"},
		{"imagem": "", "arquivo": "", "pergunta": "e isto?"},
	}}
	// "arquivo" looks like an image name but the file doesn't exist at data/a.png.
	if g := guessImageColumns(cfg, ds); len(g) != 1 || g[0] != "imagem" {
		t.Fatalf("guess = %v", g)
	}
}

// Each provider must receive the image in its own format. A fake server
// captures the request body and answers with a minimal valid response.
func TestProvidersSendImages(t *testing.T) {
	cases := []struct {
		kind     string
		response string
		want     string // fragment that must appear in the request body
	}{
		{"openai", `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`, `"url":"data:image/png;base64,`},
		{"anthropic", `{"content":[{"type":"text","text":"ok"}],"usage":{}}`, `"media_type":"image/png"`},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`, `"inlineData":{"data":"`},
		{"ollama", `{"message":{"content":"ok"}}`, `"images":["`},
	}
	for _, c := range cases {
		var body string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			io.WriteString(w, c.response)
		}))
		resp, err := callLLM(context.Background(), ProviderConfig{Kind: c.kind, BaseURL: server.URL},
			LLMRequest{Model: "m", User: "descreva", Images: []Image{{MediaType: "image/png", Data: tinyPNG}}})
		server.Close()
		if err != nil || resp.Text != "ok" {
			t.Fatalf("%s: %v %q", c.kind, err, resp.Text)
		}
		if !strings.Contains(body, c.want) {
			t.Fatalf("%s: body missing %s:\n%s", c.kind, c.want, body)
		}
		if !json.Valid([]byte(body)) {
			t.Fatalf("%s: invalid JSON", c.kind)
		}
	}
}
