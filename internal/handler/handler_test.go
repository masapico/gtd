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
		"../../templates/partials/search_results.html",
	)
	if err != nil {
		t.Fatalf("ParseFiles failed: %v", err)
	}

	h := NewHandler(database, tmpl, imagesDir)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.ServeIndex)
	mux.HandleFunc("GET /partials/sidebar", h.ServeSidebar)
	mux.HandleFunc("GET /partials/items", h.ServeItemList)
	mux.HandleFunc("GET /search", h.Search)
	mux.HandleFunc("POST /items", h.CreateItem)
	mux.HandleFunc("GET /items/{id}/detail", h.GetItemDetail)
	mux.HandleFunc("PUT /items/{id}", h.UpdateItem)
	mux.HandleFunc("PATCH /items/{id}/status", h.UpdateItemStatus)
	mux.HandleFunc("DELETE /items/{id}", h.DeleteItem)
	mux.HandleFunc("POST /items/{id}/contexts/{contextId}/toggle", h.ToggleItemContext)
	mux.HandleFunc("POST /items/{id}/attachments", h.AddAttachment)
	mux.HandleFunc("POST /items/{id}/paste-image", h.PasteImage)
	mux.HandleFunc("GET /items/{id}/attachments", h.GetAttachments)
	mux.HandleFunc("POST /attachments/{id}/open", h.OpenAttachment)
	mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("PUT /projects/{id}", h.UpdateProject)
	mux.HandleFunc("PATCH /projects/{id}/status", h.UpdateProjectStatus)
	mux.HandleFunc("DELETE /projects/{id}", h.DeleteProject)
	mux.HandleFunc("POST /contexts", h.CreateContext)
	mux.HandleFunc("PUT /contexts/{id}", h.UpdateContext)
	mux.HandleFunc("DELETE /contexts/{id}", h.DeleteContext)
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
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "itemChanged") {
		t.Errorf("expected HX-Trigger to contain 'itemChanged', got %q", trigger)
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

	// 3. Fetch with context_id filter (simulates clicking sidebar context or chip)
	req = httptest.NewRequest("GET", "/partials/items?view=next&context_id=1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for context filter, got %d", rec.Code)
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
	if strings.Contains(body, "T00:00:00Z") {
		t.Errorf("expected due date not to contain T00:00:00Z, got raw ISO timestamp")
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

func TestItemStatusChange(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	item := &model.Item{
		Title:  "Call supplier",
		Status: "inbox",
	}
	itemID, err := db.CreateItem(h.DB, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// Change status via PATCH /items/{id}/status (e.g. click "Waiting For" button)
	form := url.Values{}
	form.Set("status", "waiting")
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/items/%d/status", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "itemChanged") {
		t.Errorf("expected HX-Trigger to contain 'itemChanged', got %q", trigger)
	}

	// Check response body renders updated detail with active waiting status
	body := rec.Body.String()
	if !strings.Contains(body, `class="status-btn active"`) || !strings.Contains(body, "Waiting") {
		t.Errorf("expected Waiting button to be active in returned detail html, got: %s", body)
	}

	// Verify DB state
	updated, err := db.GetItemByID(h.DB, itemID)
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}
	if updated.Status != "waiting" {
		t.Errorf("expected status 'waiting', got %q", updated.Status)
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

	// 2. Test uploading WebP format
	bodyWebp := &bytes.Buffer{}
	writerWebp := multipart.NewWriter(bodyWebp)
	partWebp, _ := writerWebp.CreateFormFile("image", "screen.webp")
	partWebp.Write([]byte("RIFFdummyWEBP"))
	writerWebp.Close()

	reqWebp := httptest.NewRequest("POST", fmt.Sprintf("/items/%d/paste-image", itemID), bodyWebp)
	reqWebp.Header.Set("Content-Type", writerWebp.FormDataContentType())
	recWebp := httptest.NewRecorder()
	mux.ServeHTTP(recWebp, reqWebp)
	if recWebp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for webp upload, got %d", recWebp.Code)
	}

	attachmentsWebp, err := db.GetAttachmentsByItemID(h.DB, itemID)
	if err != nil || len(attachmentsWebp) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachmentsWebp))
	}
	if !strings.HasSuffix(attachmentsWebp[1].Target, ".webp") {
		t.Errorf("expected .webp suffix, got %s", attachmentsWebp[1].Target)
	}

	// 3. Test non-existent item ID returns 404
	bodyNonExistent := &bytes.Buffer{}
	writerNonExistent := multipart.NewWriter(bodyNonExistent)
	partNonExistent, _ := writerNonExistent.CreateFormFile("image", "dummy.png")
	partNonExistent.Write([]byte("dummy"))
	writerNonExistent.Close()

	reqNonExistent := httptest.NewRequest("POST", "/items/99999/paste-image", bodyNonExistent)
	reqNonExistent.Header.Set("Content-Type", writerNonExistent.FormDataContentType())
	recNonExistent := httptest.NewRecorder()
	mux.ServeHTTP(recNonExistent, reqNonExistent)
	if recNonExistent.Code != http.StatusNotFound {
		t.Fatalf("expected 404 StatusNotFound for invalid item ID, got %d", recNonExistent.Code)
	}
}

