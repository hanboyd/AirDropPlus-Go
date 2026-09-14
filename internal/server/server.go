package server

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/config"
)

type Server struct {
	cfg             config.Config
	clip            clipboard.Service
	log             *slog.Logger
	mux             *http.ServeMux
	files           sync.Map
	onAuthenticated func(string)
	version         string
}

type Option func(*Server)

func WithAuthenticatedObserver(fn func(string)) Option {
	return func(s *Server) { s.onAuthenticated = fn }
}

func WithVersion(version string) Option {
	return func(s *Server) { s.version = version }
}

type response struct {
	Success bool   `json:"success"`
	Message string `json:"msg"`
	Data    any    `json:"data"`
}

type clipboardPayload struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type fileInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type fileRef struct {
	Path      string
	CreatedAt time.Time
}

func New(cfg config.Config, clip clipboard.Service, logger *slog.Logger, options ...Option) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if clip == nil {
		return nil, errors.New("clipboard service is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(cfg.DownloadDir, 0o700); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}
	s := &Server{cfg: cfg, clip: clip, log: logger, mux: http.NewServeMux(), version: "dev"}
	for _, option := range options {
		option(s)
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.requestLog(s.mux))
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr: s.cfg.Listen, Handler: s.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute,
		WriteTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "Hello World!") })
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, response{true, "ok", map[string]string{"version": s.version}})
	})
	s.mux.Handle("GET /api/v1/clipboard", s.auth(http.HandlerFunc(s.getClipboardV1)))
	s.mux.Handle("POST /api/v1/clipboard", s.auth(http.HandlerFunc(s.postClipboardV1)))
	s.mux.Handle("POST /api/v1/files", s.auth(http.HandlerFunc(s.uploadFilesV1)))
	s.mux.Handle("GET /api/v1/files/clipboard", s.auth(http.HandlerFunc(s.clipboardFilesV1)))
	s.mux.Handle("GET /api/v1/files/{id}", s.auth(http.HandlerFunc(s.downloadFileV1)))
	// Public AirDropPlus 1.5 API compatibility for its Apple-signed Shortcut.
	s.mux.Handle("GET /clipboard", s.auth(http.HandlerFunc(s.getClipboardLegacy)))
	s.mux.Handle("POST /clipboard", s.auth(http.HandlerFunc(s.postClipboardLegacy)))
	s.mux.Handle("POST /file", s.auth(http.HandlerFunc(s.uploadFileLegacy)))
	s.mux.Handle("GET /file/{id}", s.auth(http.HandlerFunc(s.downloadFileLegacy)))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(r.Header.Get("Authorization"))
		provided = strings.TrimPrefix(provided, "Bearer ")
		if provided == "" {
			provided = r.Header.Get("X-AirDropPlus-Token")
		}
		if len(provided) != len(s.cfg.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.Token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, response{false, "invalid token", nil})
			return
		}
		if v := r.Header.Get("ShortcutVersion"); v != "" && s.cfg.LegacyShortcutVersion != "" {
			if majorMinor(v) != majorMinor(s.cfg.LegacyShortcutVersion) {
				writeJSON(w, http.StatusBadRequest, response{false, "shortcut version mismatch", nil})
				return
			}
		}
		if s.onAuthenticated != nil {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			s.onAuthenticated(host)
		}
		next.ServeHTTP(w, r)
	})
}

func majorMinor(v string) string {
	p := strings.Split(v, ".")
	if len(p) >= 2 {
		return p[0] + "." + p[1]
	}
	return v
}

func (s *Server) getClipboardV1(w http.ResponseWriter, _ *http.Request) {
	c, err := s.clip.Read()
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{false, err.Error(), nil})
		return
	}
	switch c.Kind {
	case clipboard.Text:
		writeJSON(w, http.StatusOK, response{true, "", clipboardPayload{Type: "text", Data: c.Text}})
	case clipboard.Image:
		writeJSON(w, http.StatusOK, response{true, "", clipboardPayload{Type: "image", Data: base64.StdEncoding.EncodeToString(c.PNG)}})
	case clipboard.Files:
		infos := s.registerFiles(c.Files)
		writeJSON(w, http.StatusOK, response{true, "", map[string]any{"type": "files", "files": infos}})
	default:
		writeJSON(w, http.StatusUnprocessableEntity, response{false, "unsupported clipboard format", nil})
	}
}

