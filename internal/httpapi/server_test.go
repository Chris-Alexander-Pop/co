package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Chris-Alexander-Pop/co/internal/archive"
)

func TestCacheUpload(t *testing.T) {
	ccache := t.TempDir()
	gocache := t.TempDir()
	cargo := t.TempDir()
	s := &Server{
		Token:     "tok",
		CcacheDir: ccache,
		GoCache:   gocache,
		CargoHome: cargo,
	}
	ccBlob := packDir(t, map[string]string{"a.o": "obj"})
	goBlob := packDir(t, map[string]string{"pkg/hash": "go"})
	cargoSrc := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cargoSrc, "registry", "idx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cargoSrc, "registry", "idx", "c"), []byte("crate"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cargoBuf bytes.Buffer
	if err := archive.PackPaths(cargoSrc, []string{"registry"}, &cargoBuf); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	writePart(t, w, "ccache", ccBlob)
	writePart(t, w, "gocache", goBlob)
	writePart(t, w, "cargo", cargoBuf.Bytes())
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/cache", &body)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Parts []struct {
			Name  string `json:"name"`
			Bytes int64  `json:"bytes"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res.Parts) != 3 {
		t.Fatalf("parts: %+v", res.Parts)
	}
	if got, err := os.ReadFile(filepath.Join(ccache, "a.o")); err != nil || string(got) != "obj" {
		t.Fatalf("ccache: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(gocache, "pkg", "hash")); err != nil || string(got) != "go" {
		t.Fatalf("gocache: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(cargo, "registry", "idx", "c")); err != nil || string(got) != "crate" {
		t.Fatalf("cargo: %q %v", got, err)
	}
}

func TestCacheUploadUnauthorized(t *testing.T) {
	s := &Server{Token: "tok", CcacheDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodPost, "/v1/cache", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func packDir(t *testing.T, files map[string]string) []byte {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := archive.Pack(root, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writePart(t *testing.T, w *multipart.Writer, name string, data []byte) {
	t.Helper()
	part, err := w.CreateFormFile(name, name+".tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}
