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
		"../../templates/partials/attachment_item.html",
		"../../templates/partials/review_step.html",
		"../../templates/partials/review_project_card.html",
		"../../templates/partials/search_results.html",
		"../../templates/partials/launchers_modal.html",
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
	mux.HandleFunc("GET /attachments/{id}", h.GetAttachmentItem)
	mux.HandleFunc("GET /attachments/{id}/edit", h.GetAttachmentEditForm)
	mux.HandleFunc("PUT /attachments/{id}", h.UpdateAttachment)
	mux.HandleFunc("POST /attachments/{id}/open", h.OpenAttachment)
	mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)
	mux.HandleFunc("GET /partials/launchers", h.ServeLauncherModal)
	mux.HandleFunc("POST /launchers", h.CreateLauncher)
	mux.HandleFunc("GET /launchers/{id}/edit", h.GetLauncherEditForm)
	mux.HandleFunc("PUT /launchers/{id}", h.UpdateLauncher)
	mux.HandleFunc("DELETE /launchers/{id}", h.DeleteLauncher)
	mux.HandleFunc("POST /launchers/{id}/launch", h.LaunchApp)
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("PUT /projects/{id}", h.UpdateProject)
	mux.HandleFunc("PATCH /projects/{id}/status", h.UpdateProjectStatus)
	mux.HandleFunc("DELETE /projects/{id}", h.DeleteProject)
	mux.HandleFunc("POST /contexts", h.CreateContext)
	mux.HandleFunc("PUT /contexts/{id}", h.UpdateContext)
	mux.HandleFunc("DELETE /contexts/{id}", h.DeleteContext)
	mux.HandleFunc("GET /review/step/{step}", h.ReviewStep)
	mux.HandleFunc("POST /review/projects/{id}/reviewed", h.MarkProjectReviewed)
	mux.HandleFunc("POST /review/projects/{id}/items", h.AddProjectItemInReview)
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

func TestWeeklyReviewStep1ItemTriage(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create an item in inbox
	item := &model.Item{
		Title:  "Review this inbox item",
		Status: "inbox",
	}
	itemID, err := db.CreateItem(h.DB, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 2. Fetch Step 1
	req := httptest.NewRequest("GET", "/review/step/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 failed: %d", rec.Code)
	}
	step1HTML := rec.Body.String()
	if !strings.Contains(step1HTML, "Review this inbox item") {
		t.Fatalf("expected item title in Step 1, got: %s", step1HTML)
	}

	// Verify buttons have hx-patch, hx-target, and hx-swap="delete" for continuous in-place triage
	patchAttr := fmt.Sprintf(`hx-patch="/items/%d/status"`, itemID)
	if !strings.Contains(step1HTML, patchAttr) {
		t.Errorf("expected %s in Step 1 buttons", patchAttr)
	}
	targetAttr := fmt.Sprintf(`hx-target="#review-card-%d"`, itemID)
	if !strings.Contains(step1HTML, targetAttr) {
		t.Errorf("expected %s in Step 1 buttons", targetAttr)
	}
	if !strings.Contains(step1HTML, `hx-swap="delete"`) {
		t.Errorf("expected hx-swap=\"delete\" in Step 1 buttons")
	}
	cardIDAttr := fmt.Sprintf(`id="review-card-%d"`, itemID)
	if !strings.Contains(step1HTML, cardIDAttr) {
		t.Errorf("expected %s on item card in Step 1", cardIDAttr)
	}
	// Verify the buttons do NOT contain hx-get
	for _, line := range strings.Split(step1HTML, "\n") {
		if strings.Contains(line, "hx-patch=") || strings.Contains(line, "hx-delete=") {
			if strings.Contains(line, "hx-get=") {
				t.Errorf("found conflicting hx-get on line with mutation verb: %s", line)
			}
		}
	}

	// 3. Triage item to "next" using PATCH /items/{id}/status
	form := url.Values{}
	form.Set("status", "next")
	req = httptest.NewRequest("PATCH", fmt.Sprintf("/items/%d/status", itemID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status failed: %d", rec.Code)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "itemChanged") {
		t.Errorf("expected HX-Trigger to contain 'itemChanged', got: %s", trigger)
	}

	// 4. Fetch Step 1 again: item should be gone and "Inbox Zero 達成！" should appear
	req = httptest.NewRequest("GET", "/review/step/1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 reload failed: %d", rec.Code)
	}
	step1After := rec.Body.String()
	if strings.Contains(step1After, "Review this inbox item") {
		t.Errorf("item should have been removed from Step 1, but still present")
	}
	if !strings.Contains(step1After, "Inbox Zero 達成！") {
		t.Errorf("expected Inbox Zero in Step 1 after triaging all items, got: %s", step1After)
	}
}