func (s *Server) postClipboardV1(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, min(s.cfg.MaxUploadBytes, 64<<20))
	var p clipboardPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, response{false, "invalid JSON", nil})
		return
	}
	var err error
	switch p.Type {
	case "text":
		if p.Data == "" {
			err = errors.New("clipboard text is empty")
		} else {
			err = s.clip.WriteText(p.Data)
		}
	case "image", "img":
		var b []byte
		b, err = base64.StdEncoding.DecodeString(p.Data)
		if err == nil {
			err = s.clip.WritePNG(b)
		}
	default:
		err = errors.New("type must be text or image")
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{false, err.Error(), nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "clipboard updated", nil})
}

func (s *Server) getClipboardLegacy(w http.ResponseWriter, _ *http.Request) {
	c, err := s.clip.Read()
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{false, err.Error(), nil})
		return
	}
	var data any
	switch c.Kind {
	case clipboard.Text:
		data = map[string]any{"type": "text", "data": c.Text}
	case clipboard.Image:
		data = map[string]any{"type": "img", "data": base64.StdEncoding.EncodeToString(c.PNG)}
	case clipboard.Files:
		infos := s.registerFiles(c.Files)
		ids := make([]string, 0, len(infos))
		for _, f := range infos {
			ids = append(ids, f.ID)
		}
		data = map[string]any{"type": "file", "data": ids}
	default:
		writeJSON(w, http.StatusUnprocessableEntity, response{false, "unsupported clipboard format", nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "", data})
}

func (s *Server) postClipboardLegacy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, min(s.cfg.MaxUploadBytes, 64<<20))
	// P1 diagnostic: capture what the iPhone actually sends to /clipboard
	// so we can see whether the request is multipart with HEIC bytes or
	// a urlencoded form carrying just the filename. Keep this entry-level
	// log tiny; per-part logging lives in imageFromMultipart.
	s.log.Info("clipboard upload entry",
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength)
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		pngData, err := imageFromMultipart(s, r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{false, err.Error(), nil})
			return
		}
		if err := s.clip.WritePNG(pngData); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{false, err.Error(), nil})
			return
		}
		writeJSON(w, http.StatusOK, response{true, "Send successful", nil})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, response{false, err.Error(), nil})
		return
	}
	text := r.FormValue("clipboard")
	s.log.Info("clipboard form text",
		"value_len", len(text),
		"value_preview", previewASCII(text, 64),
		"value_hex_prefix", hexPrefix(text, 32))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, response{false, "iPhone clipboard is empty", nil})
		return
	}
	if pngData, ok := inlineImage(text); ok {
		if err := s.clip.WritePNG(pngData); err != nil {
			writeJSON(w, http.StatusInternalServerError, response{false, err.Error(), nil})
			return
		}
		writeJSON(w, http.StatusOK, response{true, "Send successful", nil})
		return
	}
	// The AirDropPlus 1.5.4 signed shortcut has been observed to send the
	// shared file's name (e.g. "IMG_0042.HEIC") as the form value of
	// "clipboard" when its multipart detector fails. Without this guard the
	// PC happily writes the filename string into the Windows clipboard,
	// which the user sees as "received only the title". Treat any value
	// that looks like a single filename as a malformed image upload.
	if reason, ok := filenameOnlyReason(text); ok {
		s.log.Warn("clipboard form rejected as filename-only",
			"value_preview", previewASCII(text, 64),
			"reason", reason)
		writeJSON(w, http.StatusBadRequest, response{false, reason, nil})
		return
	}
	if err := s.clip.WriteText(text); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{false, err.Error(), nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "Send successful", nil})
}

// filenameOnlyReason reports whether v looks like a single filename or path
// (i.e. an iOS Shortcut quirk that sent the file's name instead of its
// content). It returns the human-readable reason and true when matched so
// the server can reject it without polluting the Windows clipboard.
//
// The check is deliberately narrow: real text payloads that happen to
// contain an image extension or a slash must still be accepted. Anything
// that is purely a filename (no whitespace, no sentence punctuation,
// ends with a known media extension or matches an iOS photo name) is
// treated as the documented iOS quirk.
func filenameOnlyReason(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	// No whitespace => a single token, which is exactly what iOS Shortcuts
	// leaks when "Get Contents of URL" resolves to a filename instead of
	// the binary.
	if strings.ContainsAny(v, " \t\r\n　") {
		return "", false
	}
	// Sentence punctuation (comma, semicolon, colon, question mark,
	// exclamation mark) means the value is prose, not a filename. The
	// period is intentionally allowed: real filenames like
	// "IMG_0042.HEIC" or "/a/b/photo.png" always contain a dot, and
	// excluding them here would let the documented iOS quirk slip past.
	if strings.ContainsAny(v, ",;:?!") {
		return "", false
	}
	lower := strings.ToLower(v)
	// Common image, video and document extensions the iPhone share sheet
	// would send. Matching here is safe because the prior whitespace /
	// punctuation check already removed real prose.
	mediaExts := []string{
		".heic", ".heif", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff",
		".mov", ".mp4", ".m4v", ".3gp",
		".pdf",
	}
	for _, ext := range mediaExts {
		if strings.HasSuffix(lower, ext) {
			return fmt.Sprintf("clipboard form value %q looks like a filename; send the image as multipart to /file or Base64 JSON to /api/v1/clipboard", v), true
		}
	}
	// iOS photo names follow the IMG_<digits>[_<suffix>] pattern. Matching
	// this explicitly lets us flag a name like "IMG_0042" even when iOS
	// forgot to append the extension (another known quirk).
	if iosPhotoName.MatchString(v) {
		return fmt.Sprintf("clipboard form value %q looks like an iOS photo name; send the image bytes, not its filename", v), true
	}
	return "", false
}

