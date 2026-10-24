package db

import (
	"path/filepath"
	"testing"
	"time"

	"gtd/internal/model"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	return dbPath
}

func TestDBAndItemLifecycle(t *testing.T) {
	dbPath := setupTestDB(t)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 1. Initial counts
	counts, err := GetSidebarCounts(db)
	if err != nil {
		t.Fatalf("GetSidebarCounts failed: %v", err)
	}
	if counts.Inbox != 0 {
		t.Errorf("expected 0 inbox items, got %d", counts.Inbox)
	}

	// 2. Create Inbox item
	item := &model.Item{
		Title: "Buy printer ink",
		Body:  "Check model number first",
	}
	id, err := CreateItem(db, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	counts, _ = GetSidebarCounts(db)
	if counts.Inbox != 1 {
		t.Errorf("expected 1 inbox item, got %d", counts.Inbox)
	}

	// 3. Move to Next Action
	_, err = UpdateItemStatus(db, id, "next")
	if err != nil {
		t.Fatalf("UpdateItemStatus failed: %v", err)
	}

	counts, _ = GetSidebarCounts(db)
	if counts.Inbox != 0 || counts.Next != 1 {
		t.Errorf("expected 0 inbox and 1 next, got inbox=%d next=%d", counts.Inbox, counts.Next)
	}

	// 4. Test Recurrence: Create a monthly recurring item with due date
	baseDate := time.Now().Format("2006-01-02")
	monthlyItem := &model.Item{
		Title:      "Monthly budget report",
		Status:     "next",
		DueDate:    &baseDate,
		Recurrence: "monthly",
	}
	mID, err := CreateItem(db, monthlyItem)
	if err != nil {
		t.Fatalf("Create monthly item failed: %v", err)
	}

	// Complete the monthly task
	_, err = UpdateItemStatus(db, mID, "done")
	if err != nil {
		t.Fatalf("UpdateItemStatus to done failed: %v", err)
	}

	// Verify that the completed item is now done with recurrence reset
	completed, err := GetItemByID(db, mID)
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}
	if completed.Status != "done" {
		t.Errorf("expected status 'done', got %s", completed.Status)
	}

	// Verify that a new next action was spawned with the next month's due date
	nextItems, err := GetItems(db, model.ItemFilter{View: "next"})
	if err != nil {
		t.Fatalf("GetItems next failed: %v", err)
	}

	foundNew := false
	for _, it := range nextItems {
		if it.Title == "Monthly budget report" && it.ID != mID {
			foundNew = true
			if it.DueDate == nil || *it.DueDate <= baseDate {
				t.Errorf("expected next due date to be advanced, got %v", it.DueDate)
			}
			if it.Recurrence != "monthly" {
				t.Errorf("expected recurrence 'monthly', got %s", it.Recurrence)
			}
		}
	}
	if !foundNew {
		t.Errorf("new recurring item instance was not spawned")
	}
}

func TestProjectStalledReview(t *testing.T) {
	dbPath := setupTestDB(t)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Create project
	p := &model.Project{
		Name:        "Office Renovation",
		Description: "Refurbish desks and chairs",
	}
	pID, err := CreateProject(db, p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Without next action, project is stalled
	counts, _ := GetSidebarCounts(db)
	if counts.StalledProjects != 1 {
		t.Errorf("expected 1 stalled project, got %d", counts.StalledProjects)
	}

	// Add next action to project
	item := &model.Item{
		ProjectID: &pID,
		Title:     "Call furniture vendor",
		Status:    "next",
	}
	_, err = CreateItem(db, item)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// Now it shouldn't be stalled
	counts, _ = GetSidebarCounts(db)
	if counts.StalledProjects != 0 {
		t.Errorf("expected 0 stalled projects, got %d", counts.StalledProjects)
	}
}

func TestContextCRUD(t *testing.T) {
	dbPath := setupTestDB(t)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 1. Initial seeded contexts (#PC, #会議, #連絡, #5分, #外出)
	ctxs, err := GetContexts(db)
	if err != nil {
		t.Fatalf("GetContexts failed: %v", err)
	}
	if len(ctxs) < 5 {
		t.Fatalf("expected at least 5 default contexts, got %d", len(ctxs))
	}

	// 2. Create Context without # prefix (should auto-prefix #)
	id, err := CreateContext(db, "経理")
	if err != nil {
		t.Fatalf("CreateContext failed: %v", err)
	}
	c, err := GetContextByID(db, id)
	if err != nil {
		t.Fatalf("GetContextByID failed: %v", err)
	}
	if c.Name != "#経理" {
		t.Errorf("expected '#経理', got %q", c.Name)
	}

	// 3. Create duplicate context (should return existing id without error)
	dupID, err := CreateContext(db, "#経理")
	if err != nil {
		t.Fatalf("CreateContext duplicate failed: %v", err)
	}
	if dupID != id {
		t.Errorf("expected duplicate ID %d, got %d", id, dupID)
	}

	// 4. Update Context
	err = UpdateContext(db, id, "財務・経理")
	if err != nil {
		t.Fatalf("UpdateContext failed: %v", err)
	}
	c, err = GetContextByID(db, id)
	if err != nil {
		t.Fatalf("GetContextByID after update failed: %v", err)
	}
	if c.Name != "#財務・経理" {
		t.Errorf("expected '#財務・経理', got %q", c.Name)
	}

	// 5. Delete Context
	err = DeleteContext(db, id)
	if err != nil {
		t.Fatalf("DeleteContext failed: %v", err)
	}
	_, err = GetContextByID(db, id)
	if err == nil {
		t.Errorf("expected error getting deleted context, got nil")
	}
}