func TestWeeklyReviewStep1ContinuousTriageStructure(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create two items in Inbox
	it1 := &model.Item{Title: "Inbox Item 1", Status: "inbox"}
	id1, err := db.CreateItem(h.DB, it1)
	if err != nil {
		t.Fatalf("CreateItem 1 failed: %v", err)
	}
	it2 := &model.Item{Title: "Inbox Item 2", Status: "inbox"}
	id2, err := db.CreateItem(h.DB, it2)
	if err != nil {
		t.Fatalf("CreateItem 2 failed: %v", err)
	}

	// 2. Fetch Step 1
	req := httptest.NewRequest("GET", "/review/step/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Step 1 request failed: %d", rec.Code)
	}

	html := rec.Body.String()

	// Verify both cards have distinct review-card IDs
	card1 := fmt.Sprintf(`id="review-card-%d"`, id1)
	card2 := fmt.Sprintf(`id="review-card-%d"`, id2)
	if !strings.Contains(html, card1) || !strings.Contains(html, card2) {
		t.Errorf("expected both review cards in Step 1: %s, %s", card1, card2)
	}

	// Verify both have hx-swap="delete" targeting their respective cards
	target1 := fmt.Sprintf(`hx-target="#review-card-%d"`, id1)
	target2 := fmt.Sprintf(`hx-target="#review-card-%d"`, id2)
	if !strings.Contains(html, target1) || !strings.Contains(html, target2) {
		t.Errorf("expected hx-target for both items: %s, %s", target1, target2)
	}

	// Verify items list container and remaining counter elements exist
	if !strings.Contains(html, `id="step1-items-list"`) {
		t.Errorf("expected step1-items-list in Step 1")
	}
	if !strings.Contains(html, `id="step1-remaining-count"`) {
		t.Errorf("expected step1-remaining-count in Step 1")
	}
	if !strings.Contains(html, `id="step1-tab-badge"`) {
		t.Errorf("expected step1-tab-badge in Step 1")
	}
	if !strings.Contains(html, `id="step1-zero-state"`) {
		t.Errorf("expected step1-zero-state in Step 1")
	}

	// 3. Verify item list partial has self-contained items-view-container
	req = httptest.NewRequest("GET", "/partials/items?view=next", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("next view request failed: %d", rec.Code)
	}
	nextHTML := rec.Body.String()
	if !strings.Contains(nextHTML, `id="items-view-container"`) {
		t.Errorf("expected items-view-container in next view partial")
	}
	if !strings.Contains(nextHTML, `hx-get="/partials/items?view=next"`) {
		t.Errorf("expected hx-get for next view in items-view-container")
	}
	if !strings.Contains(nextHTML, `hx-trigger="itemChanged from:document`) {
		t.Errorf("expected itemChanged trigger in items-view-container")
	}

	// 4. Verify layout.html sets from:document on sidebar-pane and does NOT put itemChanged trigger on main-content-pane
	req = httptest.NewRequest("GET", "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("layout request failed: %d", rec.Code)
	}
	layoutHTML := rec.Body.String()
	if !strings.Contains(layoutHTML, `id="sidebar-pane" class="sidebar-pane" hx-get="/partials/sidebar" hx-trigger="itemChanged from:document`) {
		t.Errorf("expected sidebar-pane to use from:document for reliable updates on deleted elements")
	}
	if strings.Contains(layoutHTML, `<main id="main-content-pane" class="main-content-pane" hx-trigger=`) {
		t.Errorf("main-content-pane should NOT have hx-trigger attribute directly")
	}
}

