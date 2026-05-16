package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedTemplatesParse(t *testing.T) {
	if _, err := template.ParseFS(templatesFS, "templates/index.html"); err != nil {
		t.Fatalf("index template parse failed: %v", err)
	}
	if _, err := template.ParseFS(templatesFS, "templates/qr.html"); err != nil {
		t.Fatalf("qr template parse failed: %v", err)
	}
	for _, name := range []string{"templates/style.css", "templates/app.js"} {
		if _, err := templatesFS.ReadFile(name); err != nil {
			t.Fatalf("%s read failed: %v", name, err)
		}
	}
}

func TestServeEmbeddedAsset(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)

	serveEmbeddedAsset("templates/style.css", "text/css; charset=utf-8")(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), ".app-container") {
		t.Fatal("style.css response did not contain expected CSS")
	}
}

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "https", raw: "https://www.youtube.com", ok: true},
		{name: "http", raw: "http://172.18.199.69:5000/health", ok: true},
		{name: "missing host", raw: "https://", ok: false},
		{name: "blocked scheme", raw: "file:///C:/Windows", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := normalizeRemoteURL(tt.raw)
			if ok != tt.ok {
				t.Fatalf("normalizeRemoteURL(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
		})
	}
}
