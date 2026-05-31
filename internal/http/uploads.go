package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) registerUploadRoutes() {
	uploadDir := s.cfg.UploadDir
	_ = os.MkdirAll(uploadDir, 0o755)
	s.mux.HandleFunc("/v1/uploads", s.handleUpload)
	s.mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))
}

func (s *Server) uploadCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !s.uploadCORS(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	const maxBody = 12 << 20 // 12 MiB
	if err := r.ParseMultipartForm(maxBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		ct := strings.ToLower(header.Header.Get("Content-Type"))
		switch ct {
		case "image/jpeg", "image/png", "image/webp":
			if ext == "" {
				ext = ".jpg"
			}
		default:
			writeError(w, http.StatusBadRequest, "only jpeg, png, webp images are allowed")
			return
		}
	}

	randPart := randomHex(8)
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixMilli(), randPart, ext)
	dest := filepath.Join(s.cfg.UploadDir, name)
	out, err := os.Create(dest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}
	_, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dest)
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	base := strings.TrimRight(s.effectivePublicBaseURL(), "/")
	if base == "" {
		base = strings.TrimRight(s.publicBaseURL(r), "/")
	}
	url := base + "/uploads/" + name
	writeJSON(w, http.StatusCreated, map[string]string{"url": url})
}

func (s *Server) publicBaseURL(r *http.Request) string {
	if b := strings.TrimSpace(s.cfg.PublicBaseURL); b != "" {
		return b
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:" + s.cfg.Port
	}
	return scheme + "://" + host
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
