package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/config"
)

type fakeClipboard struct {
	mu      sync.Mutex
	content clipboard.Content
}

func (f *fakeClipboard) Read() (clipboard.Content, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content, nil
}
func (f *fakeClipboard) WriteText(v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = clipboard.Content{Kind: clipboard.Text, Text: v}
	return nil
}
func (f *fakeClipboard) WritePNG(v []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = clipboard.Content{Kind: clipboard.Image, PNG: append([]byte(nil), v...)}
	return nil
}

func newTestServer(t *testing.T, initial clipboard.Content) (*httptest.Server, *fakeClipboard, string) {
	t.Helper()
	token := "test-token-1234567890"
	cfg := config.Default(t.TempDir())
	cfg.Token = token
	cfg.DownloadDir = filepath.Join(t.TempDir(), "received")
	f := &fakeClipboard{content: initial}
	s, err := New(cfg, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(s.Handler()), f, token
}

func req(t *testing.T, method, url, token, contentType string, body io.Reader) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", token)
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAuthentication(t *testing.T) {
	ts, _, _ := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "hello"})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/clipboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestTextClipboardRoundTripAndLegacy(t *testing.T) {
	ts, _, token := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "Windows → iPhone"})
	defer ts.Close()
	resp := req(t, "GET", ts.URL+"/clipboard", token, "", nil)
	defer resp.Body.Close()
	var got response
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Success {
		t.Fatalf("legacy response: %#v", got)
	}
	form := "clipboard=" + strings.NewReplacer(" ", "+", "→", "%E2%86%92").Replace("iPhone → Windows")
	resp = req(t, "POST", ts.URL+"/clipboard", token, "application/x-www-form-urlencoded", strings.NewReader(form))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	resp = req(t, "GET", ts.URL+"/api/v1/clipboard", token, "", nil)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("iPhone")) {
		t.Fatalf("body=%s", b)
	}
}

func TestImageClipboardRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var p bytes.Buffer
	if err := png.Encode(&p, img); err != nil {
		t.Fatal(err)
	}
	ts, f, token := newTestServer(t, clipboard.Content{})
	defer ts.Close()
	payload := clipboardPayload{Type: "image", Data: base64.StdEncoding.EncodeToString(p.Bytes())}
	b, _ := json.Marshal(payload)
	resp := req(t, "POST", ts.URL+"/api/v1/clipboard", token, "application/json", bytes.NewReader(b))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if f.content.Kind != clipboard.Image || !bytes.Equal(f.content.PNG, p.Bytes()) {
		t.Fatal("PNG was not written")
	}
}

func TestUploadSanitizesAndDeduplicates(t *testing.T) {
	ts, _, token := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "x"})
	defer ts.Close()
	upload := func(name string) int {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		part, _ := mw.CreateFormFile("file", name)
		part.Write([]byte("payload"))
		mw.Close()
		resp := req(t, "POST", ts.URL+"/api/v1/files", token, mw.FormDataContentType(), &b)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if got := upload("../unsafe?.txt"); got != http.StatusCreated {
		t.Fatalf("first=%d", got)
	}
	if got := upload("../unsafe?.txt"); got != http.StatusCreated {
		t.Fatalf("second=%d", got)
	}
}

func TestImageUploadAlsoUpdatesClipboard(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 255})
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, img); err != nil {
		t.Fatal(err)
	}
	ts, f, token := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "before"})
	defer ts.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "photo.png")
	_, _ = part.Write(pngBody.Bytes())
	_ = mw.Close()
	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if f.content.Kind != clipboard.Image || len(f.content.PNG) == 0 {
		t.Fatal("uploaded image was not written to clipboard")
	}
}

func TestLegacyClipboardAcceptsMultipartImageInsteadOfFilename(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(1, 1, color.NRGBA{R: 30, G: 120, B: 220, A: 255})
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, img); err != nil {
		t.Fatal(err)
	}
	ts, f, token := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "before"})
	defer ts.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("clipboard", "IMG_0042.PNG")
	_, _ = part.Write(imageBody.Bytes())
	_ = mw.Close()
	resp := req(t, "POST", ts.URL+"/clipboard", token, mw.FormDataContentType(), &body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if f.content.Kind != clipboard.Image || bytes.Contains(f.content.PNG, []byte("IMG_0042.PNG")) {
		t.Fatal("multipart clipboard image was reduced to its filename")
	}
}

func TestLegacyClipboardAcceptsInlineImageData(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 1, color.NRGBA{R: 220, G: 80, A: 255})
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, img); err != nil {
		t.Fatal(err)
	}
	ts, f, token := newTestServer(t, clipboard.Content{})
	defer ts.Close()
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBody.Bytes())
	form := url.Values{"clipboard": {value}}.Encode()
	resp := req(t, "POST", ts.URL+"/clipboard", token, "application/x-www-form-urlencoded", strings.NewReader(form))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || f.content.Kind != clipboard.Image {
		t.Fatalf("inline image was not decoded: status=%d kind=%s", resp.StatusCode, f.content.Kind)
	}
}

func TestClipboardFileUsesOpaqueReference(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(file, []byte("file-body"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts, _, token := newTestServer(t, clipboard.Content{Kind: clipboard.Files, Files: []string{file}})
	defer ts.Close()
	resp := req(t, "GET", ts.URL+"/clipboard", token, "", nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if bytes.Contains(b, []byte(file)) {
		t.Fatalf("absolute path leaked: %s", b)
	}
	var envelope struct {
		Data struct {
			Data []string `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Data) != 1 {
		t.Fatalf("body=%s", b)
	}
	resp = req(t, "GET", ts.URL+"/file/"+envelope.Data.Data[0], token, "", nil)
	defer resp.Body.Close()
	downloaded, _ := io.ReadAll(resp.Body)
	if string(downloaded) != "file-body" {
		t.Fatalf("download=%q", downloaded)
	}
}