func TestWeeklyReviewStep2MarkProjectReviewed(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create an active project
	p := &model.Project{Name: "Review Test Project", Status: "active"}
	pID, err := db.CreateProject(h.DB, p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. Fetch Step 2 and verify button markup
	req := httptest.NewRequest("GET", "/review/step/2", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Step 2 request failed: %d", rec.Code)
	}
	html := rec.Body.String()
	cardID := fmt.Sprintf(`id="project-review-card-%d"`, pID)
	targetAttr := fmt.Sprintf(`hx-target="#project-review-card-%d"`, pID)
	postAttr := fmt.Sprintf(`hx-post="/review/projects/%d/reviewed"`, pID)

	if !strings.Contains(html, cardID) {
		t.Errorf("expected %s in Step 2 html", cardID)
	}
	if !strings.Contains(html, targetAttr) {
		t.Errorf("expected %s in Step 2 html", targetAttr)
	}
	if !strings.Contains(html, postAttr) {
		t.Errorf("expected %s in Step 2 html", postAttr)
	}
	if !strings.Contains(html, `hx-swap="outerHTML"`) {
		t.Errorf("expected hx-swap=\"outerHTML\" in Step 2 html")
	}

	// 3. Post to MarkProjectReviewed
	req = httptest.NewRequest("POST", fmt.Sprintf("/review/projects/%d/reviewed", pID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("MarkProjectReviewed request failed: %d", rec.Code)
	}

	cardHTML := rec.Body.String()
	if !strings.Contains(cardHTML, "✓ 点検済") {
		t.Errorf("expected '✓ 点検済' in returned project card, got: %s", cardHTML)
	}
	if !strings.Contains(cardHTML, "btn-reviewed") {
		t.Errorf("expected 'btn-reviewed' class in returned project card")
	}
	if !strings.Contains(cardHTML, "前回点検:") {
		t.Errorf("expected '前回点検:' in returned project card")
	}

	// 4. Verify DB was updated
	proj, err := db.GetProjectByID(h.DB, pID)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if proj.LastReviewedAt == nil {
		t.Fatalf("expected LastReviewedAt to be set")
	}

	// 5. Add next action in Step 2
	formData := strings.NewReader("title=My+Next+Step")
	req = httptest.NewRequest("POST", fmt.Sprintf("/review/projects/%d/items", pID), formData)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("AddProjectItemInReview failed: %d", rec.Code)
	}
	afterAddHTML := rec.Body.String()
	if !strings.Contains(afterAddHTML, "Next Action: <strong>1</strong> 件") {
		t.Errorf("expected Next Action count to become 1, got: %s", afterAddHTML)
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

	// 6. Verify URL attribute is rendered in partials container
	if !strings.Contains(body, `hx-get="/partials/items?view=contexts"`) {
		t.Errorf("expected hx-get attribute in partials view, got: %s", body)
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

func TestLauncherHandlers(t *testing.T) {
	_, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. GET /partials/launchers (初期空状態のモーダル描画)
	req := httptest.NewRequest("GET", "/partials/launchers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "アプリケーションランチャー") {
		t.Errorf("expected modal title in body, got: %s", body)
	}
	if !strings.Contains(body, "ランチャーがまだ登録されていません") {
		t.Errorf("expected empty state in body, got: %s", body)
	}

	// 2. POST /launchers (新規登録)
	// 一時ファイルを作成してパスとして使用
	tmpFile, err := os.CreateTemp("", "gtd_test_launcher_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpPath := tmpFile.Name()

	form := url.Values{}
	form.Set("name", "テストメモ帳")
	form.Set("target", tmpPath)
	form.Set("description", "メモ 記録")
	req = httptest.NewRequest("POST", "/launchers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Errorf("expected HX-Trigger to contain showToast, got %s", trigger)
	}
	createdBody := rec.Body.String()
	if !strings.Contains(createdBody, "テストメモ帳") {
		t.Errorf("expected created launcher in modal body, got: %s", createdBody)
	}

	// 3. GET /launchers/1/edit (編集モーダル取得)
	req = httptest.NewRequest("GET", "/launchers/1/edit", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	editBody := rec.Body.String()
	if !strings.Contains(editBody, "ランチャーの編集") {
		t.Errorf("expected edit mode in body, got: %s", editBody)
	}

	// 4. PUT /launchers/1 (更新)
	formUpdate := url.Values{}
	formUpdate.Set("name", "更新テストメモ帳")
	formUpdate.Set("target", tmpPath)
	formUpdate.Set("type", "file")
	formUpdate.Set("description", "更新メモ")
	req = httptest.NewRequest("PUT", "/launchers/1", strings.NewReader(formUpdate.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	updatedBody := rec.Body.String()
	if !strings.Contains(updatedBody, "更新テストメモ帳") {
		t.Errorf("expected updated launcher name in body, got: %s", updatedBody)
	}

	// 5. POST /launchers/1/launch (正常起動テスト)
	// モックオープン関数を設定して安全にテスト
	originalOpen := openFileFunc
	openedTarget := ""
	openFileFunc = func(target string) error {
		openedTarget = target
		return nil
	}
	defer func() { openFileFunc = originalOpen }()

	req = httptest.NewRequest("POST", "/launchers/1/launch", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for launch, got %d: %s", rec.Code, rec.Body.String())
	}
	if openedTarget != tmpPath {
		t.Errorf("expected openFileFunc to be called with %q, got %q", tmpPath, openedTarget)
	}
	if trigger := rec.Header().Get("HX-Trigger"); !strings.Contains(trigger, "showToast") {
		t.Errorf("expected HX-Trigger to contain showToast, got %s", trigger)
	}

	// 6. POST /launchers/2/launch (存在しないランチャーID -> 404)
	req = httptest.NewRequest("POST", "/launchers/999/launch", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for nonexistent ID, got %d", rec.Code)
	}

	// 7. パスが存在しない場合の起動テスト -> 404 Not Found & パスエラーメッセージ
	formBad := url.Values{}
	formBad.Set("name", "壊れたパスアプリ")
	formBad.Set("target", "/nonexistent/fake/file/path_12345.exe")
	req = httptest.NewRequest("POST", "/launchers", strings.NewReader(formBad.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for creating launcher, got %d", rec.Code)
	}

	req = httptest.NewRequest("POST", "/launchers/2/launch", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for missing target file, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "見つかりません") {
		t.Errorf("expected error message to mention not found, got %s", rec.Body.String())
	}

	// 8. DELETE /launchers/1 (削除)
	req = httptest.NewRequest("DELETE", "/launchers/1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for delete, got %d", rec.Code)
	}
	deleteBody := rec.Body.String()
	if strings.Contains(deleteBody, "更新テストメモ帳") {
		t.Errorf("expected deleted launcher to disappear, got: %s", deleteBody)
	}
}

func TestItemDetailProjectAssignmentAndSidebarCount(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. プロジェクトを作成
	pID, err := db.CreateProject(h.DB, &model.Project{
		Name:   "新ポータル開発",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. アイテムを作成 (Inbox)
	itID, err := db.CreateItem(h.DB, &model.Item{
		Title:  "要件定義ドラフト作成",
		Status: "inbox",
	})
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 3. サイドバーの初期表示確認（タスク0件）
	req := httptest.NewRequest("GET", "/partials/sidebar", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/sidebar failed: %d", rec.Code)
	}
	sidebarHTML := rec.Body.String()
	if !strings.Contains(sidebarHTML, "新ポータル開発") {
		t.Fatalf("expected project '新ポータル開発' in sidebar")
	}
	if !strings.Contains(sidebarHTML, "Next Actionがありません（タスク0件）") {
		t.Errorf("expected 0 items stalled tooltip in sidebar, got: %s", sidebarHTML)
	}

	// 4. アイテム詳細からプロジェクトに紐付け (PUT /items/{id})
	form := url.Values{}
	form.Set("title", "要件定義ドラフト作成")
	form.Set("project_id", fmt.Sprintf("%d", pID))
	form.Set("status", "inbox")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/items/%d", itID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /items/{id} failed: %d", rec.Code)
	}

	// HX-Trigger に itemChanged が含まれること
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "itemChanged") {
		t.Errorf("expected 'itemChanged' in HX-Trigger, got: %s", trigger)
	}

	// 5. サイドバーを再取得し、0/1 バッジ（Next Action 0件 / 合計 1件）が表示されること
	req = httptest.NewRequest("GET", "/partials/sidebar", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/sidebar failed: %d", rec.Code)
	}
	sidebarHTML = rec.Body.String()
	if !strings.Contains(sidebarHTML, "0/1") {
		t.Errorf("expected '0/1' badge for project in sidebar, got: %s", sidebarHTML)
	}
	if !strings.Contains(sidebarHTML, "タスク 1件（Next Action 0件）") {
		t.Errorf("expected tooltip 'タスク 1件（Next Action 0件）', got: %s", sidebarHTML)
	}

	// 6. アイテムのステータスを next に変更 (PATCH /items/{id}/status)
	statusForm := url.Values{}
	statusForm.Set("status", "next")
	req = httptest.NewRequest("PATCH", fmt.Sprintf("/items/%d/status", itID), strings.NewReader(statusForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /items/{id}/status failed: %d", rec.Code)
	}
	statusTrigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(statusTrigger, "itemChanged") {
		t.Errorf("expected 'itemChanged' in status trigger, got: %s", statusTrigger)
	}

	// 7. サイドバーを再取得し、Next Action 1件（通常バッジ "1"）となること
	req = httptest.NewRequest("GET", "/partials/sidebar", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/sidebar failed: %d", rec.Code)
	}
	sidebarHTML = rec.Body.String()
	if !strings.Contains(sidebarHTML, "Next Action 1件") {
		t.Errorf("expected 'Next Action 1件' in sidebar, got: %s", sidebarHTML)
	}
}

func TestProjectStatusSectionsAndDetailContext(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. プロジェクトを作成
	projForm := url.Values{}
	projForm.Set("name", "GTD改善プロジェクト")
	projForm.Set("description", "UI/UX改善の完了")
	req := httptest.NewRequest("POST", "/projects", strings.NewReader(projForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /projects failed: %d", rec.Code)
	}

	projs, _ := db.GetProjects(h.DB, "active")
	if len(projs) < 1 {
		t.Fatalf("expected at least 1 project, got %d", len(projs))
	}
	var projID int64
	for _, p := range projs {
		if p.Name == "GTD改善プロジェクト" {
			projID = p.ID
			break
		}
	}
	if projID == 0 {
		t.Fatalf("could not find created project")
	}

	// 2. Next, Waiting, Someday, Done のタスクを作成
	statuses := []string{"next", "waiting", "someday", "done"}
	taskTitles := []string{"UI設計書を作成する", "B氏のレビュー待ち", "将来の機能拡張アイデア", "要件定義の完了"}
	var taskIDs []int64

	for i, st := range statuses {
		itemForm := url.Values{}
		itemForm.Set("title", taskTitles[i])
		itemForm.Set("project_id", fmt.Sprintf("%d", projID))
		itemForm.Set("status", st)
		req = httptest.NewRequest("POST", "/items", strings.NewReader(itemForm.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /items with status %s failed: %d", st, rec.Code)
		}
	}

	items, _ := db.GetItems(h.DB, model.ItemFilter{ProjectID: &projID})
	for _, it := range items {
		taskIDs = append(taskIDs, it.ID)
	}

	// 3. プロジェクト一覧ビュー (GET /partials/items?project_id=...)
	req = httptest.NewRequest("GET", fmt.Sprintf("/partials/items?project_id=%d", projID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/items?project_id failed: %d", rec.Code)
	}
	projectHTML := rec.Body.String()

	// セクション見出しの検証
	if !strings.Contains(projectHTML, "Next Actions") {
		t.Errorf("expected 'Next Actions' section in HTML, got:\n%s", projectHTML)
	}
	if !strings.Contains(projectHTML, "Waiting For") {
		t.Errorf("expected 'Waiting For' section in HTML, got:\n%s", projectHTML)
	}
	if !strings.Contains(projectHTML, "Someday / Ideas") {
		t.Errorf("expected 'Someday / Ideas' section in HTML, got:\n%s", projectHTML)
	}
	if !strings.Contains(projectHTML, "完了済み") {
		t.Errorf("expected '完了済み' section in HTML, got:\n%s", projectHTML)
	}

	// 進捗バーとフィルタチップの検証
	if !strings.Contains(projectHTML, "project-progress-wrap") {
		t.Errorf("expected 'project-progress-wrap' in HTML")
	}
	if !strings.Contains(projectHTML, "chip-next") || !strings.Contains(projectHTML, "chip-waiting") {
		t.Errorf("expected status filter chips in HTML")
	}

	// カード上のステータスバッジとクイックトグルボタンの検証
	if !strings.Contains(projectHTML, "tag-status-next") {
		t.Errorf("expected 'tag-status-next' badge in HTML")
	}
	if !strings.Contains(projectHTML, "tag-status-waiting") {
		t.Errorf("expected 'tag-status-waiting' badge in HTML")
	}
	if !strings.Contains(projectHTML, "item-quick-toggle-btn") {
		t.Errorf("expected 'item-quick-toggle-btn' in HTML")
	}

	// 4. ステータス指定フィルタリング (GET /partials/items?project_id=...&item_status=waiting)
	req = httptest.NewRequest("GET", fmt.Sprintf("/partials/items?project_id=%d&item_status=waiting", projID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET with item_status=waiting failed: %d", rec.Code)
	}
	filteredHTML := rec.Body.String()
	if !strings.Contains(filteredHTML, "Waiting For") {
		t.Errorf("expected waiting section in filtered HTML")
	}
	if strings.Contains(filteredHTML, "Next Actions") {
		t.Errorf("expected next section to be hidden when item_status=waiting")
	}

	// 5. 詳細ペインのコンテキストバーと大見出し検証 (GET /items/{id}/detail)
	if len(taskIDs) > 0 {
		firstID := taskIDs[0]
		req = httptest.NewRequest("GET", fmt.Sprintf("/items/%d/detail", firstID), nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /items/{id}/detail failed: %d", rec.Code)
		}
		detailHTML := rec.Body.String()

		// ロケーションコンテキストバーの検証
		if !strings.Contains(detailHTML, "detail-context-bar") {
			t.Errorf("expected 'detail-context-bar' in detail pane HTML")
		}
		if !strings.Contains(detailHTML, "detail-project-crumb") {
			t.Errorf("expected 'detail-project-crumb' in detail pane HTML")
		}
		if !strings.Contains(detailHTML, "detail-title-input") {
			t.Errorf("expected 'detail-title-input' in detail pane HTML")
		}
	}
}

func TestAttachmentRenameFlow(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. アイテムと各種添付（画像、URL、パス）を作成
	item := &model.Item{Title: "添付ファイル名称変更テストタスク", Status: "inbox"}
	itemID, err := db.CreateItem(h.DB, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	imgAtt := &model.Attachment{
		ItemID: itemID,
		Label:  "スクショ 10/24 19:29",
		Target: "clip_20261024_192951_0396eb1c.png",
		Type:   "image",
	}
	imgID, err := db.CreateAttachment(h.DB, imgAtt)
	if err != nil {
		t.Fatalf("CreateAttachment (image) failed: %v", err)
	}

	pathAtt := &model.Attachment{
		ItemID: itemID,
		Label:  "旧設計書",
		Target: `C:\projects\spec.pdf`,
		Type:   "path",
	}
	pathID, err := db.CreateAttachment(h.DB, pathAtt)
	if err != nil {
		t.Fatalf("CreateAttachment (path) failed: %v", err)
	}

	// 2. GET /attachments/{id}/edit（編集フォームの取得）
	req := httptest.NewRequest("GET", fmt.Sprintf("/attachments/%d/edit", imgID), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /attachments/{id}/edit, got %d", rec.Code)
	}
	editHTML := rec.Body.String()
	if !strings.Contains(editHTML, fmt.Sprintf("hx-put=\"/attachments/%d\"", imgID)) {
		t.Errorf("expected form with hx-put to attachment %d, got: %s", imgID, editHTML)
	}
	if !strings.Contains(editHTML, "スクショ 10/24 19:29") {
		t.Errorf("expected input value 'スクショ 10/24 19:29' in form, got: %s", editHTML)
	}
	if !strings.Contains(editHTML, "キャンセル") {
		t.Errorf("expected cancel button in edit form, got: %s", editHTML)
	}

	// 3. PUT /attachments/{id}（名称更新）
	form := url.Values{}
	form.Set("label", "ログイン画面のエラー")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/attachments/%d", imgID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT /attachments/{id}, got %d: %s", rec.Code, rec.Body.String())
	}
	updateHTML := rec.Body.String()
	if !strings.Contains(updateHTML, "ログイン画面のエラー") {
		t.Errorf("expected updated label in response HTML, got: %s", updateHTML)
	}
	if strings.Contains(updateHTML, "<form hx-put=") {
		t.Errorf("expected view mode HTML (not form) after update, got: %s", updateHTML)
	}

	// HX-Trigger 検証 (itemChanged)
	trigger := rec.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "itemChanged") {
		t.Errorf("expected itemChanged in HX-Trigger header, got: %s", trigger)
	}

	// DB内レコード検証
	dbAtt, err := db.GetAttachmentByID(h.DB, imgID)
	if err != nil {
		t.Fatalf("GetAttachmentByID failed: %v", err)
	}
	if dbAtt.Label != "ログイン画面のエラー" {
		t.Errorf("expected DB label 'ログイン画面のエラー', got %q", dbAtt.Label)
	}

	// 4. GET /attachments/{id}（通常表示・キャンセル時エンドポイント）
	req = httptest.NewRequest("GET", fmt.Sprintf("/attachments/%d", imgID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /attachments/{id}, got %d", rec.Code)
	}
	viewHTML := rec.Body.String()
	if !strings.Contains(viewHTML, "ログイン画面のエラー") {
		t.Errorf("expected label in view HTML, got: %s", viewHTML)
	}
	if !strings.Contains(viewHTML, fmt.Sprintf("hx-get=\"/attachments/%d/edit\"", imgID)) {
		t.Errorf("expected edit button in view HTML, got: %s", viewHTML)
	}

	// 5. 空文字送信時の挙動
	// 5-1. 画像: 空白のみを送信した場合は現在の名称が維持されること
	emptyForm := url.Values{}
	emptyForm.Set("label", "   ")
	req = httptest.NewRequest("PUT", fmt.Sprintf("/attachments/%d", imgID), strings.NewReader(emptyForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT with empty label on image, got %d", rec.Code)
	}
	dbAttImg, _ := db.GetAttachmentByID(h.DB, imgID)
	if dbAttImg.Label != "ログイン画面のエラー" {
		t.Errorf("expected image to keep readable label, got %q", dbAttImg.Label)
	}

	// 5-2. パス: 空白を送信した場合はラベルがクリアされ、CleanTargetNameがファイル名にフォールバックすること
	req = httptest.NewRequest("PUT", fmt.Sprintf("/attachments/%d", pathID), strings.NewReader(emptyForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT with empty label on path, got %d", rec.Code)
	}
	dbAttPath, _ := db.GetAttachmentByID(h.DB, pathID)
	if dbAttPath.Label != "" {
		t.Errorf("expected path label to be empty, got %q", dbAttPath.Label)
	}
	if dbAttPath.CleanTargetName() != "spec.pdf" {
		t.Errorf("expected CleanTargetName fallback to 'spec.pdf', got %q", dbAttPath.CleanTargetName())
	}

	// 6. 異常系テスト
	// 6-1. 不正なID
	req = httptest.NewRequest("GET", "/attachments/invalid/edit", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ID, got %d", rec.Code)
	}

	// 6-2. 存在しないID
	req = httptest.NewRequest("GET", "/attachments/999999/edit", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent ID, got %d", rec.Code)
	}

	req = httptest.NewRequest("PUT", "/attachments/999999", strings.NewReader("label=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent ID PUT, got %d", rec.Code)
	}

	// 7. アイテム詳細画面（GET /items/{id}/detail）に添付一覧と名称変更ボタンが含まれていること
	req = httptest.NewRequest("GET", fmt.Sprintf("/items/%d/detail", itemID), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /items/{id}/detail, got %d", rec.Code)
	}
	detailHTML := rec.Body.String()
	if !strings.Contains(detailHTML, "ログイン画面のエラー") {
		t.Errorf("expected renamed attachment label in detail HTML")
	}
	if !strings.Contains(detailHTML, fmt.Sprintf("hx-get=\"/attachments/%d/edit\"", imgID)) {
		t.Errorf("expected rename button in detail HTML")
	}
}

