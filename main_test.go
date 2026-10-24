package main

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gtd/internal/db"
	"gtd/internal/handler"
)

func TestEmbeddedServer(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gtd.db")
	imagesDir := filepath.Join(dir, "images")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	tmpl, err := template.New("").Funcs(handler.TemplateFuncs()).ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/partials/*.html",
	)
	if err != nil {
		t.Fatalf("ParseFS failed: %v", err)
	}

	h := handler.NewHandler(database, tmpl, imagesDir)

	mux := http.NewServeMux()
	subStatic, err := fs.Sub(staticFS, "static")
	if err != nil {
		t.Fatalf("fs.Sub failed: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subStatic))))
	mux.HandleFunc("GET /", h.ServeIndex)
	mux.HandleFunc("GET /partials/sidebar", h.ServeSidebar)

	// 1. Verify embedded static file serving
	req := httptest.NewRequest("GET", "/static/style.css", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /static/style.css, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "--md-sys-color-primary") {
		t.Errorf("static/style.css content mismatch")
	}

	// 2. Verify embedded layout template rendering
	req = httptest.NewRequest("GET", "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Personal Task & Info Manager") {
		t.Errorf("expected layout header in body")
	}
	if !strings.Contains(body, "quick-capture-input") {
		t.Errorf("expected quick capture input in body")
	}
	if !strings.Contains(body, "content-title") {
		t.Errorf("expected content-title in body, got: %s", body)
	}
}

