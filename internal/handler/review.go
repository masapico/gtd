package handler

import (
	"net/http"
	"strconv"

	"gtd/internal/db"
	"gtd/internal/model"
)

// ReviewData は週次レビュー画面用のデータモデルです
type ReviewData struct {
	Step           int
	InboxItems     []model.Item
	ActiveProjects []model.Project
	StaleItems     []model.Item
	Counts         model.SidebarCounts
}

// StartReview は週次レビューを開始し、Step 1（Get Clear）を表示します
func (h *Handler) StartReview(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/review/step/1", http.StatusSeeOther)
}

// ReviewStep は指定されたステップのレビュー画面を描画します
func (h *Handler) ReviewStep(w http.ResponseWriter, r *http.Request) {
	stepStr := r.PathValue("step")
	step, err := strconv.Atoi(stepStr)
	if err != nil || step < 1 || step > 3 {
		step = 1
	}

	counts, _ := db.GetSidebarCounts(h.DB)
	data := ReviewData{
		Step:   step,
		Counts: counts,
	}

	switch step {
	case 1:
		inboxItems, _ := db.GetInboxItems(h.DB)
		data.InboxItems = inboxItems
	case 2:
		projects, _ := db.GetActiveProjectsForReview(h.DB)
		data.ActiveProjects = projects
	case 3:
		staleItems, _ := db.GetStaleItemsForReview(h.DB)
		data.StaleItems = staleItems
	}

	h.Render(w, "review_step.html", data)
}

// MarkProjectReviewed はプロジェクトの点検完了を記録します
func (h *Handler) MarkProjectReviewed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := db.MarkProjectReviewed(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// MarkItemReviewed は待機・保留アイテムの点検完了を記録します
func (h *Handler) MarkItemReviewed(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if err := db.MarkItemReviewed(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	w.WriteHeader(http.StatusOK)
}

