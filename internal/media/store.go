// Package media materializes OpenAI image_url / input_image payloads to disk
// so the Cursor CLI can read them via file paths (and optional --image flags).
package media

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxDownloadBytes = 20 << 20 // 20 MiB

// Store writes request attachments under a state directory.
type Store struct {
	Root   string
	Client *http.Client
}

func NewStore(root string) *Store {
	return &Store{
		Root: root,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Session is a per-request attachment directory that should be cleaned up
// after the agent run finishes.
type Session struct {
	Dir   string
	Paths []string
}

func (s Session) Cleanup() {
	if s.Dir == "" {
		return
	}
	_ = os.RemoveAll(s.Dir)
}

// Materialize writes each image URL (data: or http(s)) into a fresh directory.
// Empty urls returns a no-op session.
func (s *Store) Materialize(requestID string, urls []string) (Session, error) {
	cleaned := uniqueNonEmpty(urls)
	if len(cleaned) == 0 {
		return Session{}, nil
	}
	if s.Root == "" {
		return Session{}, fmt.Errorf("media store root is empty")
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return Session{}, err
	}
	id := sanitizeID(requestID)
	dir, err := os.MkdirTemp(s.Root, id+"-*")
	if err != nil {
		return Session{}, err
	}
	sess := Session{Dir: dir}
	for i, raw := range cleaned {
		path, err := s.writeOne(dir, i+1, raw)
		if err != nil {
			sess.Cleanup()
			return Session{}, fmt.Errorf("image %d: %w", i+1, err)
		}
		sess.Paths = append(sess.Paths, path)
	}
	return sess, nil
}

func (s *Store) writeOne(dir string, index int, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "data:") {
		return writeDataURL(dir, index, raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid image url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return s.downloadHTTP(dir, index, raw)
	case "file":
		return copyLocalFile(dir, index, u.Path)
	default:
		// Bare paths from some clients.
		if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
			return copyLocalFile(dir, index, raw)
		}
		return "", fmt.Errorf("unsupported image url scheme %q", u.Scheme)
	}
}

func writeDataURL(dir string, index int, raw string) (string, error) {
	meta, data, ok := strings.Cut(raw, ",")
	if !ok {
		return "", fmt.Errorf("malformed data url")
	}
	mime := "application/octet-stream"
	if rest, ok := strings.CutPrefix(meta, "data:"); ok {
		if mediaType, _, ok := strings.Cut(rest, ";"); ok && mediaType != "" {
			mime = mediaType
		} else if rest != "" && !strings.Contains(rest, ";") {
			mime = rest
		}
	}
	var (
		bytes []byte
		err   error
	)
	if strings.Contains(meta, ";base64") {
		bytes, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			// Some clients emit URL-safe base64.
			bytes, err = base64.RawURLEncoding.DecodeString(data)
		}
		if err != nil {
			return "", fmt.Errorf("decode base64 image: %w", err)
		}
	} else {
		unescaped, err := url.QueryUnescape(data)
		if err != nil {
			return "", fmt.Errorf("decode data url payload: %w", err)
		}
		bytes = []byte(unescaped)
	}
	if len(bytes) == 0 {
		return "", fmt.Errorf("empty image data")
	}
	name := fmt.Sprintf("image-%02d%s", index, extForMIME(mime))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) downloadHTTP(dir string, index int, raw string) (string, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(raw)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxDownloadBytes+1)
	bytes, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if len(bytes) > maxDownloadBytes {
		return "", fmt.Errorf("image exceeds %d byte limit", maxDownloadBytes)
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = http.DetectContentType(bytes)
	}
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	name := fmt.Sprintf("image-%02d%s", index, extForMIME(mime))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func copyLocalFile(dir string, index int, src string) (string, error) {
	bytes, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if len(bytes) == 0 {
		return "", fmt.Errorf("empty image file")
	}
	ext := filepath.Ext(src)
	if ext == "" {
		ext = extForMIME(http.DetectContentType(bytes))
	}
	name := fmt.Sprintf("image-%02d%s", index, ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func extForMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".bin"
	}
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "req"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
