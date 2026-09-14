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

func newTestServerWithDir(t *testing.T, initial clipboard.Content) (*httptest.Server, *fakeClipboard, string, string) {
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
	return httptest.NewServer(s.Handler()), f, token, cfg.DownloadDir
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

// TestUploadFileContentSavedOnDisk guards the "only the image name is
// received, never the content" bug: when the iPhone sends a real binary
// image, the saved file on disk must contain those exact bytes — not 0
// bytes and not just the filename.
func TestUploadFileContentSavedOnDisk(t *testing.T) {
	ts, _, token, dlDir := newTestServerWithDir(t, clipboard.Content{})
	defer ts.Close()
	img := image.NewNRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8((x + y) / 2), A: 255})
		}
	}
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, img); err != nil {
		t.Fatal(err)
	}
	if pngBody.Len() < 1000 {
		t.Fatalf("expected non-trivial PNG, got %d bytes", pngBody.Len())
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "IMG_0042.PNG")
	_, _ = part.Write(pngBody.Bytes())
	_ = mw.Close()

	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, respBody)
	}

	files, err := os.ReadDir(dlDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file on disk, got %d", len(files))
	}
	got, err := os.ReadFile(filepath.Join(dlDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pngBody.Bytes()) {
		t.Fatalf("file content mismatch: got %d bytes, expected %d bytes", len(got), pngBody.Len())
	}
	if !bytes.HasPrefix(got, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("file does not start with PNG magic: %x...", got[:8])
	}
}

// TestUploadEmptyPartIsRejected ensures that an iOS Shortcut quirk — a
// multipart part with a filename but an empty body — does not silently
// produce a 0-byte file under the expected name.
func TestUploadEmptyPartIsRejected(t *testing.T) {
	ts, _, token, dlDir := newTestServerWithDir(t, clipboard.Content{})
	defer ts.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_, _ = mw.CreateFormFile("file", "IMG_EMPTY.PNG")
	_ = mw.Close() // part body stays empty

	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("empty upload should be rejected; got status=%d body=%s", resp.StatusCode, respBody)
	}
	if !bytes.Contains(respBody, []byte("empty")) {
		t.Fatalf("error message should mention the empty file; got %s", respBody)
	}
	if files, _ := os.ReadDir(dlDir); len(files) != 0 {
		for _, f := range files {
			info, _ := f.Info()
			t.Fatalf("server left %s (%d bytes) on disk after rejecting an empty upload", f.Name(), info.Size())
		}
	}
}

// TestUploadFilenameOnlyBodyIsRejected rejects uploads whose body is just
// the original filename string (another iOS Shortcut quirk) — saving that
// would otherwise leave a misleading file whose content equals its name.
func TestUploadFilenameOnlyBodyIsRejected(t *testing.T) {
	ts, _, token, dlDir := newTestServerWithDir(t, clipboard.Content{})
	defer ts.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, _ := mw.CreateFormFile("file", "IMG_0042.PNG")
	_, _ = p.Write([]byte("IMG_0042.PNG"))
	_ = mw.Close()

	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("filename-as-body upload should be rejected; got status=%d body=%s", resp.StatusCode, respBody)
	}
	if files, _ := os.ReadDir(dlDir); len(files) != 0 {
		for _, f := range files {
			info, _ := f.Info()
			t.Fatalf("server left %s (%d bytes) on disk after rejecting a filename-as-body upload", f.Name(), info.Size())
		}
	}
}

// TestUploadSmallTextPayloadStillAccepted makes sure the rejection above
// does not catch legitimate small text payloads (e.g. a one-line note).
func TestUploadSmallTextPayloadStillAccepted(t *testing.T) {
	ts, _, token, dlDir := newTestServerWithDir(t, clipboard.Content{})
	defer ts.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, _ := mw.CreateFormFile("file", "note.txt")
	_, _ = p.Write([]byte("hello from the iPhone"))
	_ = mw.Close()

	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, respBody)
	}
	files, _ := os.ReadDir(dlDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file on disk, got %d", len(files))
	}
	got, _ := os.ReadFile(filepath.Join(dlDir, files[0].Name()))
	if string(got) != "hello from the iPhone" {
		t.Fatalf("text payload was corrupted: %q", got)
	}
}

