package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gtd/internal/db"
	"gtd/internal/model"
)

func getBody(t *testing.T, mux http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", path, rec.Code, rec.Body.String())
	}
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestGroupDoneItems(t *testing.T) {
	loc := time.Local
	at := func(s string) *time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return &tm
	}
	ids := func(secs []model.DoneSection) string {
		var parts []string
		for _, s := range secs {
			parts = append(parts, fmt.Sprintf("%s:%d", s.ID, len(s.Items)))
		}
		return strings.Join(parts, ",")
	}

	// 2026-11-19 は木曜。今週は 11/16(月)〜、先週は 11/9(月)〜
	now := *at("2026-11-19 12:00")
	items := []model.Item{
		{ID: 1, CompletedAt: at("2026-11-19 09:00")},
		{ID: 2, CompletedAt: at("2026-11-18 23:59")},
		{ID: 3, CompletedAt: at("2026-11-16 00:00")},
		{ID: 4, CompletedAt: at("2026-11-15 23:59")},
		{ID: 5, CompletedAt: at("2026-11-09 00:00")},
		{ID: 6, CompletedAt: at("2026-11-08 10:00")},
		{ID: 7, CompletedAt: at("2026-10-31 10:00")},
		{ID: 8, CompletedAt: at("2026-10-01 10:00")},
		{ID: 9, UpdatedAt: *at("2025-12-31 10:00")}, // completed_at の無い古いデータは更新日時で代用
	}
	got := ids(groupDoneItems(items, now))
	want := "today:1,yesterday:1,this_week:1,last_week:2,month-2026-11:1,month-2026-10:2,month-2025-12:1"
	if got != want {
		t.Fatalf("groups = %s\nwant     %s", got, want)
	}

	// 月曜日: 昨日（日曜）は「昨日」、土曜は「先週」
	monday := *at("2026-11-23 08:00")
	got = ids(groupDoneItems([]model.Item{
		{ID: 1, CompletedAt: at("2026-11-22 20:00")},
		{ID: 2, CompletedAt: at("2026-11-21 20:00")},
	}, monday))
	if got != "yesterday:1,last_week:1" {
		t.Fatalf("monday groups = %s", got)
	}

	sections := groupDoneItems([]model.Item{{ID: 1, CompletedAt: at("2026-10-01 10:00")}}, now)
	if sections[0].Title != "2026年10月" {
		t.Fatalf("month title = %q", sections[0].Title)
	}
}

func TestUndoneRestoresPreviousStatus(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	id, err := db.CreateItem(h.DB, &model.Item{Title: "返事待ち", Status: "waiting"})
	if err != nil {
		t.Fatal(err)
	}
	patch := func(status string) *httptest.ResponseRecorder {
		form := url.Values{"status": {status}}
		req := httptest.NewRequest("PATCH", fmt.Sprintf("/items/%d/status", id), strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH %s: expected 200, got %d: %s", status, rec.Code, rec.Body.String())
		}
		return rec
	}

	patch("done")
	rec := patch("undone")

	it, _ := db.GetItemByID(h.DB, id)
	if it.Status != "waiting" {
		t.Fatalf("status after undone = %q, want waiting", it.Status)
	}
	// 「元に戻す」Snackbar には実際に設定したステータスを通知する
	if trig := rec.Header().Get("HX-Trigger"); !strings.Contains(trig, `"status":"waiting"`) || !strings.Contains(trig, `"prev":"done"`) {
		t.Fatalf("unexpected HX-Trigger: %s", trig)
	}

	// Done ビューのカードは「undone」を送る
	patch("done")
	if html := getBody(t, mux, "/partials/items?view=done"); !strings.Contains(html, `{"status": "undone"}`) {
		t.Fatalf("done card should send status=undone")
	}
}

func TestDoneViewSectionsAndMore(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	for i := 0; i < doneDefaultLimit+1; i++ {
		id, err := db.CreateItem(h.DB, &model.Item{Title: fmt.Sprintf("完了 %d", i), Status: "next"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpdateItemStatus(h.DB, id, "done"); err != nil {
			t.Fatal(err)
		}
	}

	html := getBody(t, mux, "/partials/items?view=done")
	if !strings.Contains(html, `id="done-section-today"`) {
		t.Fatalf("expected today section in done view")
	}
	if got := strings.Count(html, `class="item-card `); got != doneDefaultLimit {
		t.Fatalf("expected %d cards, got %d", doneDefaultLimit, got)
	}
	if !strings.Contains(html, fmt.Sprintf("view=done&limit=%d", 2*doneDefaultLimit)) {
		t.Fatalf("expected 'さらに表示' link with limit=%d", 2*doneDefaultLimit)
	}

	html = getBody(t, mux, fmt.Sprintf("/partials/items?view=done&limit=%d", 2*doneDefaultLimit))
	if got := strings.Count(html, `class="item-card `); got != doneDefaultLimit+1 {
		t.Fatalf("expected %d cards with larger limit, got %d", doneDefaultLimit+1, got)
	}
	if strings.Contains(html, "done-more-row") {
		t.Fatalf("no 'さらに表示' expected when everything is shown")
	}
	// 再取得 URL に上限を引き継ぐ
	if !strings.Contains(html, fmt.Sprintf("&limit=%d\"", 2*doneDefaultLimit)) {
		t.Fatalf("refresh URL should keep limit")
	}
}

func TestInboxViewShowsTriageButtons(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	if _, err := db.CreateItem(h.DB, &model.Item{Title: "未整理メモ", Status: "inbox"}); err != nil {
		t.Fatal(err)
	}
	html := getBody(t, mux, "/partials/items?view=inbox")
	if !strings.Contains(html, "inbox-triage-btns") || !strings.Contains(html, "Nextへ") {
		t.Fatalf("inbox view should show triage buttons")
	}
}

func TestEmptyStatesPerView(t *testing.T) {
	h, mux, cleanup := setupTestServer(t)
	defer cleanup()

	if html := getBody(t, mux, "/partials/items?view=inbox"); !strings.Contains(html, "Inbox Zero") {
		t.Fatalf("empty inbox should say Inbox Zero")
	}
	if html := getBody(t, mux, "/partials/items?view=next"); !strings.Contains(html, "プロジェクトを見る") {
		t.Fatalf("empty next (no inbox) should link to projects")
	}

	if _, err := db.CreateItem(h.DB, &model.Item{Title: "未整理", Status: "inbox"}); err != nil {
		t.Fatal(err)
	}
	html := getBody(t, mux, "/partials/items?view=next")
	if !strings.Contains(html, "Inbox を処理する（1件）") {
		t.Fatalf("empty next with inbox items should link to inbox")
	}
	if html := getBody(t, mux, "/partials/items?view=done"); !strings.Contains(html, "まだ完了したタスクはありません") {
		t.Fatalf("empty done message missing")
	}
	if html := getBody(t, mux, "/partials/items?view=inbox&search=zzz"); !strings.Contains(html, "「zzz」に一致するアイテムはありません") {
		t.Fatalf("search empty message missing")
	}
}