// iosPhotoName matches typical iPhone camera filenames: "IMG_0042",
// "IMG_0042_1", "IMG_E0042", etc. The pattern intentionally allows
// optional underscores and letters before the trailing digits so that
// HDR / live / portrait variants are also caught.
var iosPhotoName = regexp.MustCompile(`^IMG_[A-Z]*\d+(?:_\d+)?$`)

// previewASCII returns the first n bytes of s as a printable string,
// substituting '·' for non-printable runes so log lines stay one line.
func previewASCII(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f || r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		} else {
			b.WriteByte('·')
		}
	}
	return b.String()
}

// hexPrefix returns the first n bytes of s as hex, capped safely.
func hexPrefix(s string, n int) string {
	if n > len(s) {
		n = len(s)
	}
	return hex.EncodeToString([]byte(s[:n]))
}

func inlineImage(value string) ([]byte, bool) {
	encoded := strings.TrimSpace(value)
	if i := strings.Index(encoded, ","); strings.HasPrefix(strings.ToLower(encoded), "data:image/") && i >= 0 {
		encoded = encoded[i+1:]
	} else if len(encoded) < 128 {
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, false
	}
	pngData, err := imageAsPNG(bytes.NewReader(raw))
	return pngData, err == nil
}

func imageFromMultipart(s *Server, r *http.Request) ([]byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("multipart image required: %w", err)
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("multipart request contains no decodable image")
		}
		if err != nil {
			return nil, err
		}
		isCandidate := part.FileName() != "" || part.FormName() == "clipboard" || strings.HasPrefix(strings.ToLower(part.Header.Get("Content-Type")), "image/")
		s.log.Info("clipboard multipart part",
			"file_name", part.FileName(),
			"form_name", part.FormName(),
			"content_type", part.Header.Get("Content-Type"),
			"is_candidate", isCandidate)
		if !isCandidate {
			part.Close()
			continue
		}
		// P1 diagnostic: tee the body through a counter so we know how
		// many bytes the iPhone actually sent without breaking image.Decode,
		// which needs to see the magic bytes at the very start of the stream.
		counter := &readCounter{}
		body := io.TeeReader(part, counter)
		pngData, decodeErr := imageAsPNG(body)
		part.Close()
		s.log.Info("clipboard multipart part result",
			"file_name", part.FileName(),
			"form_name", part.FormName(),
			"bytes_read", counter.n,
			"decode_ok", decodeErr == nil,
			"decode_error", errString(decodeErr))
		if decodeErr == nil {
			return pngData, nil
		}
	}
}

// readCounter is an io.Writer that records how many bytes were passed in
// (used as the sink of an io.TeeReader to learn the size of a multipart
// part body without consuming the bytes themselves).
type readCounter struct{ n int64 }

func (c *readCounter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) uploadFileLegacy(w http.ResponseWriter, r *http.Request) {
	names, err := s.saveMultipart(w, r)
	if err != nil {
		writeJSON(w, uploadStatus(err), response{false, err.Error(), nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "Send successful", names})
}

func (s *Server) uploadFilesV1(w http.ResponseWriter, r *http.Request) {
	names, err := s.saveMultipart(w, r)
	if err != nil {
		writeJSON(w, uploadStatus(err), response{false, err.Error(), nil})
		return
	}
	writeJSON(w, http.StatusCreated, response{true, "files received", names})
}

func (s *Server) saveMultipart(w http.ResponseWriter, r *http.Request) ([]string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("multipart form required: %w", err)
	}
	var names []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		name, err := s.savePart(part)
		part.Close()
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, errors.New("no files supplied")
	}
	return names, nil
}

