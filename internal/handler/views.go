package handler

import (
	"net/http"
	"strconv"

	"gtd/internal/db"
	"gtd/internal/model"
)

// IndexData はトップ画面描画用のデータモデルです
type IndexData struct {
	Sidebar  SidebarData
	ItemList ItemListData
}

// SidebarData はサイドバー描画用のデータモデルです
type SidebarData struct {
	ActiveView string
	Counts     model.SidebarCounts
	Projects   []model.Project
	Contexts   []model.Context
	SelectedID int64
}

// ItemListData は中央リスト描画用のデータモデルです
type ItemListData struct {
	View           string
	Title          string
	Description    string
	Items          []model.Item
	Contexts       []model.Context
	CurrentProject *model.Project
	CurrentContext *model.Context
	ProjectID      *int64
	ContextID      *int64
	Search         string
	Projects       []model.Project
	ProjectStatus  string
	Counts         model.SidebarCounts
}

func (h *Handler) buildSidebarData(view string, selectedProjectID int64) SidebarData {
	counts, _ := db.GetSidebarCounts(h.DB)
	projects, _ := db.GetProjects(h.DB, "active")
	contexts, _ := db.GetContexts(h.DB)

	return SidebarData{
		ActiveView: view,
		Counts:     counts,
		Projects:   projects,
		Contexts:   contexts,
		SelectedID: selectedProjectID,
	}
}

func (h *Handler) buildItemListData(view string, pID *int64, cID *int64, search string, projectStatus string) (ItemListData, error) {
	if view == "" {
		view = "inbox"
	}
	if pID != nil {
		view = "project"
	}

	counts, _ := db.GetSidebarCounts(h.DB)
	contexts, _ := db.GetContexts(h.DB)

	var items []model.Item
	var err error
	var projects []model.Project

	if view == "projects" {
		if projectStatus == "" {
			projectStatus = "all"
		}
		statusQuery := projectStatus
		if projectStatus == "all" {
			statusQuery = ""
		}
		projects, err = db.GetProjects(h.DB, statusQuery)
		if err != nil {
			return ItemListData{}, err
		}
	} else {
		items, err = db.GetItems(h.DB, model.ItemFilter{
			View:      view,
			ProjectID: pID,
			ContextID: cID,
			Search:    search,
		})
		if err != nil {
			return ItemListData{}, err
		}
	}

	var currentProject *model.Project
	if pID != nil {
		currentProject, _ = db.GetProjectByID(h.DB, *pID)
	}

	var currentContext *model.Context
	if cID != nil {
		for _, c := range contexts {
			if c.ID == *cID {
				currentContext = &c
				break
			}
		}
	}

	// 表示タイトル
	title := "Inbox"
	description := "頭の中にあるものをすべて書き出す場所。判断は後から行います。"
	switch view {
	case "next":
		title = "Next Actions"
		description = "今すぐ、または次に実行可能な具体的行動。"
	case "waiting":
		title = "Waiting For"
		description = "誰かの返信や成果物待ちのタスク。"
	case "someday":
		title = "Someday / Maybe"
		description = "いつかやりたいこと、将来のアイデア、保留項目。"
	case "reference":
		title = "References"
		description = "行動を伴わない資料・メモ・保管情報。"
	case "done":
		title = "Done"
		description = "完了したタスクの記録・業務履歴。"
	case "project":
		if currentProject != nil {
			title = currentProject.Name
			description = currentProject.Description
		} else {
			title = "Projects"
			description = "複数の行動を要するゴール。"
		}
	case "projects":
		title = "プロジェクト管理・履歴"
		description = "進行中・保留中・完了済みの全プロジェクトの確認とステータス変更。"
	}

	if currentContext != nil {
		title = title + " (" + currentContext.Name + ")"
	}

	return ItemListData{
		View:           view,
		Title:          title,
		Description:    description,
		Items:          items,
		Contexts:       contexts,
		CurrentProject: currentProject,
		CurrentContext: currentContext,
		ProjectID:      pID,
		ContextID:      cID,
		Search:         search,
		Projects:       projects,
		ProjectStatus:  projectStatus,
		Counts:         counts,
	}, nil
}

// ServeIndex は初回アクセス時の3ペイン全体レイアウトを描画します
func (h *Handler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "inbox"
	}

	var pID *int64
	var selectedID int64
	if pidStr := r.URL.Query().Get("project_id"); pidStr != "" {
		if id, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
			pID = &id
			selectedID = id
		}
	}

	var cID *int64
	if cidStr := r.URL.Query().Get("context_id"); cidStr != "" {
		if id, err := strconv.ParseInt(cidStr, 10, 64); err == nil {
			cID = &id
		}
	}

	search := r.URL.Query().Get("search")
	projectStatus := r.URL.Query().Get("project_status")

	sidebarData := h.buildSidebarData(view, selectedID)
	itemListData, err := h.buildItemListData(view, pID, cID, search, projectStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := IndexData{
		Sidebar:  sidebarData,
		ItemList: itemListData,
	}

	h.Render(w, "layout.html", data)
}

// ServeSidebar はサイドバー（左ペイン）パーシャルを描画します
func (h *Handler) ServeSidebar(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "inbox"
	}

	var selectedID int64
	if pidStr := r.URL.Query().Get("project_id"); pidStr != "" {
		selectedID, _ = strconv.ParseInt(pidStr, 10, 64)
	}

	data := h.buildSidebarData(view, selectedID)
	h.Render(w, "sidebar.html", data)
}

// ServeItemList は中央ペインのアイテム一覧パーシャルを描画します
func (h *Handler) ServeItemList(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "inbox"
	}

	var pID *int64
	if pidStr := r.URL.Query().Get("project_id"); pidStr != "" {
		if id, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
			pID = &id
		}
	}

	var cID *int64
	if cidStr := r.URL.Query().Get("context_id"); cidStr != "" {
		if id, err := strconv.ParseInt(cidStr, 10, 64); err == nil {
			cID = &id
		}
	}

	search := r.URL.Query().Get("search")
	projectStatus := r.URL.Query().Get("project_status")

	data, err := h.buildItemListData(view, pID, cID, search, projectStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Render(w, "item_list.html", data)
}
