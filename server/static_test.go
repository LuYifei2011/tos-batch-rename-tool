package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDoesNotServeStaticFiles(t *testing.T) {
	web := t.TempDir()
	const content = "<h1>demo frontend</h1>"
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	a := &App{mode: "tos", web: web}
	for _, method := range []string{"GET", "HEAD", "POST"} {
		for _, path := range []string{"/", "/index.html", "/batch-rename/", "/v2/proxy/batch-rename/"} {
			w := httptest.NewRecorder()
			a.ServeHTTP(w, httptest.NewRequest(method, "http://nas:8181"+path, nil))
			if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), content) {
				t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
		}
	}
	a.mode = "demo"
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:18946/", nil))
	if w.Code != http.StatusOK || w.Body.String() != content {
		t.Fatalf("demo: %d %s", w.Code, w.Body.String())
	}
}
