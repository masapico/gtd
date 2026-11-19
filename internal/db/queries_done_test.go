package db_test

import (
	"path/filepath"
	"testing"

	"gtd/internal/db"
	"gtd/internal/model"
)

func TestCompletionTracksTimestampAndPrevStatus(t *testing.T) {
	database := openReorderTestDB(t)

	id, err := db.CreateItem(database, &model.Item{Title: "返信待ち", Status: "waiting"})
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	if _, err := db.UpdateItemStatus(database, id, "done"); err != nil {
		t.Fatalf("UpdateItemStatus(done) failed: %v", err)
	}
	it, err := db.GetItemByID(database, id)
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}
	if it.CompletedAt == nil {
		t.Fatalf("completed_at should be set after done")
	}
	if it.PrevStatus != "waiting" {
		t.Fatalf("prev_status = %q, want waiting", it.PrevStatus)
	}

	// 完了済みのまま詳細を保存しても完了日時・完了前ステータスは変わらない
	it.Body = "メモ追記"
	if err := db.UpdateItem(database, it); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}
	after, _ := db.GetItemByID(database, id)
	if after.CompletedAt == nil || !after.CompletedAt.Equal(*it.CompletedAt) || after.PrevStatus != "waiting" {
		t.Fatalf("completion info changed by UpdateItem: %+v", after)
	}

	// 完了を取り消すと完了前のステータスへ戻り、完了日時は消える
	restored, err := db.UpdateItemStatus(database, id, db.StatusUndone)
	if err != nil {
		t.Fatalf("UpdateItemStatus(undone) failed: %v", err)
	}
	if restored.Status != "waiting" {
		t.Fatalf("restored status = %q, want waiting", restored.Status)
	}
	it, _ = db.GetItemByID(database, id)
	if it.Status != "waiting" || it.CompletedAt != nil {
		t.Fatalf("after undone: status=%q completed_at=%v", it.Status, it.CompletedAt)
	}
}

func TestUndoneWithoutPrevStatusFallsBackToNext(t *testing.T) {
	database := openReorderTestDB(t)

	// 最初から done で登録されたアイテム（完了前ステータスの記録がない）
	id, err := db.CreateItem(database, &model.Item{Title: "過去の完了", Status: "done"})
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	restored, err := db.UpdateItemStatus(database, id, db.StatusUndone)
	if err != nil {
		t.Fatalf("UpdateItemStatus(undone) failed: %v", err)
	}
	if restored.Status != "next" {
		t.Fatalf("status = %q, want next", restored.Status)
	}
}

func TestDoneViewOrderAndLimit(t *testing.T) {
	database := openReorderTestDB(t)

	var ids []int64
	for _, title := range []string{"A", "B", "C"} {
		id, err := db.CreateItem(database, &model.Item{Title: title, Status: "next"})
		if err != nil {
			t.Fatalf("CreateItem failed: %v", err)
		}
		if _, err := db.UpdateItemStatus(database, id, "done"); err != nil {
			t.Fatalf("UpdateItemStatus failed: %v", err)
		}
		ids = append(ids, id)
	}
	// 完了日時: A が最新、C が最古
	stamps := []string{"2026-11-19 10:00:00", "2026-11-18 10:00:00", "2026-11-01 10:00:00"}
	for i, id := range ids {
		if _, err := database.Exec("UPDATE items SET completed_at = ? WHERE id = ?", stamps[i], id); err != nil {
			t.Fatalf("set completed_at failed: %v", err)
		}
	}
	// 最古の C を編集して updated_at を新しくしても、並びは完了日時のまま
	c, _ := db.GetItemByID(database, ids[2])
	c.Body = "後から編集"
	if err := db.UpdateItem(database, c); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}

	items, err := db.GetItems(database, model.ItemFilter{View: "done"})
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(items) != 3 || items[0].ID != ids[0] || items[1].ID != ids[1] || items[2].ID != ids[2] {
		t.Fatalf("unexpected done order: %+v", items)
	}

	limited, err := db.GetItems(database, model.ItemFilter{View: "done", Limit: 2})
	if err != nil {
		t.Fatalf("GetItems(limit) failed: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 items with Limit, got %d", len(limited))
	}
}

func TestMigrationBackfillsCompletedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	// 列追加前に完了したアイテムを模す
	if _, err := database.Exec(`INSERT INTO items (title, status, updated_at) VALUES ('旧データ', 'done', '2026-01-02 03:04:05')`); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	database.Close()

	database, err = db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer database.Close()

	var completedAt string
	if err := database.QueryRow(`SELECT completed_at FROM items WHERE title = '旧データ'`).Scan(&completedAt); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if completedAt == "" {
		t.Fatalf("completed_at should be backfilled from updated_at")
	}
}