func TestDeleteAttachment(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	item := &model.Item{Title: "Item with attachment"}
	itemID, _ := db.CreateItem(h.DB, item)

	// Create dummy image file in ImagesDir
	os.MkdirAll(h.ImagesDir, 0755)
	dummyFilename := "test_screenshot.png"
	filePath := filepath.Join(h.ImagesDir, dummyFilename)
	os.WriteFile(filePath, []byte("fake image data"), 0644)

	att := &model.Attachment{
		ItemID: itemID,
		Label:  "Test Screenshot",
		Target: dummyFilename,
		Type:   "image",
	}
	attID, err := db.CreateAttachment(h.DB, att)
	if err != nil {
		t.Fatalf("CreateAttachment failed: %v", err)
	}

	// Delete attachment via DELETE /attachments/{id}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/attachments/%d", attID), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Verify response contains updated attachments partial
	if !strings.Contains(rec.Body.String(), "attachments-section") && !strings.Contains(rec.Body.String(), "form-group") {
		t.Errorf("expected attachments partial in response, got %s", rec.Body.String())
	}

	// Verify attachment was removed from DB
	attachments, err := db.GetAttachmentsByItemID(h.DB, itemID)
	if err != nil || len(attachments) != 0 {
		t.Fatalf("expected 0 attachments after delete, got %d", len(attachments))
	}

	// Verify image file was removed from disk
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected image file to be deleted from disk at %s", filePath)
	}
}

