package db_test

import (
	"path/filepath"
	"testing"

	"gtd/internal/db"
	"gtd/internal/model"
)

func TestProjectSequentialAndOrdering(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_gtd.db")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	// 1. 冪等性テスト: もう一度 InitDB を呼んでもエラーにならないこと
	if _, err := db.InitDB(dbPath); err != nil {
		t.Fatalf("Second InitDB failed: %v", err)
	}

	// 2. 順次プロジェクトの作成
	proj := &model.Project{
		Name:   "順次テストプロジェクト",
		Type:   "sequential",
		Status: "active",
	}
	pID, err := db.CreateProject(database, proj)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// 3. 3つのタスクを追加
	t1 := &model.Item{ProjectID: &pID, Title: "Step 1", Status: "next"}
	id1, _ := db.CreateItem(database, t1)

	t2 := &model.Item{ProjectID: &pID, Title: "Step 2", Status: "next"}
	id2, _ := db.CreateItem(database, t2)

	t3 := &model.Item{ProjectID: &pID, Title: "Step 3", Status: "next"}
	id3, _ := db.CreateItem(database, t3)

	// 4. Next Actions ビューでの取得確認: Step 1 のみが is_available で、Step 2, 3 は除外されること
	nextItems, err := db.GetItems(database, model.ItemFilter{View: "next"})
	if err != nil {
		t.Fatalf("GetItems(next) failed: %v", err)
	}
	if len(nextItems) != 1 || nextItems[0].ID != id1 {
		t.Fatalf("Expected only Step 1 in next view, got: %+v", nextItems)
	}

	// 5. プロジェクトビューでの取得確認: Step 1 は blocked=false, Step 2, 3 は blocked=true
	pItems, err := db.GetItems(database, model.ItemFilter{View: "project", ProjectID: &pID})
	if err != nil {
		t.Fatalf("GetItems(project) failed: %v", err)
	}
	if len(pItems) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(pItems))
	}
	if pItems[0].IsBlocked || !pItems[1].IsBlocked || !pItems[2].IsBlocked {
		t.Fatalf("Unexpected blocked flags: [0]=%v, [1]=%v, [2]=%v", pItems[0].IsBlocked, pItems[1].IsBlocked, pItems[2].IsBlocked)
	}

	// 6. Step 1 を done に変更 -> Step 2 が自動で利用可能になり、Step 3 は依然 blocked
	if _, err := db.UpdateItemStatus(database, id1, "done"); err != nil {
		t.Fatalf("UpdateItemStatus failed: %v", err)
	}

	nextItems, _ = db.GetItems(database, model.ItemFilter{View: "next"})
	if len(nextItems) != 1 || nextItems[0].ID != id2 {
		t.Fatalf("Expected Step 2 in next view after Step 1 done, got: %+v", nextItems)
	}

	pItems, _ = db.GetItems(database, model.ItemFilter{View: "project", ProjectID: &pID})
	for _, it := range pItems {
		if it.ID == id2 && it.IsBlocked {
			t.Errorf("Step 2 should not be blocked")
		}
		if it.ID == id3 && !it.IsBlocked {
			t.Errorf("Step 3 should still be blocked")
		}
	}

	// 7. 並び替えテスト: MoveItem (Step 3 を up 移動 -> Step 3 が Step 2 の前に来る)
	if err := db.MoveItem(database, id3, "up"); err != nil {
		t.Fatalf("MoveItem(up) failed: %v", err)
	}

	// Step 3 が先頭になったので、Step 3 が実行可能になり、Step 2 が blocked になるはず
	nextItems, _ = db.GetItems(database, model.ItemFilter{View: "next"})
	if len(nextItems) != 1 || nextItems[0].ID != id3 {
		t.Fatalf("Expected Step 3 in next view after move up, got: %+v", nextItems)
	}

	// 8. 並列プロジェクトへの変更テスト
	if err := db.UpdateProjectType(database, pID, "parallel"); err != nil {
		t.Fatalf("UpdateProjectType failed: %v", err)
	}

	// 並列になると、未完了の Step 2, Step 3 両方が Next Actions ビューに現れる
	nextItems, _ = db.GetItems(database, model.ItemFilter{View: "next"})
	if len(nextItems) != 2 {
		t.Fatalf("Expected 2 items in next view for parallel project, got %d", len(nextItems))
	}

	// 9. 停滞判定テスト: 先頭が Waiting の場合、プロジェクトは Stalled にならず WaitingOnly になる
	if err := db.UpdateProjectType(database, pID, "sequential"); err != nil {
		t.Fatalf("UpdateProjectType failed: %v", err)
	}
	// Step 3 (先頭) を waiting にする
	if _, err := db.UpdateItemStatus(database, id3, "waiting"); err != nil {
		t.Fatalf("UpdateItemStatus failed: %v", err)
	}

	projUpdated, err := db.GetProjectByID(database, pID)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if projUpdated.NextActionCount != 0 {
		t.Errorf("Expected 0 next actions, got %d", projUpdated.NextActionCount)
	}
	if projUpdated.WaitingCount != 1 {
		t.Errorf("Expected 1 waiting, got %d", projUpdated.WaitingCount)
	}
	if projUpdated.IsStalled() {
		t.Errorf("Project with active Waiting should not be marked as Stalled")
	}
	if !projUpdated.IsWaitingOnly() {
		t.Errorf("Project should be IsWaitingOnly")
	}
}