func (s *Server) savePart(part *multipart.Part) (string, error) {
	name := sanitizeName(part.FileName())
	path, err := availablePath(s.cfg.DownloadDir, name)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	written, err := io.Copy(f, part)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := checkUploadBody(name, part.FileName(), path, written); err != nil {
		return "", err
	}
	ok = true
	s.log.Info("file received", "name", filepath.Base(path), "bytes", written)
	if s.cfg.ImageUploadClipboard {
		pngData, decodeErr := fileAsPNG(path)
		if decodeErr != nil {
			// Common case is the iPhone sending HEIC/HEIF bytes — Go's
			// image.Decode only supports PNG/JPEG/GIF, so the file lands
			// on disk but the clipboard stays at whatever was there before.
			// Surface this loudly so the user doesn't think the upload
			// silently produced an empty clipboard entry.
			s.log.Warn("image saved but clipboard update skipped: decoder does not support this format",
				"name", filepath.Base(path),
				"bytes", written,
				"decode_error", decodeErr.Error())
		} else if err := s.clip.WritePNG(pngData); err != nil {
			s.log.Warn("image saved but clipboard update failed",
				"name", filepath.Base(path),
				"error", err.Error())
		}
	}
	return filepath.Base(path), nil
}

// checkUploadBody rejects uploads that look like an iOS Shortcut quirk:
// the multipart part has a filename but no real file content. Saving those
// silently produces either a 0-byte file or a tiny file whose body is just
// the filename string, leaving the Windows side with "name only, no content".
func checkUploadBody(sanitizedName, rawName, path string, written int64) error {
	if written == 0 {
		return fmt.Errorf("received file %q is empty; check the iOS shortcut's file field", sanitizedName)
	}
	// Heuristic: very small bodies that look like a single text token are
	// almost certainly the filename itself (or a path) leaking through as
	// the part body. Anything binary or larger than a few hundred bytes is
	// assumed legitimate even if it happens to look like a filename.
	if written > 512 {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Errorf("received file %q is empty; check the iOS shortcut's file field", sanitizedName)
	}
	if isLikelyNameEcho(trimmed, sanitizedName, rawName) {
		return fmt.Errorf("received file %q contains only its name; check the iOS shortcut's file field", sanitizedName)
	}
	return nil
}

func isLikelyNameEcho(body, sanitizedName, rawName string) bool {
	candidates := []string{sanitizedName, rawName}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if body == c {
			return true
		}
		if base := filepath.Base(c); base != "" && body == base {
			return true
		}
	}
	return false
}

func fileAsPNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return imageAsPNG(f)
}

func imageAsPNG(r io.Reader) ([]byte, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func sanitizeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "unnamed"
	}
	return name
}

func availablePath(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		path := filepath.Join(dir, candidate)
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("too many duplicate filenames")
}

func (s *Server) registerFiles(paths []string) []fileInfo {
	out := make([]fileInfo, 0, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		id := randomID()
		s.files.Store(id, fileRef{Path: p, CreatedAt: time.Now()})
		out = append(out, fileInfo{ID: id, Name: st.Name(), Size: st.Size(), URL: "/api/v1/files/" + id})
	}
	return out
}

func randomID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) clipboardFilesV1(w http.ResponseWriter, _ *http.Request) {
	c, err := s.clip.Read()
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{false, err.Error(), nil})
		return
	}
	if c.Kind != clipboard.Files {
		writeJSON(w, http.StatusConflict, response{false, "clipboard does not contain files", nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "", s.registerFiles(c.Files)})
}

func (s *Server) downloadFileV1(w http.ResponseWriter, r *http.Request) {
	s.serveRegistered(w, r, r.PathValue("id"))
}
func (s *Server) downloadFileLegacy(w http.ResponseWriter, r *http.Request) {
	s.serveRegistered(w, r, r.PathValue("id"))
}

func (s *Server) serveRegistered(w http.ResponseWriter, r *http.Request, id string) {
	v, ok := s.files.Load(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, response{false, "file reference not found or expired", nil})
		return
	}
	ref := v.(fileRef)
	if time.Since(ref.CreatedAt) > 10*time.Minute {
		s.files.Delete(id)
		writeJSON(w, http.StatusGone, response{false, "file reference expired", nil})
		return
	}
	path := ref.Path
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		writeJSON(w, http.StatusNotFound, response{false, "file no longer exists", nil})
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", st.Name()))
	http.ServeFile(w, r, path)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "client", host)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func uploadStatus(err error) int {
	if strings.Contains(err.Error(), "request body too large") {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
