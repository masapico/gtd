package db_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"gtd/internal/db"
	"gtd/internal/model"
)

func openReorderTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.InitDB(filepath.Join(t.TempDir(), "reorder.db"))
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createReorderProject(t *testing.T, database *sql.DB, projectType string) int64 {
	t.Helper()
	pID, err := db.CreateProject(database, &model.Project{Name: "P-" + projectType, Type: projectType, Status: "active"})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}
	return pID
}

func createReorderItem(t *testing.T, database *sql.DB, pID int64, title, status string) int64 {
	t.Helper()
	id, err := db.CreateItem(database, &model.Item{ProjectID: &pID, Title: title, Status: status})
	if err != nil {
		t.Fatalf("CreateItem(%s) failed: %v", title, err)
	}
	return id
}

// projectOrder はプロジェクト内の全アイテム ID を並び順で返します
func projectOrder(t *testing.T, database *sql.DB, pID int64) []int64 {
	t.Helper()
	rows, err := database.Query("SELECT id FROM items WHERE project_id = ? ORDER BY sort_order ASC, id ASC", pID)
	if err != nil {
		t.Fatalf("query order failed: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func assertOrder(t *testing.T, database *sql.DB, pID int64, want []int64) {
	t.Helper()
	if got := projectOrder(t, database, pID); !reflect.DeepEqual(got, want) {
		t.Fatalf("order mismatch: got %v, want %v", got, want)
	}
}

func TestMoveItemSkipsOtherSections(t *testing.T) {
	database := openReorderTestDB(t)
	pID := createReorderProject(t, database, "parallel")

	a := createReorderItem(t, database, pID, "A", "next")
	w := createReorderItem(t, database, pID, "W", "waiting")
	d := createReorderItem(t, database, pID, "D", "done")
	b := createReorderItem(t, database, pID, "B", "next")

	// B を上へ: 間の W(waiting) と D(done) を飛ばして A と入れ替わる
	if err := db.MoveItem(database, b, "up"); err != nil {
		t.Fatalf("MoveItem(up) failed: %v", err)
	}
	assertOrder(t, database, pID, []int64{b, w, d, a})

	// セクションの先頭でさらに上へ: 何も変わらない
	if err := db.MoveItem(database, b, "up"); err != nil {
		t.Fatalf("MoveItem(up at top) failed: %v", err)
	}
	assertOrder(t, database, pID, []int64{b, w, d, a})

	// W は waiting セクションに 1 件だけなので、どちらへも動かない
	if err := db.MoveItem(database, w, "down"); err != nil {
		t.Fatalf("MoveItem(waiting) failed: %v", err)
	}
	assertOrder(t, database, pID, []int64{b, w, d, a})

	// 完了済みアイテムは移動しない（エラーにもしない）
	if err := db.MoveItem(database, d, "up"); err != nil {
		t.Fatalf("MoveItem(done) failed: %v", err)
	}
	assertOrder(t, database, pID, []int64{b, w, d, a})

	if err := db.MoveItem(database, a, "sideways"); err == nil {
		t.Fatalf("expected error for invalid direction")
	}
}

func TestMoveItemSequentialStepsSpanNextAndWaiting(t *testing.T) {
	database := openReorderTestDB(t)
	pID := createReorderProject(t, database, "sequential")

	s1 := createReorderItem(t, database, pID, "S1", "next")
	m := createReorderItem(t, database, pID, "M", "someday")
	s2 := createReorderItem(t, database, pID, "S2", "waiting")

	// 順次プロジェクトでは Next と Waiting は同じ「ステップ」列。someday を飛ばして入れ替わる
	if err := db.MoveItem(database, s2, "up"); err != nil {
		t.Fatalf("MoveItem failed: %v", err)
	}
	assertOrder(t, database, pID, []int64{s2, m, s1})
}

func TestReorderItemsPreservesOtherSections(t *testing.T) {
	database := openReorderTestDB(t)
	pID := createReorderProject(t, database, "parallel")

	a := createReorderItem(t, database, pID, "A", "next")
	w1 := createReorderItem(t, database, pID, "W1", "waiting")
	b := createReorderItem(t, database, pID, "B", "next")
	d := createReorderItem(t, database, pID, "D", "done")
	c := createReorderItem(t, database, pID, "C", "next")

	if err := db.ReorderItems(database, pID, []int64{c, a, b}); err != nil {
		t.Fatalf("ReorderItems failed: %v", err)
	}
	// next のスロット (0, 2, 4) だけが C, A, B で埋め直され、W1 と D の位置は変わらない
	assertOrder(t, database, pID, []int64{c, w1, a, d, b})
}

func TestReorderItemsValidation(t *testing.T) {
	database := openReorderTestDB(t)
	pID := createReorderProject(t, database, "parallel")
	otherID := createReorderProject(t, database, "parallel")

	a := createReorderItem(t, database, pID, "A", "next")
	b := createReorderItem(t, database, pID, "B", "next")
	w := createReorderItem(t, database, pID, "W", "waiting")
	d1 := createReorderItem(t, database, pID, "D1", "done")
	d2 := createReorderItem(t, database, pID, "D2", "done")
	x := createReorderItem(t, database, otherID, "X", "next")

	cases := map[string][]int64{
		"empty":         {},
		"other project": {a, x},
		"mixed section": {a, w},
		"duplicate":     {a, b, a},
		"done section":  {d2, d1},
	}
	for name, ids := range cases {
		err := db.ReorderItems(database, pID, ids)
		if !errors.Is(err, db.ErrInvalidReorder) {
			t.Errorf("%s: expected ErrInvalidReorder, got %v", name, err)
		}
	}
	assertOrder(t, database, pID, []int64{a, b, w, d1, d2})

	if err := db.ReorderItems(database, 99999, []int64{a}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown project: expected sql.ErrNoRows, got %v", err)
	}
}
