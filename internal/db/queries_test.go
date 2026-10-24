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

func TestStartDateAndScheduledView(t *testing.T) {
	dbPath := setupTestDB(t)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	// 1. 未来の開始日を持つ Next Action アイテム（Tickler / 待機中）
	futureItem := &model.Item{
		Title:     "Future action",
		Status:    "next",
		StartDate: &tomorrow,
	}
	fID, err := CreateItem(db, futureItem)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 2. 今日開始の Next Action アイテム
	todayItem := &model.Item{
		Title:     "Today action",
		Status:    "next",
		StartDate: &today,
	}
	_, err = CreateItem(db, todayItem)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 3. 期限切れ（昨日）のタスク
	overdueItem := &model.Item{
		Title:   "Overdue task",
		Status:  "next",
		DueDate: &yesterday,
	}
	_, err = CreateItem(db, overdueItem)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// Next Actions リストの検証: 未来のタスクは除外され、Today action と Overdue task の2件のみ取得されること
	nextItems, err := GetItems(db, model.ItemFilter{View: "next"})
	if err != nil {
		t.Fatalf("GetItems next failed: %v", err)
	}
	if len(nextItems) != 2 {
		t.Errorf("expected 2 next actions (excluding deferred), got %d", len(nextItems))
	}
	for _, it := range nextItems {
		if it.ID == fID {
			t.Errorf("futureItem should not appear in next actions list")
		}
	}

	// サイドバーカウントの検証
	counts, err := GetSidebarCounts(db)
	if err != nil {
		t.Fatalf("GetSidebarCounts failed: %v", err)
	}
	if counts.Next != 2 {
		t.Errorf("expected counts.Next = 2, got %d", counts.Next)
	}
	if counts.ScheduledOverdueToday != 1 {
		t.Errorf("expected counts.ScheduledOverdueToday = 1, got %d", counts.ScheduledOverdueToday)
	}
	// ScheduledTotal: futureItem(未来開始) + overdueItem(期限付き) = 2
	if counts.ScheduledTotal != 2 {
		t.Errorf("expected counts.ScheduledTotal = 2, got %d", counts.ScheduledTotal)
	}

	// Scheduled ビューの取得検証
	schedItems, err := GetItems(db, model.ItemFilter{View: "scheduled"})
	if err != nil {
		t.Fatalf("GetItems scheduled failed: %v", err)
	}
	if len(schedItems) != 2 {
		t.Errorf("expected 2 scheduled items, got %d", len(schedItems))
	}

	// GetItemByID と UpdateItem での StartDate 保存・更新
	retrieved, err := GetItemByID(db, fID)
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}
	if retrieved.StartDate == nil || *retrieved.StartDate != tomorrow {
		t.Errorf("expected StartDate %s, got %v", tomorrow, retrieved.StartDate)
	}

	// 更新
	retrieved.StartDate = &today
	if err := UpdateItem(db, retrieved); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}
	updated, err := GetItemByID(db, fID)
	if err != nil {
		t.Fatalf("GetItemByID after update failed: %v", err)
	}
	if updated.StartDate == nil || *updated.StartDate != today {
		t.Errorf("expected updated StartDate %s, got %v", today, updated.StartDate)
	}
}

func TestSearchOmni(t *testing.T) {
	dbPath := setupTestDB(t)
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 1. プロジェクト作成
	pID, err := CreateProject(db, &model.Project{
		Name:        "社内システム刷新プロジェクト",
		Description: "基幹システムの全面リプレイス",
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 2. アイテム作成
	item1 := &model.Item{
		ProjectID: &pID,
		Title:     "要件定義書の初稿作成",
		Body:      "非機能要件とセキュリティ要件を詳細に記述する",
		Status:    "next",
	}
	id1, err := CreateItem(db, item1)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	item2 := &model.Item{
		Title:  "定例会議の議事録共有",
		Body:   "要件定義書レビューの日程調整についても触れる",
		Status: "done",
	}
	id2, err := CreateItem(db, item2)
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	// 3. 添付ファイル作成
	att1 := &model.Attachment{
		ItemID: id1,
		Label:  "新仕様書ドラフト.xlsx",
		Target: `C:\docs\spec_v1.xlsx`,
		Type:   "path",
	}
	if _, err := CreateAttachment(db, att1); err != nil {
		t.Fatalf("CreateAttachment failed: %v", err)
	}

	att2 := &model.Attachment{
		ItemID: id2,
		Label:  "会議録リンク",
		Target: "https://wiki.example.com/minutes/20261024",
		Type:   "url",
	}
	if _, err := CreateAttachment(db, att2); err != nil {
		t.Fatalf("CreateAttachment failed: %v", err)
	}

	// 4. タイトル検索テスト（"要件定義書"）
	res1, err := SearchOmni(db, "要件定義書", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni failed: %v", err)
	}
	if len(res1.Items) != 2 {
		t.Fatalf("expected 2 items for '要件定義書', got %d", len(res1.Items))
	}
	// 未完了(status=next)が先頭に来ていること
	if res1.Items[0].ID != id1 {
		t.Errorf("expected active item id1 first, got %d", res1.Items[0].ID)
	}

	// 5. 本文検索テスト（"セキュリティ要件"）
	res2, err := SearchOmni(db, "セキュリティ要件", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni failed: %v", err)
	}
	if len(res2.Items) != 1 || res2.Items[0].ID != id1 {
		t.Errorf("expected 1 item with id1 for body search, got %d", len(res2.Items))
	}

	// 6. プロジェクト名検索テスト（"社内システム"）
	res3, err := SearchOmni(db, "社内システム", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni failed: %v", err)
	}
	if len(res3.Items) != 1 || res3.Items[0].ID != id1 {
		t.Errorf("expected 1 item matching project name, got %d", len(res3.Items))
	}

	// 7. 添付ファイルラベル検索テスト（"新仕様書ドラフト"）
	res4, err := SearchOmni(db, "新仕様書ドラフト", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni failed: %v", err)
	}
	if len(res4.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res4.Attachments))
	}
	if res4.Attachments[0].ItemID != id1 {
		t.Errorf("expected attachment itemID %d, got %d", id1, res4.Attachments[0].ItemID)
	}
	if res4.Attachments[0].MatchField != "label" {
		t.Errorf("expected match_field 'label', got %s", res4.Attachments[0].MatchField)
	}

	// 8. 添付ファイルTarget検索テスト（"wiki.example.com"）
	res5, err := SearchOmni(db, "wiki.example.com", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni failed: %v", err)
	}
	if len(res5.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res5.Attachments))
	}
	if res5.Attachments[0].ItemID != id2 {
		t.Errorf("expected attachment itemID %d, got %d", id2, res5.Attachments[0].ItemID)
	}

	// 9. 空文字検索テスト
	resEmpty, err := SearchOmni(db, "   ", 10, 10)
	if err != nil {
		t.Fatalf("SearchOmni empty failed: %v", err)
	}
	if resEmpty.TotalCount != 0 {
		t.Errorf("expected 0 total count for empty query, got %d", resEmpty.TotalCount)
	}
}


