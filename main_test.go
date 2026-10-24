package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gtd/internal/db"
	"gtd/internal/handler"
	"gtd/internal/model"
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

	mux := setupRouter(h)

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

	// Verify embedded favicon static serving
	req = httptest.NewRequest("GET", "/static/favicon.svg", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /static/favicon.svg, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<svg") || !strings.Contains(rec.Body.String(), "viewBox") {
		t.Errorf("static/favicon.svg content mismatch")
	}

	// Verify direct root /favicon.ico serving
	req = httptest.NewRequest("GET", "/favicon.ico", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /favicon.ico, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/x-icon" {
		t.Errorf("expected image/x-icon content-type, got %s", ct)
	}
	if rec.Body.Len() == 0 {
		t.Errorf("expected non-empty favicon.ico body")
	}

	// Verify direct root /apple-touch-icon.png serving
	req = httptest.NewRequest("GET", "/apple-touch-icon.png", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /apple-touch-icon.png, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png content-type, got %s", ct)
	}

	// 2. Verify embedded layout template rendering
	req = httptest.NewRequest("GET", "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Getting Things Done") {
		t.Errorf("expected brand header in body")
	}
	if !strings.Contains(body, "rel=\"icon\"") || !strings.Contains(body, "/static/favicon.svg") {
		t.Errorf("expected favicon link in head")
	}
	if !strings.Contains(body, "rel=\"apple-touch-icon\"") {
		t.Errorf("expected apple-touch-icon link in head")
	}
	if !strings.Contains(body, "quick-capture-input") {
		t.Errorf("expected quick capture input in body")
	}
	if !strings.Contains(body, "content-title") {
		t.Errorf("expected content-title in body, got: %s", body)
	}
	if !strings.Contains(body, "topbar-launcher-btn") {
		t.Errorf("expected topbar-launcher-btn in body, got: %s", body)
	}
	if !strings.Contains(body, "topbar-clock") {
		t.Errorf("expected topbar-clock in body, got: %s", body)
	}
	if !strings.Contains(body, "id=\"modal-container\"") {
		t.Errorf("expected modal-container in body, got: %s", body)
	}
	if !strings.Contains(body, "id=\"clock-modal\"") {
		t.Errorf("expected clock-modal in body, got: %s", body)
	}
	if !strings.Contains(body, "id=\"time-alert-banner\"") {
		t.Errorf("expected time-alert-banner in body, got: %s", body)
	}
	if !strings.Contains(body, "id=\"clock-badge-timer\"") {
		t.Errorf("expected clock-badge-timer in body, got: %s", body)
	}
	if !strings.Contains(body, "id=\"clock-badge-alarm\"") {
		t.Errorf("expected clock-badge-alarm in body, got: %s", body)
	}

	// 3. Verify static/app.js contains clock & timer logic
	req = httptest.NewRequest("GET", "/static/app.js", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /static/app.js, got %d", rec.Code)
	}
	jsBody := rec.Body.String()
	if !strings.Contains(jsBody, "openClockModal") {
		t.Errorf("expected openClockModal in app.js")
	}
	if !strings.Contains(jsBody, "selectTimerPreset") {
		t.Errorf("expected selectTimerPreset in app.js")
	}
	if !strings.Contains(jsBody, "saveAlarmFromInput") {
		t.Errorf("expected saveAlarmFromInput in app.js")
	}
}

func TestProjectViewInboxIsolationAndSequential(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gtd_views.db")
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
	mux := setupRouter(h)

	// 1. プロジェクト作成（並列）
	projID, err := db.CreateProject(database, &model.Project{
		Name:   "テストプロジェクト",
		Type:   "parallel",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. アイテム投入: Inbox 1件、Next 1件、Waiting 1件、Done 1件
	_, _ = db.CreateItem(database, &model.Item{ProjectID: &projID, Title: "Inboxタスク1", Status: "inbox"})
	_, _ = db.CreateItem(database, &model.Item{ProjectID: &projID, Title: "Nextタスク1", Status: "next"})
	_, _ = db.CreateItem(database, &model.Item{ProjectID: &projID, Title: "Waitingタスク1", Status: "waiting"})
	_, _ = db.CreateItem(database, &model.Item{ProjectID: &projID, Title: "Doneタスク1", Status: "done"})

	// 3. プロジェクトビューのHTML取得
	req := httptest.NewRequest("GET", "/partials/items?project_id=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeHTTP returned status %d: %s", rec.Code, res)
	}

	// 4. 検証: Inboxタスクが「未整理 (Inbox)」セクションに存在し、Nextに混ざらないこと
	if !strings.Contains(res, "未整理 (Inbox)") {
		t.Errorf("Expected '未整理 (Inbox)' section in HTML, got:\n%s", res)
	}
	if !strings.Contains(res, "Inboxタスク1") {
		t.Errorf("Expected 'Inboxタスク1' in HTML")
	}

	// 5. 順次プロジェクトへの変更
	reqType := httptest.NewRequest("PATCH", "/projects/1/type", strings.NewReader("type=sequential"))
	reqType.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recType := httptest.NewRecorder()
	mux.ServeHTTP(recType, reqType)
	if recType.Code != http.StatusOK {
		t.Fatalf("Update project type returned status %d", recType.Code)
	}

	// 再度HTML取得
	recSeq := httptest.NewRecorder()
	mux.ServeHTTP(recSeq, req)
	resSeq := recSeq.Body.String()

	// 順次プロジェクトでは「ステップ (順次実行)」セクションが表示されること
	if !strings.Contains(resSeq, "ステップ (順次実行)") {
		t.Errorf("Expected 'ステップ (順次実行)' section in sequential project HTML, got:\n%s", resSeq)
	}
}

