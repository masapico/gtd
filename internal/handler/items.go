package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gtd/internal/db"
	"gtd/internal/model"
	"gtd/internal/smartadd"
)

// ItemDetailData は右ペイン（インスペクタ）描画用のデータモデルです
type ItemDetailData struct {
	Item        *model.Item
	AllProjects []model.Project
	AllContexts []model.Context
}

// CreateItem は新規アイテムを作成します（Quick Capture、Projectビュー、レビュー画面）
func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rawTitle := strings.TrimSpace(r.FormValue("title"))
	if rawTitle == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	// Smart Add 構文解析 (^期日, #コンテキスト)
	parsed := smartadd.Parse(rawTitle, time.Now())
	title := parsed.CleanTitle
	if title == "" {
		title = rawTitle
	}

	status := r.FormValue("status")
	if status == "" {
		status = "inbox"
	}

	var pID *int64
	if pidStr := r.FormValue("project_id"); pidStr != "" {
		if id, err := strconv.ParseInt(pidStr, 10, 64); err == nil && id > 0 {
			pID = &id
		}
	}

	dueDate := r.FormValue("due_date")
	var pDueDate *string
	if dueDate != "" {
		pDueDate = &dueDate
	} else if parsed.DueDate != nil {
		pDueDate = parsed.DueDate
	}

	startDate := r.FormValue("start_date")
	var pStartDate *string
	if startDate != "" {
		pStartDate = &startDate
	} else if parsed.StartDate != nil {
		pStartDate = parsed.StartDate
	}

	recurrence := r.FormValue("recurrence")
	if recurrence == "" {
		recurrence = "none"
	}

	// 解析されたContextタグを取得または自動作成
	var itemContexts []model.Context
	for _, ctxName := range parsed.Contexts {
		c, err := db.FindOrCreateContext(h.DB, ctxName)
		if err == nil && c != nil {
			itemContexts = append(itemContexts, *c)
		}
	}

	item := &model.Item{
		ProjectID:  pID,
		Title:      title,
		Body:       r.FormValue("body"),
		Status:     status,
		StartDate:  pStartDate,
		DueDate:    pDueDate,
		Recurrence: recurrence,
		Contexts:   itemContexts,
	}

	_, err := db.CreateItem(h.DB, item)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 画面全体の各ペインへ再読み込みイベントを発火
	h.Trigger(w, "itemChanged", "contextChanged")
	w.WriteHeader(http.StatusOK)
}

// GetItemDetail は右ペイン（インスペクタ）にアイテムの詳細情報を描画します
func (h *Handler) GetItemDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	item, err := db.GetItemByID(h.DB, id)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	allProjects, _ := db.GetProjects(h.DB, "")
	allContexts, _ := db.GetContexts(h.DB)

	data := ItemDetailData{
		Item:        item,
		AllProjects: allProjects,
		AllContexts: allContexts,
	}

	h.Render(w, "item_detail.html", data)
}

// UpdateItem は右ペインからの自動保存（タイピング停止後・変更後）を処理します
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = "（タイトルなし）"
	}

	var pDueDate *string
	if d := r.FormValue("due_date"); d != "" {
		pDueDate = &d
	}

	var pStartDate *string
	if s := r.FormValue("start_date"); s != "" {
		pStartDate = &s
	}

	// Smart Add 構文解析 (^期日, ~開始日, #コンテキスト)
	parsed := smartadd.Parse(title, time.Now())
	syntaxDetected := false
	if parsed.CleanTitle != title || len(parsed.Contexts) > 0 || (parsed.DueDate != nil && (pDueDate == nil || *pDueDate != *parsed.DueDate)) || (parsed.StartDate != nil && (pStartDate == nil || *pStartDate != *parsed.StartDate)) {
		syntaxDetected = true
		title = parsed.CleanTitle
		if title == "" {
			title = "（タイトルなし）"
		}
		if parsed.StartDate != nil {
			pStartDate = parsed.StartDate
		}
		if parsed.DueDate != nil {
			pDueDate = parsed.DueDate
		}
		for _, ctxName := range parsed.Contexts {
			c, err := db.FindOrCreateContext(h.DB, ctxName)
			if err == nil && c != nil {
				_, _ = h.DB.Exec("INSERT OR IGNORE INTO item_contexts (item_id, context_id) VALUES (?, ?)", id, c.ID)
			}
		}
	}

	var pID *int64
	if pidStr := r.FormValue("project_id"); pidStr != "" {
		if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil && pid > 0 {
			pID = &pid
		}
	}

	recurrence := r.FormValue("recurrence")
	if recurrence == "" {
		recurrence = "none"
	}

	status := r.FormValue("status")
	if status == "" {
		status = "inbox"
	}

	item := &model.Item{
		ID:         id,
		ProjectID:  pID,
		Title:      title,
		Body:       r.FormValue("body"),
		Status:     status,
		StartDate:  pStartDate,
		DueDate:    pDueDate,
		Recurrence: recurrence,
	}

	if err := db.UpdateItem(h.DB, item); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 変更を中央リストやサイドバーに同期
	if syntaxDetected {
		h.Trigger(w, "itemChanged", "contextChanged", "itemDetailRefresh")
	} else {
		h.Trigger(w, "itemChanged")
	}
	w.WriteHeader(http.StatusOK)
}

// UpdateItemStatus はステータスを即時変更します（完了チェックボックスやステータスボタン）
func (h *Handler) UpdateItemStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newStatus := r.FormValue("status")
	if newStatus == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	_, err = db.UpdateItemStatus(h.DB, id, newStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	// 詳細ペインを最新データで再描画
	h.GetItemDetail(w, r)
}

// ToggleItemContext はアイテムへのContextタグをトグルします
func (h *Handler) ToggleItemContext(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	ctxIDStr := r.PathValue("contextId")
	ctxID, err := strconv.ParseInt(ctxIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid context ID", http.StatusBadRequest)
		return
	}

	if err := db.ToggleItemContext(h.DB, itemID, ctxID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	// 詳細ペインを再描画
	h.GetItemDetail(w, r)
}

// DeleteItem はアイテムを削除します
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// 紐づく添付ファイル（画像実体）を削除前にクリーンアップ
	if attachments, err := db.GetAttachmentsByItemID(h.DB, id); err == nil {
		for _, att := range attachments {
			if att.Type == "image" && att.Target != "" {
				filePath := filepath.Join(h.ImagesDir, filepath.Base(att.Target))
				_ = os.Remove(filePath)
			}
		}
	}

	if err := db.DeleteItem(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// CreateContext は新しいContextタグを追加します
func (h *Handler) CreateContext(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	_, err := db.CreateContext(h.DB, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "contextChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// UpdateContext はContextタグの名前を変更します
func (h *Handler) UpdateContext(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid context ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if err := db.UpdateContext(h.DB, id, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "contextChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// DeleteContext はContextタグを削除します
func (h *Handler) DeleteContext(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid context ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteContext(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "contextChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

