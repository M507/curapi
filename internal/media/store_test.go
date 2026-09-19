package media

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeDataURL(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	sess, err := store.Materialize("req-1", []string{url, url})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Cleanup()
	if len(sess.Paths) != 1 {
		t.Fatalf("dedupe expected 1 path, got %v", sess.Paths)
	}
	got, err := os.ReadFile(sess.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(png) {
		t.Fatalf("bytes mismatch")
	}
	if filepath.Ext(sess.Paths[0]) != ".png" {
		t.Fatalf("ext %q", sess.Paths[0])
	}
	sess.Cleanup()
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup left dir: %v", err)
	}
}

func TestMaterializeHTTP(t *testing.T) {
	payload := []byte("hello-image")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	store := NewStore(t.TempDir())
	sess, err := store.Materialize("abc", []string{srv.URL + "/x.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Cleanup()
	got, err := os.ReadFile(sess.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("%q", got)
	}
	if !strings.HasSuffix(sess.Paths[0], ".jpg") {
		t.Fatalf("%s", sess.Paths[0])
	}
}

func TestMaterializeUnsupported(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Materialize("x", []string{"ftp://example/a.png"})
	if err == nil {
		t.Fatal("expected error")
	}
}