func TestDeleteItemCleansUpImages(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	item := &model.Item{Title: "Task to delete"}
	itemID, _ := db.CreateItem(h.DB, item)

	// Create dummy image file in ImagesDir
	os.MkdirAll(h.ImagesDir, 0755)
	dummyFilename := "task_image_to_delete.png"
	filePath := filepath.Join(h.ImagesDir, dummyFilename)
	os.WriteFile(filePath, []byte("fake image data"), 0644)

	att := &model.Attachment{
		ItemID: itemID,
		Label:  "Attached Image",
		Target: dummyFilename,
		Type:   "image",
	}
	_, err := db.CreateAttachment(h.DB, att)
	if err != nil {
		t.Fatalf("CreateAttachment failed: %v", err)
	}

	// Delete item via DELETE /items/{id}
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/items/%d", itemID), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	// Verify item was removed from DB
	_, err = db.GetItemByID(h.DB, itemID)
	if err == nil {
		t.Errorf("expected item to be deleted from DB")
	}

	// Verify image file on disk was also deleted
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected image file to be deleted from disk when item was deleted at %s", filePath)
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

func TestProjectStatusTransitionsAndRestore(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create a project
	p := &model.Project{
		Name:        "Q4 Financial Audit",
		Description: "Internal review",
	}
	pID, err := db.CreateProject(h.DB, p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. Mark project as done
	form := url.Values{}
	form.Set("status", "done")
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/projects/%d/status", pID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Mark done failed: %d", rec.Code)
	}

	// 3. View project detail: should show '完了済み' and 'アクティブに戻す' button
	req = httptest.NewRequest("GET", fmt.Sprintf("/partials/items?project_id=%d", pID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("View project failed: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "完了済み") || !strings.Contains(body, "アクティブに戻す") {
		t.Errorf("expected restore button in completed project view, got: %s", body)
	}

	// 4. View projects overview list: should be visible in 'done' filter
	req = httptest.NewRequest("GET", "/partials/items?view=projects&project_status=done", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("View projects overview failed: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Q4 Financial Audit") {
		t.Errorf("expected completed project in projects overview")
	}

	// 5. Restore project to active
	form = url.Values{}
	form.Set("status", "active")
	req = httptest.NewRequest("PATCH", fmt.Sprintf("/projects/%d/status", pID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Restore failed: %d", rec.Code)
	}

	// Verify project status is active again
	restored, err := db.GetProjectByID(h.DB, pID)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if restored.Status != "active" {
		t.Errorf("expected status 'active', got %s", restored.Status)
	}
}

func TestSmartAddAndContextIntegration(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Quick Capture with Smart Add syntax: "#PC" and "^today"
	today := time.Now().Format("2006-01-02")
	form := url.Values{}
	form.Set("title", "プレゼン資料の作成 #PC #重要 ^today")
	req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Quick capture failed: %d", rec.Code)
	}

	// Verify created item
	items, err := db.GetItems(h.DB, model.ItemFilter{View: "inbox"})
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	it := items[0]
	if it.Title != "プレゼン資料の作成" {
		t.Errorf("expected clean title 'プレゼン資料の作成', got %q", it.Title)
	}
	if it.DueDate == nil || *it.DueDate != today {
		t.Errorf("expected due date %q, got %v", today, it.DueDate)
	}
	if len(it.Contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d (%v)", len(it.Contexts), it.Contexts)
	}

	// 2. Test Smart Add in Item Detail Title update
	form = url.Values{}
	form.Set("title", "プレゼン資料の作成とレビュー #会議 ^tomorrow")
	form.Set("status", "next")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/items/%d", it.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateItem failed: %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "itemDetailRefresh") {
		t.Errorf("expected itemDetailRefresh trigger, got %q", trigger)
	}

	updated, err := db.GetItemByID(h.DB, it.ID)
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}
	if updated.Title != "プレゼン資料の作成とレビュー" {
		t.Errorf("expected clean title 'プレゼン資料の作成とレビュー', got %q", updated.Title)
	}
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if updated.DueDate == nil || *updated.DueDate != tomorrow {
		t.Errorf("expected due date %q, got %v", tomorrow, updated.DueDate)
	}
	if !updated.HasContext(it.Contexts[0].ID) {
		t.Errorf("expected existing context to be preserved")
	}
}

func TestContextManagementViewAndCRUD(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. View Contexts management screen
	req := httptest.NewRequest("GET", "/partials/items?view=contexts", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("View contexts failed: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Contexts 管理") {
		t.Errorf("expected 'Contexts 管理' in view, got: %s", body)
	}
	if !strings.Contains(body, "#PC") {
		t.Errorf("expected '#PC' in contexts list")
	}

	// 2. Create Context via POST /contexts
	form := url.Values{}
	form.Set("name", "出張")
	req = httptest.NewRequest("POST", "/contexts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("CreateContext failed: %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "contextChanged") {
		t.Errorf("expected contextChanged trigger, got %q", trigger)
	}

	// 3. Verify in Contexts view
	req = httptest.NewRequest("GET", "/partials/items?view=contexts", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "#出張") {
		t.Errorf("expected '#出張' in contexts list")
	}

	// 4. Update Context via PUT /contexts/{id}
	// #出張 のIDを取得するためにシード件数(5)の次であるID 6を想定、またはクエリ
	req = httptest.NewRequest("GET", "/partials/items?view=contexts", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	form = url.Values{}
	form.Set("name", "国内出張")
	req = httptest.NewRequest("PUT", "/contexts/6", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateContext failed: %d", rec.Code)
	}

	// 5. Delete Context via DELETE /contexts/{id}
	req = httptest.NewRequest("DELETE", "/contexts/6", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DeleteContext failed: %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "contextChanged") {
		t.Errorf("expected contextChanged trigger on delete, got %q", trigger)
	}

	// 6. Verify URL sync script is rendered in partials
	if !strings.Contains(body, "pane.setAttribute('hx-get', '/partials/items?view=contexts')") {
		t.Errorf("expected hx-get sync script in partials view, got: %s", body)
	}
}

func TestRelativeDueDateRenderingInItemList(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	now := time.Now()
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Create items with today, tomorrow, yesterday
	itemsToCreate := []struct {
		title   string
		dueDate string
	}{
		{title: "Today Task", dueDate: today},
		{title: "Tomorrow Task", dueDate: tomorrow},
		{title: "Yesterday Task", dueDate: yesterday},
	}

	for _, item := range itemsToCreate {
		form := url.Values{}
		form.Set("title", item.title)
		form.Set("due_date", item.dueDate)
		req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Create item %s failed: %d", item.title, rec.Code)
		}
	}

	// Fetch items partial
	req := httptest.NewRequest("GET", "/partials/items?view=inbox", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Get items failed: %d", rec.Code)
	}

	body := rec.Body.String()

	// Verify labels
	if !strings.Contains(body, "<span>今日</span>") {
		t.Errorf("expected '<span>今日</span>' in item list")
	}
	if !strings.Contains(body, "<span>明日</span>") {
		t.Errorf("expected '<span>明日</span>' in item list")
	}
	if !strings.Contains(body, "<span>期限超過 (昨日)</span>") {
		t.Errorf("expected '<span>期限超過 (昨日)</span>' in item list")
	}

	// Verify CSS classes
	if !strings.Contains(body, "due-today") {
		t.Errorf("expected 'due-today' class in item list")
	}
	if !strings.Contains(body, "due-soon") {
		t.Errorf("expected 'due-soon' class in item list")
	}
	if !strings.Contains(body, "due-overdue") {
		t.Errorf("expected 'due-overdue' class in item list")
	}
}

func TestUpdateProjectNameAndDescription(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. プロジェクト作成
	p := &model.Project{
		Name:        "旧プロジェクト名",
		Description: "プロジェクトの目的",
		Status:      "someday",
	}
	pID, err := db.CreateProject(h.DB, p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. PUT /projects/{id} で名前のみを変更（statusやdescriptionを省略）
	form := url.Values{}
	form.Set("name", "新プロジェクト名")
	req := httptest.NewRequest("PUT", fmt.Sprintf("/projects/%d", pID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateProject failed: %d", rec.Code)
	}

	// HX-Trigger に projectChanged が含まれること
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "projectChanged") {
		t.Errorf("expected 'projectChanged' in HX-Trigger, got: %s", trigger)
	}

	// DB確認: 名前が更新され、既存のstatus(someday)とdescriptionが維持されていること
	updated, err := db.GetProjectByID(h.DB, pID)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if updated.Name != "新プロジェクト名" {
		t.Errorf("expected Name '新プロジェクト名', got '%s'", updated.Name)
	}
	if updated.Status != "someday" {
		t.Errorf("expected Status 'someday' preserved, got '%s'", updated.Status)
	}
	if updated.Description != "プロジェクトの目的" {
		t.Errorf("expected Description preserved, got '%s'", updated.Description)
	}

	// 3. PUT /projects/{id} で名前と説明の両方を変更
	form = url.Values{}
	form.Set("name", "最新プロジェクト名")
	form.Set("description", "更新後のゴール説明")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/projects/%d", pID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateProject with desc failed: %d", rec.Code)
	}

	updated, _ = db.GetProjectByID(h.DB, pID)
	if updated.Name != "最新プロジェクト名" || updated.Description != "更新後のゴール説明" {
		t.Errorf("expected updated name & description, got: %+v", updated)
	}

	// 4. プロジェクト個別ビューの表示確認: 最新名、編集ボタン、インライン編集フォームが含まれていること
	req = httptest.NewRequest("GET", fmt.Sprintf("/partials/items?project_id=%d", pID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("View project failed: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "最新プロジェクト名") {
		t.Errorf("expected '最新プロジェクト名' in view")
	}
	if !strings.Contains(body, "edit-project-btn") || !strings.Contains(body, "project-title-edit") {
		t.Errorf("expected edit button and inline edit form in project header")
	}

	// 5. プロジェクト一覧ビューの表示確認: 一覧カード内にも編集ボタンが含まれていること
	req = httptest.NewRequest("GET", "/partials/items?view=projects&project_status=all", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("View projects list failed: %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, fmt.Sprintf("project-card-edit-%d", pID)) {
		t.Errorf("expected project card inline edit in projects list")
	}

	// 6. 空の名前で更新しようとした場合に400エラーが返ること
	form = url.Values{}
	form.Set("name", "   ")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/projects/%d", pID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty project name, got: %d", rec.Code)
	}
}

func TestCleanPathInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Trims spaces and double quotes",
			input:    `  "C:\Users\ma\Desktop\some file.png"  `,
			expected: `C:\Users\ma\Desktop\some file.png`,
		},
		{
			name:     "Trims single quotes",
			input:    `'/Users/ma/Desktop/test.txt'`,
			expected: `/Users/ma/Desktop/test.txt`,
		},
		{
			name:     "Replaces half-width yen with backslash",
			input:    `¥¥server¥some¥file.txt`,
			expected: `\\server\some\file.txt`,
		},
		{
			name:     "Replaces full-width yen with backslash",
			input:    `￥￥server￥share￥file.txt`,
			expected: `\\server\share\file.txt`,
		},
		{
			name:     "Normalizes file:/// scheme for Unix",
			input:    `file:///Users/ma/Pictures/photo.jpg`,
			expected: `/Users/ma/Pictures/photo.jpg`,
		},
		{
			name:     "Normalizes file:/// scheme for Windows",
			input:    `file:///C:/Users/ma/Desktop/doc.pdf`,
			expected: `C:/Users/ma/Desktop/doc.pdf`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := CleanPathInput(tt.input)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestOpenAttachment_PathCheckAndErrors(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. テスト用アイテムを作成
	form := url.Values{}
	form.Set("title", "添付オープン機能のテスト")
	req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// アイテムIDを取得するため詳細を直接確認（ID=1）
	itemID := int64(1)

	// 2. 存在しないパス（空白入り）を添付として登録
	nonExistentPath := `"/Users/ma/Deskstop/2026-10-24 9.02.30.png"`
	form = url.Values{}
	form.Set("target", nonExistentPath)
	form.Set("label", "存在しないファイル")
	req = httptest.NewRequest("POST", fmt.Sprintf("/items/%d/attachments", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Add attachment failed: %d", rec.Code)
	}

	attID := int64(1)

	// 3. 存在しないパスを開こうとした場合、404 Not Found と分かりやすいエラーメッセージが返ること
	req = httptest.NewRequest("POST", fmt.Sprintf("/attachments/%d/open", attID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent file, got: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ファイルまたはフォルダーが見つかりません") {
		t.Errorf("expected error message about missing file, got: %s", rec.Body.String())
	}

	// 4. 実在する一時ファイルを作成して添付し、パスチェックが正常に通ることを確認
	var openedTarget string
	origOpenFileFunc := openFileFunc
	openFileFunc = func(target string) error {
		openedTarget = target
		return nil
	}
	defer func() {
		openFileFunc = origOpenFileFunc
	}()

	tmpFile, err := os.CreateTemp("", "gtd_test_open_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("hello GTD")
	tmpFile.Close()

	form = url.Values{}
	form.Set("target", tmpFile.Name())
	form.Set("label", "実在する一時ファイル")
	req = httptest.NewRequest("POST", fmt.Sprintf("/items/%d/attachments", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Add attachment for temp file failed: %d", rec.Code)
	}

	attID2 := int64(2)

	// 実行
	req = httptest.NewRequest("POST", fmt.Sprintf("/attachments/%d/open", attID2), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// 実在ファイルを開いた場合、エラーなく200 OKが返り、トースト通知ヘッダが含まれること
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for existing file, got: %d (%s)", rec.Code, rec.Body.String())
	}
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "attachmentOpened") {
		t.Errorf("expected HX-Trigger attachmentOpened header, got: %s", trigger)
	}
	// HTTPヘッダー値がすべてASCII（文字化け・RFC違反防止）であること
	for i := 0; i < len(trigger); i++ {
		if trigger[i] > 127 {
			t.Errorf("HX-Trigger header contains non-ASCII byte at %d: %x (header: %s)", i, trigger[i], trigger)
			break
		}
	}
	if openedTarget != tmpFile.Name() {
		t.Errorf("expected opened target %q, got %q", tmpFile.Name(), openedTarget)
	}
}

func TestStartDateAndScheduledViewRendering(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. クイックキャプチャで未来開始・未来期日タスクを作成 (Smart Add: ~明日 ^fri)
	form := url.Values{}
	form.Set("title", "未来のタスク ~明日 ^fri #PC")
	form.Set("status", "next")
	req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Create item failed: %d", rec.Code)
	}

	// 2. Next Actions ビューを取得: 未来開始タスクは非表示であること
	req = httptest.NewRequest("GET", "/partials/items?view=next", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Get next actions failed: %d", rec.Code)
	}
	nextHtml := rec.Body.String()
	if strings.Contains(nextHtml, "未来のタスク") {
		t.Errorf("expected future item to be hidden in next actions, but it was found in: %s", nextHtml)
	}

	// 3. 今日から開始可能なタスクを作成
	form = url.Values{}
	form.Set("title", "今日からできるタスク ~today")
	form.Set("status", "next")
	req = httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Create today item failed: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/partials/items?view=next", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "今日からできるタスク") {
		t.Errorf("expected today item to appear in next actions")
	}

	// 4. 期限・予定 (scheduled) ビューを取得
	req = httptest.NewRequest("GET", "/partials/items?view=scheduled", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Get scheduled items failed: %d", rec.Code)
	}
	schedHtml := rec.Body.String()
	if !strings.Contains(schedHtml, "期限・予定") {
		t.Errorf("expected title '期限・予定', got: %s", schedHtml)
	}
	if !strings.Contains(schedHtml, "未来のタスク") {
		t.Errorf("expected '未来のタスク' in scheduled view, got: %s", schedHtml)
	}

	// 5. サイドバーを取得し、「期限・予定」ナビゲーション項目が含まれることを確認
	req = httptest.NewRequest("GET", "/partials/sidebar", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Get sidebar failed: %d", rec.Code)
	}
	sidebarHtml := rec.Body.String()
	if !strings.Contains(sidebarHtml, "期限・予定") {
		t.Errorf("expected '期限・予定' in sidebar, got: %s", sidebarHtml)
	}
	if !strings.Contains(sidebarHtml, "/partials/items?view=scheduled") {
		t.Errorf("expected scheduled link in sidebar, got: %s", sidebarHtml)
	}

	// 6. 詳細ペインの表示と更新テスト
	req = httptest.NewRequest("GET", "/items/1/detail", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Get item detail failed: %d", rec.Code)
	}
	detailHtml := rec.Body.String()
	if !strings.Contains(detailHtml, "name=\"start_date\"") {
		t.Errorf("expected start_date input in detail pane, got: %s", detailHtml)
	}
	if !strings.Contains(detailHtml, "Next Actions から保留") {
		t.Errorf("expected deferral notice in detail pane, got: %s", detailHtml)
	}

	// 詳細ペインからの更新
	form = url.Values{}
	form.Set("title", "未来のタスク（更新後）")
	form.Set("status", "next")
	form.Set("start_date", "2026-11-01")
	form.Set("due_date", "2026-11-05")
	req = httptest.NewRequest("PUT", "/items/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Update item failed: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/items/1/detail", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	updatedDetail := rec.Body.String()
	if !strings.Contains(updatedDetail, "2026-11-01") {
		t.Errorf("expected updated start_date 2026-11-01 in detail pane")
	}
}

func TestSearchHandler(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. タスク追加
	form := url.Values{}
	form.Set("title", "オムニサーチの設計書作成")
	req := httptest.NewRequest("POST", "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Create item failed: %d", rec.Code)
	}

	// 2. 添付ファイル追加
	attForm := url.Values{}
	attForm.Set("type", "path")
	attForm.Set("target", "C:\\projects\\omnisearch_spec.pdf")
	attForm.Set("label", "オムニ仕様PDF")
	req = httptest.NewRequest("POST", "/items/1/attachments", strings.NewReader(attForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Add attachment failed: %d", rec.Code)
	}

	// 3. 空クエリでの検索リクエスト -> 空レスポンス
	req = httptest.NewRequest("GET", "/search?q=", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Search empty failed: %d", rec.Code)
	}
	if rec.Body.String() != "" {
		t.Errorf("expected empty body for empty query, got: %s", rec.Body.String())
	}

	// 4. クエリ "オムニ" で検索 -> タスクと添付ファイルの両方がヒット
	req = httptest.NewRequest("GET", "/search?q=オムニ", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Search query failed: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "オムニサーチの設計書作成") {
		t.Errorf("expected task title in search results, got: %s", body)
	}
	if !strings.Contains(body, "オムニ仕様PDF") {
		t.Errorf("expected attachment label in search results, got: %s", body)
	}
	if !strings.Contains(body, "hx-get=\"/items/1/detail\"") {
		t.Errorf("expected hx-get detail link in search results, got: %s", body)
	}

	// 5. 該当なしのクエリ
	req = httptest.NewRequest("GET", "/search?q=nonexistentkeyword12345", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Search nonexistent failed: %d", rec.Code)
	}
	noResultBody := rec.Body.String()
	if !strings.Contains(noResultBody, "一致する結果は見つかりませんでした") {
		t.Errorf("expected empty state message, got: %s", noResultBody)
	}
}

