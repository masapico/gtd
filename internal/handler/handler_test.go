package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gtd/internal/db"
	"gtd/internal/model"
)

func setupTestServer(t *testing.T) (*Handler, http.Handler, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	imagesDir := filepath.Join(dir, "images")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// テンプレート解析 (ローカルのファイルから読み込み)
	tmpl, err := template.New("").Funcs(TemplateFuncs()).ParseFiles(
		"../../templates/layout.html",
		"../../templates/partials/sidebar.html",
		"../../templates/partials/item_list.html",
		"../../templates/partials/item_detail.html",
		"../../templates/partials/attachments.html",
		"../../templates/partials/review_step.html",
	)
	if err != nil {
		t.Fatalf("ParseFiles failed: %v", err)
	}

	h := NewHandler(database, tmpl, imagesDir)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.ServeIndex)
	mux.HandleFunc("GET /partials/sidebar", h.ServeSidebar)
	mux.HandleFunc("GET /partials/items", h.ServeItemList)
	mux.HandleFunc("POST /items", h.CreateItem)
	mux.HandleFunc("GET /items/{id}/detail", h.GetItemDetail)
	mux.HandleFunc("PUT /items/{id}", h.UpdateItem)
	mux.HandleFunc("PATCH /items/{id}/status", h.UpdateItemStatus)
	mux.HandleFunc("DELETE /items/{id}", h.DeleteItem)
	mux.HandleFunc("POST /items/{id}/contexts/{contextId}/toggle", h.ToggleItemContext)
	mux.HandleFunc("POST /items/{id}/attachments", h.AddAttachment)
	mux.HandleFunc("POST /items/{id}/paste-image", h.PasteImage)
	mux.HandleFunc("GET /items/{id}/attachments", h.GetAttachments)
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("GET /review/step/{step}", h.ReviewStep)
	mux.HandleFunc("POST /review/projects/{id}/reviewed", h.MarkProjectReviewed)
	mux.HandleFunc("POST /review/items/{id}/reviewed", h.MarkItemReviewed)

	cleanup := func() {
		database.Close()
	}

	return h, mux, cleanup
}

func TestQuickCaptureAndItemList(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Quick Capture POST /items
	form := url.Values{}
	form.Set("title", "Submit expense report")
	req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); trigger != "itemChanged" {
		t.Errorf("expected HX-Trigger 'itemChanged', got %q", trigger)
	}

	// 2. Fetch partial items list
	req = httptest.NewRequest("GET", "/partials/items?view=inbox", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Submit expense report") {
		t.Errorf("expected item to be in inbox view")
	}
}

func TestRecurringTaskLifecycle(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create recurring item directly
	dueDate := time.Now().Format("2006-01-02")
	item := &model.Item{
		Title:      "Monthly server backup check",
		Status:     "next",
		DueDate:    &dueDate,
		Recurrence: "monthly",
	}
	itemID, err := db.CreateItem(h.DB, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 2. Complete item via PATCH /items/{id}/status
	form := url.Values{}
	form.Set("status", "done")
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/items/%d/status", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// 3. Verify in Next view that a new recurring task instance has appeared
	req = httptest.NewRequest("GET", "/partials/items?view=next", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Monthly server backup check") {
		t.Errorf("expected new monthly task instance in next actions view")
	}
	if !strings.Contains(body, "毎月") {
		t.Errorf("expected recurrence badge '毎月' in next actions view")
	}
}

func TestItemDetailAndAutoSave(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	item := &model.Item{
		Title:  "Draft contract",
		Status: "inbox",
	}
	itemID, _ := db.CreateItem(h.DB, item)

	// 1. Get Detail
	req := httptest.NewRequest("GET", fmt.Sprintf("/items/%d/detail", itemID), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Draft contract") {
		t.Errorf("expected item title in detail view")
	}

	// 2. Auto-save PUT /items/{id}
	form := url.Values{}
	form.Set("title", "Draft contract for Client X")
	form.Set("body", "Remember to add SLA clause")
	form.Set("status", "next")
	form.Set("recurrence", "none")

	req = httptest.NewRequest("PUT", fmt.Sprintf("/items/%d", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Verify updated
	updated, _ := db.GetItemByID(h.DB, itemID)
	if updated.Title != "Draft contract for Client X" || updated.Body != "Remember to add SLA clause" || updated.Status != "next" {
		t.Errorf("unexpected updated item: %+v", updated)
	}
}

func TestPasteImageAndAttachment(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	item := &model.Item{Title: "Fix UI glitch"}
	itemID, _ := db.CreateItem(h.DB, item)

	// Simulate multipart image upload (Ctrl+V)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "screenshot.png")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	// Write dummy PNG header bytes
	part.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRdummy"))
	writer.Close()

	req := httptest.NewRequest("POST", fmt.Sprintf("/items/%d/paste-image", itemID), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify attachment was registered
	attachments, err := db.GetAttachmentsByItemID(h.DB, itemID)
	if err != nil || len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Type != "image" {
		t.Errorf("expected attachment type 'image', got %s", attachments[0].Type)
	}

	// Verify image file exists on disk
	filePath := filepath.Join(h.ImagesDir, attachments[0].Target)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("expected image file to exist at %s", filePath)
	}
}

func TestWeeklyReviewSteps(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// Step 1
	req := httptest.NewRequest("GET", "/review/step/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Step 1: Get Clear") {
		t.Errorf("Step 1 render failed: %d", rec.Code)
	}

	// Step 2
	req = httptest.NewRequest("GET", "/review/step/2", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Step 2: Get Current") {
		t.Errorf("Step 2 render failed: %d", rec.Code)
	}

	// Step 3
	req = httptest.NewRequest("GET", "/review/step/3", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Step 3: Get Creative") {
		t.Errorf("Step 3 render failed: %d", rec.Code)
	}
}
