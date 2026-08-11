package server

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
	"strings"
	"sync"
	"time"

	"github.com/hanboyd/AirDropPlus-Go/internal/clipboard"
	"github.com/hanboyd/AirDropPlus-Go/internal/config"
)

type Server struct {
	cfg   config.Config
	clip  clipboard.Service
	log   *slog.Logger
	mux   *http.ServeMux
	files sync.Map
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

func New(cfg config.Config, clip clipboard.Service, logger *slog.Logger) (*Server, error) {
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
	s := &Server{cfg: cfg, clip: clip, log: logger, mux: http.NewServeMux()}
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
		writeJSON(w, http.StatusOK, response{true, "ok", map[string]string{"version": "0.1.0"}})
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
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, response{false, err.Error(), nil})
		return
	}
	text := r.FormValue("clipboard")
	if text == "" {
		writeJSON(w, http.StatusBadRequest, response{false, "iPhone clipboard is empty", nil})
		return
	}
	if err := s.clip.WriteText(text); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{false, err.Error(), nil})
		return
	}
	writeJSON(w, http.StatusOK, response{true, "Send successful", nil})
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
	if _, err := io.Copy(f, part); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	ok = true
	s.log.Info("file received", "name", filepath.Base(path))
	if s.cfg.ImageUploadClipboard {
		if pngData, err := fileAsPNG(path); err == nil {
			if err := s.clip.WritePNG(pngData); err != nil {
				s.log.Warn("image saved but clipboard update failed", "error", err)
			}
		}
	}
	return filepath.Base(path), nil
}

func fileAsPNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
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
