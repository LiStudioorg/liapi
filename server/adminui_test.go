package server

import (
	"net/http"
	"strings"
	"testing"
)

// The admin UI is a Nuxt-generated static SPA embedded via go:embed. These
// tests pin the serving contract: / → index.html, /_nuxt/* → hashed assets,
// and any other GET path → index.html (client-side router fallback).
func TestAdminUIServesSPA(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	t.Run("root serves index.html", func(t *testing.T) {
		rec := serve(srv, newRequest("GET", "/", "", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("content-type = %q, want text/html", ct)
		}
		if !strings.Contains(rec.Body.String(), "Liapi") {
			t.Fatalf("index.html body missing expected marker")
		}
	})

	t.Run("client route falls back to index.html", func(t *testing.T) {
		rec := serve(srv, newRequest("GET", "/upstreams", "", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("content-type = %q, want text/html", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("admin redirects to root", func(t *testing.T) {
		rec := serve(srv, newRequest("GET", "/admin", "", ""))
		if rec.Code != http.StatusMovedPermanently {
			t.Fatalf("status = %d, want 301", rec.Code)
		}
	})

	t.Run("admin api is open while login is disabled", func(t *testing.T) {
		rec := serve(srv, newRequest("GET", "/admin/api/overview", "", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (open console)", rec.Code)
		}
	})
}

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"index.html":          "text/html; charset=utf-8",
		"_nuxt/entry.abc.js":  "text/javascript; charset=utf-8",
		"_nuxt/entry.abc.css": "text/css; charset=utf-8",
		"favicon.ico":         "image/x-icon",
		"logo.svg":            "image/svg+xml",
	}
	for in, want := range cases {
		if got := contentTypeFor(in); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", in, got, want)
		}
	}
}