// TestLegacyClipboardRejectsFilenameOnlyFormValue guards the "the PC only
// receives the title, not the image" bug. When the iOS Shortcut falls
// back to the urlencoded `clipboard` form and puts just the photo's
// filename there (a documented 1.5.4 quirk), the server must refuse to
// write that string to the Windows clipboard as text.
func TestLegacyClipboardRejectsFilenameOnlyFormValue(t *testing.T) {
	cases := []string{
		"IMG_0042.HEIC",
		"IMG_0042.PNG",
		"IMG_0042",
		"photo.png",
		"微信图片_20260817000108.jpg",
		"/private/var/mobile/Containers/Data/Application/.../IMG_0042.HEIC",
	}
	for _, value := range cases {
		value := value
		t.Run(value, func(t *testing.T) {
			ts, f, token := newTestServer(t, clipboard.Content{Kind: clipboard.Text, Text: "stale"})
			defer ts.Close()
			form := url.Values{"clipboard": {value}}.Encode()
			resp := req(t, "POST", ts.URL+"/clipboard", token, "application/x-www-form-urlencoded", strings.NewReader(form))
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for filename-only value, got %d body=%s", resp.StatusCode, body)
			}
			if !bytes.Contains(body, []byte("filename")) {
				t.Fatalf("error body should mention filename; got %s", body)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.content.Kind != clipboard.Text || f.content.Text != "stale" {
				t.Fatalf("clipboard should be untouched; got kind=%s text=%q", f.content.Kind, f.content.Text)
			}
		})
	}
}

// TestLegacyClipboardStillAcceptsTextThatLooksAlmostLikeAFilename makes
// sure the filename guard above does not reject legitimate text payloads
// just because they happen to contain an image extension or iOS name.
func TestLegacyClipboardStillAcceptsTextThatLooksAlmostLikeAFilename(t *testing.T) {
	cases := []string{
		"please send me report.pdf",
		"my photo.png is missing — any chance to recover it?",
		"this is fine, IMG_0042.PNG was just the name",
		"https://example.com/img.png is the URL",
	}
	for _, value := range cases {
		value := value
		t.Run(value, func(t *testing.T) {
			ts, f, token := newTestServer(t, clipboard.Content{})
			defer ts.Close()
			form := url.Values{"clipboard": {value}}.Encode()
			resp := req(t, "POST", ts.URL+"/clipboard", token, "application/x-www-form-urlencoded", strings.NewReader(form))
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.content.Kind != clipboard.Text || f.content.Text != value {
				t.Fatalf("clipboard should contain the text verbatim; got %q", f.content.Text)
			}
		})
	}
}

// TestUploadUndecodableImageStillSavesFile is the companion to the warn
// log added in savePart: an image whose format Go's image decoder cannot
// read (here a fake HEIC magic prefix) must still be persisted to disk,
// but the Windows clipboard must not be silently overwritten with empty
// bytes.
func TestUploadUndecodableImageStillSavesFile(t *testing.T) {
	ts, f, token, dlDir := newTestServerWithDir(t, clipboard.Content{Kind: clipboard.Text, Text: "previous"})
	defer ts.Close()
	heicBytes := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	p, _ := mw.CreateFormFile("file", "IMG_0042.HEIC")
	_, _ = p.Write(heicBytes)
	_ = mw.Close()

	resp := req(t, "POST", ts.URL+"/file", token, mw.FormDataContentType(), &body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	files, _ := os.ReadDir(dlDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file on disk, got %d", len(files))
	}
	got, _ := os.ReadFile(filepath.Join(dlDir, files[0].Name()))
	if !bytes.Equal(got, heicBytes) {
		t.Fatalf("HEIC bytes were not preserved verbatim: got %x want %x", got, heicBytes)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.content.Kind != clipboard.Text || f.content.Text != "previous" {
		t.Fatalf("clipboard must remain at the previous text entry; got kind=%s text=%q", f.content.Kind, f.content.Text)
	}
}
