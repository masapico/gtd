package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

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
	View              string
	Title             string
	Description       string
	Items             []model.Item
	ScheduledSections []model.ScheduledSection
	ProjectSections   []model.ProjectSection
	ProjectStats      model.ProjectStats
	ItemStatusFilter  string
	Contexts          []model.Context
	CurrentProject    *model.Project
	CurrentContext    *model.Context
	ProjectID         *int64
	ContextID         *int64
	Search            string
	Projects          []model.Project
	ProjectStatus     string
	Counts            model.SidebarCounts
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

func (h *Handler) buildItemListData(view string, pID *int64, cID *int64, search string, projectStatus string, itemStatusFilter string) (ItemListData, error) {
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
	var scheduledSections []model.ScheduledSection
	var projectSections []model.ProjectSection
	var projectStats model.ProjectStats

	switch view {
	case "next":
		title = "Next Actions"
		description = "今すぐ、または次に実行可能な具体的行動。"
	case "scheduled":
		title = "期限・予定"
		description = "期日があるタスクおよび指定日待ちの行動を時系列で把握します。"

		now := time.Now().In(time.Local)
		todayStr := now.Format("2006-01-02")
		tomorrowStr := now.AddDate(0, 0, 1).Format("2006-01-02")
		weekLaterStr := now.AddDate(0, 0, 7).Format("2006-01-02")

		var overdueItems, todayItems, tomorrowItems, weekItems, laterItems, deferredItems []model.Item

		for _, it := range items {
			due := it.FormattedDueDate()
			if due != "" {
				if due < todayStr {
					overdueItems = append(overdueItems, it)
				} else if due == todayStr {
					todayItems = append(todayItems, it)
				} else if due == tomorrowStr {
					tomorrowItems = append(tomorrowItems, it)
				} else if due <= weekLaterStr {
					weekItems = append(weekItems, it)
				} else {
					laterItems = append(laterItems, it)
				}
			} else if it.IsDeferred() {
				deferredItems = append(deferredItems, it)
			}
		}

		if len(overdueItems) > 0 {
			scheduledSections = append(scheduledSections, model.ScheduledSection{
				ID:         "overdue",
				Title:      "期限超過",
				Badge:      fmt.Sprintf("%d件", len(overdueItems)),
				BadgeClass: "badge-alert",
				Items:      overdueItems,
			})
		}
		// 今日枠は常に表示
		scheduledSections = append(scheduledSections, model.ScheduledSection{
			ID:         "today",
			Title:      "今日",
			Badge:      fmt.Sprintf("%d件", len(todayItems)),
			BadgeClass: "badge-today",
			Items:      todayItems,
		})
		if len(tomorrowItems) > 0 {
			scheduledSections = append(scheduledSections, model.ScheduledSection{
				ID:         "tomorrow",
				Title:      "明日",
				Badge:      fmt.Sprintf("%d件", len(tomorrowItems)),
				BadgeClass: "",
				Items:      tomorrowItems,
			})
		}
		if len(weekItems) > 0 {
			scheduledSections = append(scheduledSections, model.ScheduledSection{
				ID:         "week",
				Title:      "直近7日間",
				Badge:      fmt.Sprintf("%d件", len(weekItems)),
				BadgeClass: "",
				Items:      weekItems,
			})
		}
		if len(laterItems) > 0 {
			scheduledSections = append(scheduledSections, model.ScheduledSection{
				ID:         "later",
				Title:      "今後 (8日以降)",
				Badge:      fmt.Sprintf("%d件", len(laterItems)),
				BadgeClass: "",
				Items:      laterItems,
			})
		}
		if len(deferredItems) > 0 {
			scheduledSections = append(scheduledSections, model.ScheduledSection{
				ID:         "deferred",
				Title:      "開始待ち",
				Badge:      fmt.Sprintf("%d件", len(deferredItems)),
				BadgeClass: "",
				Items:      deferredItems,
			})
		}
	case "waiting":
		title = "Waiting For"
		description = "誰かの返信や成果物待ちのタスク。"
	case "someday":
		title = "Someday"
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

		// ステータス別アイテムの分類
		var nextItems, waitingItems, somedayItems, doneItems []model.Item
		for _, it := range items {
			switch it.Status {
			case "next", "inbox":
				nextItems = append(nextItems, it)
			case "waiting":
				waitingItems = append(waitingItems, it)
			case "someday", "reference":
				somedayItems = append(somedayItems, it)
			case "done":
				doneItems = append(doneItems, it)
			default:
				nextItems = append(nextItems, it)
			}
		}

		totalCount := len(items)
		doneCount := len(doneItems)
		progress := 0
		if totalCount > 0 {
			progress = int(math.Round(float64(doneCount) / float64(totalCount) * 100.0))
		}
		isStalled := (currentProject != nil && currentProject.Status == "active" && len(nextItems) == 0)

		projectStats = model.ProjectStats{
			TotalCount:   totalCount,
			DoneCount:    doneCount,
			NextCount:    len(nextItems),
			WaitingCount: len(waitingItems),
			SomedayCount: len(somedayItems),
			Progress:     progress,
			IsStalled:    isStalled,
		}

		// セクションの構築（itemStatusFilterに応じた絞り込み、空なら全セクション）
		if itemStatusFilter == "" || itemStatusFilter == "all" || itemStatusFilter == "next" {
			projectSections = append(projectSections, model.ProjectSection{
				ID:             "next",
				Title:          "Next Actions",
				Icon:           "next",
				BadgeClass:     "badge-next",
				Items:          nextItems,
				Count:          len(nextItems),
				Collapsed:      false,
				AddPlaceholder: "+ このプロジェクトのNext Actionを追加...",
			})
		}
		if itemStatusFilter == "" || itemStatusFilter == "all" || itemStatusFilter == "waiting" {
			projectSections = append(projectSections, model.ProjectSection{
				ID:             "waiting",
				Title:          "Waiting For",
				Icon:           "waiting",
				BadgeClass:     "badge-waiting",
				Items:          waitingItems,
				Count:          len(waitingItems),
				Collapsed:      false,
				AddPlaceholder: "+ 連絡待ち・依頼中のタスクを追加...",
			})
		}
		if itemStatusFilter == "" || itemStatusFilter == "all" || itemStatusFilter == "someday" {
			projectSections = append(projectSections, model.ProjectSection{
				ID:             "someday",
				Title:          "Someday / Ideas",
				Icon:           "someday",
				BadgeClass:     "badge-someday",
				Items:          somedayItems,
				Count:          len(somedayItems),
				Collapsed:      false,
				AddPlaceholder: "+ 将来やるタスク・アイデアを追加...",
			})
		}
		if itemStatusFilter == "" || itemStatusFilter == "all" || itemStatusFilter == "done" {
			projectSections = append(projectSections, model.ProjectSection{
				ID:             "done",
				Title:          "完了済み",
				Icon:           "done",
				BadgeClass:     "badge-done",
				Items:          doneItems,
				Count:          len(doneItems),
				Collapsed:      true, // 初期状態折りたたみ
				AddPlaceholder: "",
			})
		}
	case "projects":
		title = "プロジェクト管理・履歴"
		description = "進行中・保留中・完了済みの全プロジェクトの確認とステータス変更。"
	case "contexts":
		title = "Contexts 管理"
		description = "作業場所・状況・タグの確認、名前の変更、新規作成および削除。"
	}

	if currentContext != nil {
		title = title + " (" + currentContext.Name + ")"
	}

	return ItemListData{
		View:              view,
		Title:             title,
		Description:       description,
		Items:             items,
		ScheduledSections: scheduledSections,
		ProjectSections:   projectSections,
		ProjectStats:      projectStats,
		ItemStatusFilter:  itemStatusFilter,
		Contexts:          contexts,
		CurrentProject:    currentProject,
		CurrentContext:    currentContext,
		ProjectID:         pID,
		ContextID:         cID,
		Search:            search,
		Projects:          projects,
		ProjectStatus:     projectStatus,
		Counts:            counts,
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
	itemStatus := r.URL.Query().Get("item_status")

	sidebarData := h.buildSidebarData(view, selectedID)
	itemListData, err := h.buildItemListData(view, pID, cID, search, projectStatus, itemStatus)
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
	itemStatus := r.URL.Query().Get("item_status")

	data, err := h.buildItemListData(view, pID, cID, search, projectStatus, itemStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Render(w, "item_list.html", data)
}
