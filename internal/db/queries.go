package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gtd/internal/model"
)

// CalculateNextDueDate は繰り返しルールに基づいて次回期日（YYYY-MM-DD）を算出します
func CalculateNextDueDate(baseDateStr string, recurrence string) string {
	var base time.Time
	var err error
	if baseDateStr != "" {
		base, err = time.Parse("2006-01-02", baseDateStr)
	}
	if err != nil || base.IsZero() {
		base = time.Now()
	}

	next := base
	switch recurrence {
	case "daily":
		next = next.AddDate(0, 0, 1)
	case "weekdays":
		next = next.AddDate(0, 0, 1)
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
	case "weekly":
		next = next.AddDate(0, 0, 7)
	case "monthly":
		next = addMonths(next, 1)
	case "yearly":
		next = next.AddDate(1, 0, 0)
	default:
		return ""
	}

	// もし期日が過去の日付のまま完了された場合、今日以降の日付まで進める
	today := time.Now().Format("2006-01-02")
	for next.Format("2006-01-02") < today {
		switch recurrence {
		case "daily":
			next = next.AddDate(0, 0, 1)
		case "weekdays":
			next = next.AddDate(0, 0, 1)
			for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
				next = next.AddDate(0, 0, 1)
			}
		case "weekly":
			next = next.AddDate(0, 0, 7)
		case "monthly":
			next = addMonths(next, 1)
		case "yearly":
			next = next.AddDate(1, 0, 0)
		}
	}

	return next.Format("2006-01-02")
}

func addMonths(t time.Time, months int) time.Time {
	year, month, day := t.Date()
	targetMonth := month + time.Month(months)
	firstOfTarget := time.Date(year, targetMonth, 1, 0, 0, 0, 0, t.Location())
	lastDay := firstOfTarget.AddDate(0, 1, -1).Day()
	targetDay := day
	if targetDay > lastDay {
		targetDay = lastDay
	}
	return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), targetDay, 0, 0, 0, 0, t.Location())
}

// GetSidebarCounts はサイドバーに表示する各ビューのタスク件数とプロジェクト件数を取得します
func GetSidebarCounts(db *sql.DB) (model.SidebarCounts, error) {
	counts := model.SidebarCounts{}
	rows, err := db.Query(`
		SELECT status, COUNT(*) 
		FROM items 
		GROUP BY status
	`)
	if err != nil {
		return counts, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return counts, err
		}
		switch status {
		case "inbox":
			counts.Inbox = count
		case "next":
			counts.Next = count
		case "waiting":
			counts.Waiting = count
		case "someday":
			counts.Someday = count
		case "reference":
			counts.Reference = count
		case "done":
			counts.Done = count
		}
	}

	// プロジェクト総数（active）
	_ = db.QueryRow(`SELECT COUNT(*) FROM projects WHERE status = 'active'`).Scan(&counts.Projects)

	// 止まっているプロジェクト（Next Actionが0件のアクティブプロジェクト）
	_ = db.QueryRow(`
		SELECT COUNT(*) 
		FROM projects p 
		WHERE p.status = 'active' 
		  AND (SELECT COUNT(*) FROM items i WHERE i.project_id = p.id AND i.status = 'next') = 0
	`).Scan(&counts.StalledProjects)

	return counts, nil
}

// GetItems はフィルタ条件に従ってアイテム一覧を取得します
func GetItems(db *sql.DB, filter model.ItemFilter) ([]model.Item, error) {
	var conditions []string
	var args []interface{}

	if filter.View != "" {
		if filter.View == "project" {
			// プロジェクト別表示の場合は全ステータス（または未完了優先）
			if filter.ProjectID != nil {
				conditions = append(conditions, "i.project_id = ?")
				args = append(args, *filter.ProjectID)
			}
		} else {
			conditions = append(conditions, "i.status = ?")
			args = append(args, filter.View)
		}
	}

	if filter.ContextID != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM item_contexts ic WHERE ic.item_id = i.id AND ic.context_id = ?)")
		args = append(args, *filter.ContextID)
	}

	if filter.Search != "" {
		conditions = append(conditions, "(i.title LIKE ? OR i.body LIKE ?)")
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm, searchTerm)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// ビューに応じたソート順
	orderBy := "i.created_at DESC"
	switch filter.View {
	case "inbox":
		orderBy = "i.created_at ASC"
	case "next":
		orderBy = "CASE WHEN i.due_date IS NULL OR i.due_date = '' THEN 1 ELSE 0 END, i.due_date ASC, i.created_at DESC"
	case "done":
		orderBy = "i.updated_at DESC"
	case "project":
		orderBy = "CASE i.status WHEN 'next' THEN 1 WHEN 'inbox' THEN 2 WHEN 'waiting' THEN 3 WHEN 'someday' THEN 4 WHEN 'reference' THEN 5 ELSE 6 END, i.created_at DESC"
	}

	query := fmt.Sprintf(`
		SELECT 
			i.id, i.project_id, COALESCE(p.name, '') AS project_name, 
			i.title, COALESCE(i.body, '') AS body, i.status, 
			i.due_date, i.recurrence, i.last_reviewed_at, i.created_at, i.updated_at,
			(SELECT COUNT(*) FROM attachments a WHERE a.item_id = i.id) AS attach_count
		FROM items i
		LEFT JOIN projects p ON i.project_id = p.id
		%s
		ORDER BY %s
	`, whereClause, orderBy)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var it model.Item
		var pID sql.NullInt64
		var dDate sql.NullString
		var lRev sql.NullTime

		err := rows.Scan(
			&it.ID, &pID, &it.ProjectName,
			&it.Title, &it.Body, &it.Status,
			&dDate, &it.Recurrence, &lRev,
			&it.CreatedAt, &it.UpdatedAt,
			&it.AttachCount,
		)
		if err != nil {
			return nil, err
		}

		if pID.Valid {
			it.ProjectID = &pID.Int64
		}
		if dDate.Valid && dDate.String != "" {
			it.DueDate = &dDate.String
		}
		if lRev.Valid {
			it.LastReviewedAt = &lRev.Time
		}

		items = append(items, it)
	}

	// 各アイテムのContextを取得
	for idx := range items {
		ctxRows, err := db.Query(`
			SELECT c.id, c.name
			FROM contexts c
			JOIN item_contexts ic ON c.id = ic.context_id
			WHERE ic.item_id = ?
			ORDER BY c.name ASC
		`, items[idx].ID)
		if err == nil {
			var contexts []model.Context
			for ctxRows.Next() {
				var c model.Context
				if err := ctxRows.Scan(&c.ID, &c.Name); err == nil {
					contexts = append(contexts, c)
				}
			}
			ctxRows.Close()
			items[idx].Contexts = contexts
		}
	}

	return items, nil
}

// GetItemByID は単一のアイテム（詳細情報、Context、Attachmentを含む）を取得します
func GetItemByID(db *sql.DB, id int64) (*model.Item, error) {
	row := db.QueryRow(`
		SELECT 
			i.id, i.project_id, COALESCE(p.name, '') AS project_name, 
			i.title, COALESCE(i.body, '') AS body, i.status, 
			i.due_date, i.recurrence, i.last_reviewed_at, i.created_at, i.updated_at
		FROM items i
		LEFT JOIN projects p ON i.project_id = p.id
		WHERE i.id = ?
	`, id)

	var it model.Item
	var pID sql.NullInt64
	var dDate sql.NullString
	var lRev sql.NullTime

	err := row.Scan(
		&it.ID, &pID, &it.ProjectName,
		&it.Title, &it.Body, &it.Status,
		&dDate, &it.Recurrence, &lRev,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if pID.Valid {
		it.ProjectID = &pID.Int64
	}
	if dDate.Valid && dDate.String != "" {
		it.DueDate = &dDate.String
	}
	if lRev.Valid {
		it.LastReviewedAt = &lRev.Time
	}

	// Contexts取得
	ctxRows, err := db.Query(`
		SELECT c.id, c.name
		FROM contexts c
		JOIN item_contexts ic ON c.id = ic.context_id
		WHERE ic.item_id = ?
		ORDER BY c.name ASC
	`, it.ID)
	if err == nil {
		for ctxRows.Next() {
			var c model.Context
			if err := ctxRows.Scan(&c.ID, &c.Name); err == nil {
				it.Contexts = append(it.Contexts, c)
			}
		}
		ctxRows.Close()
	}

	// Attachments取得
	attRows, err := db.Query(`
		SELECT id, item_id, COALESCE(label, ''), target, type, created_at
		FROM attachments
		WHERE item_id = ?
		ORDER BY created_at ASC
	`, it.ID)
	if err == nil {
		for attRows.Next() {
			var a model.Attachment
			if err := attRows.Scan(&a.ID, &a.ItemID, &a.Label, &a.Target, &a.Type, &a.CreatedAt); err == nil {
				it.Attachments = append(it.Attachments, a)
			}
		}
		attRows.Close()
	}
	it.AttachCount = len(it.Attachments)

	return &it, nil
}

// CreateItem は新しいアイテムを登録します
func CreateItem(db *sql.DB, item *model.Item) (int64, error) {
	if item.Status == "" {
		item.Status = "inbox"
	}
	if item.Recurrence == "" {
		item.Recurrence = "none"
	}

	res, err := db.Exec(`
		INSERT INTO items (project_id, title, body, status, due_date, recurrence)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ProjectID, item.Title, item.Body, item.Status, item.DueDate, item.Recurrence)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	item.ID = id

	// Context の保存
	for _, ctx := range item.Contexts {
		_, _ = db.Exec("INSERT OR IGNORE INTO item_contexts (item_id, context_id) VALUES (?, ?)", id, ctx.ID)
	}

	return id, nil
}

// UpdateItem はアイテムの詳細情報を更新します
func UpdateItem(db *sql.DB, item *model.Item) error {
	_, err := db.Exec(`
		UPDATE items
		SET project_id = ?, title = ?, body = ?, status = ?, due_date = ?, recurrence = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, item.ProjectID, item.Title, item.Body, item.Status, item.DueDate, item.Recurrence, item.ID)
	return err
}

// UpdateItemStatus はステータスを変更します。繰り返しタスクが 'done' になった場合は次期タスクを自動生成します
func UpdateItemStatus(db *sql.DB, id int64, newStatus string) (*model.Item, error) {
	it, err := GetItemByID(db, id)
	if err != nil {
		return nil, err
	}

	// 繰り返しタスクが完了（done）された場合、次期タスクを自動生成
	if newStatus == "done" && it.Recurrence != "none" && it.Recurrence != "" {
		baseDate := ""
		if it.DueDate != nil {
			baseDate = *it.DueDate
		}
		nextDue := CalculateNextDueDate(baseDate, it.Recurrence)

		// 次回インスタンスを生成
		nextItem := &model.Item{
			ProjectID:  it.ProjectID,
			Title:      it.Title,
			Body:       it.Body,
			Status:     "next",
			DueDate:    &nextDue,
			Recurrence: it.Recurrence,
			Contexts:   it.Contexts,
		}
		_, _ = CreateItem(db, nextItem)

		// 完了した現在のタスクは完了ログとして残し、重複自動生成を防ぐため recurrence を 'none' に解除
		_, _ = db.Exec(`UPDATE items SET recurrence = 'none' WHERE id = ?`, id)
	}

	_, err = db.Exec(`
		UPDATE items
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, newStatus, id)
	if err != nil {
		return nil, err
	}

	it.Status = newStatus
	return it, nil
}

// DeleteItem はアイテムを削除します（カスケードにより関連Context・Attachmentレコードも削除）
func DeleteItem(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM items WHERE id = ?`, id)
	return err
}

// ToggleItemContext はアイテムへのContextタグを付加・解除します
func ToggleItemContext(db *sql.DB, itemID int64, contextID int64) error {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM item_contexts WHERE item_id = ? AND context_id = ?
	`, itemID, contextID).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		_, err = db.Exec(`DELETE FROM item_contexts WHERE item_id = ? AND context_id = ?`, itemID, contextID)
	} else {
		_, err = db.Exec(`INSERT OR IGNORE INTO item_contexts (item_id, context_id) VALUES (?, ?)`, itemID, contextID)
	}
	return err
}

// Projects Queries

// GetProjects はステータス別にプロジェクト一覧（Next Action数付き）を取得します
func GetProjects(db *sql.DB, status string) ([]model.Project, error) {
	query := `
		SELECT 
			p.id, p.name, COALESCE(p.description, ''), p.status, 
			p.last_reviewed_at, p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM items i WHERE i.project_id = p.id AND i.status = 'next') AS next_action_count,
			(SELECT COUNT(*) FROM items i WHERE i.project_id = p.id) AS total_item_count
		FROM projects p
	`
	var rows *sql.Rows
	var err error
	if status != "" {
		query += " WHERE p.status = ? ORDER BY p.name ASC"
		rows, err = db.Query(query, status)
	} else {
		query += " ORDER BY p.name ASC"
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		var lRev sql.NullTime
		err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.Status,
			&lRev, &p.CreatedAt, &p.UpdatedAt,
			&p.NextActionCount, &p.TotalItemCount,
		)
		if err != nil {
			return nil, err
		}
		if lRev.Valid {
			p.LastReviewedAt = &lRev.Time
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// GetProjectByID は単一のプロジェクトを取得します
func GetProjectByID(db *sql.DB, id int64) (*model.Project, error) {
	row := db.QueryRow(`
		SELECT 
			p.id, p.name, COALESCE(p.description, ''), p.status, 
			p.last_reviewed_at, p.created_at, p.updated_at,
			(SELECT COUNT(*) FROM items i WHERE i.project_id = p.id AND i.status = 'next') AS next_action_count,
			(SELECT COUNT(*) FROM items i WHERE i.project_id = p.id) AS total_item_count
		FROM projects p
		WHERE p.id = ?
	`, id)

	var p model.Project
	var lRev sql.NullTime
	err := row.Scan(
		&p.ID, &p.Name, &p.Description, &p.Status,
		&lRev, &p.CreatedAt, &p.UpdatedAt,
		&p.NextActionCount, &p.TotalItemCount,
	)
	if err != nil {
		return nil, err
	}
	if lRev.Valid {
		p.LastReviewedAt = &lRev.Time
	}
	return &p, nil
}

// CreateProject は新しいプロジェクトを作成します
func CreateProject(db *sql.DB, p *model.Project) (int64, error) {
	if p.Status == "" {
		p.Status = "active"
	}
	res, err := db.Exec(`
		INSERT INTO projects (name, description, status)
		VALUES (?, ?, ?)
	`, p.Name, p.Description, p.Status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateProject はプロジェクト情報を更新します
func UpdateProject(db *sql.DB, p *model.Project) error {
	_, err := db.Exec(`
		UPDATE projects
		SET name = ?, description = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, p.Name, p.Description, p.Status, p.ID)
	return err
}

// UpdateProjectStatus はプロジェクトのステータスを変更します
func UpdateProjectStatus(db *sql.DB, id int64, status string) error {
	_, err := db.Exec(`
		UPDATE projects
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, id)
	return err
}

// DeleteProject はプロジェクトを削除します（所属アイテムのproject_idはNULLになります）
func DeleteProject(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// MarkProjectReviewed はプロジェクトのレビュー日時を現在日時に更新します
func MarkProjectReviewed(db *sql.DB, id int64) error {
	_, err := db.Exec(`
		UPDATE projects
		SET last_reviewed_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	return err
}

// Contexts Queries

// GetContexts は全コンテキスト（タスク件数付き）を取得します
func GetContexts(db *sql.DB) ([]model.Context, error) {
	rows, err := db.Query(`
		SELECT c.id, c.name, COUNT(ic.item_id) AS item_count
		FROM contexts c
		LEFT JOIN item_contexts ic ON c.id = ic.context_id
		LEFT JOIN items i ON ic.item_id = i.id AND i.status != 'done'
		GROUP BY c.id
		ORDER BY c.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contexts []model.Context
	for rows.Next() {
		var c model.Context
		if err := rows.Scan(&c.ID, &c.Name, &c.ItemCount); err != nil {
			return nil, err
		}
		contexts = append(contexts, c)
	}
	return contexts, nil
}

// CreateContext は新しいコンテキストタグを作成します
func CreateContext(db *sql.DB, name string) (int64, error) {
	if !strings.HasPrefix(name, "@") {
		name = "@" + name
	}
	res, err := db.Exec(`INSERT OR IGNORE INTO contexts (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteContext はコンテキストを削除します
func DeleteContext(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM contexts WHERE id = ?`, id)
	return err
}

// Attachments Queries

// GetAttachmentsByItemID はアイテムに紐づく全添付ファイルを取得します
func GetAttachmentsByItemID(db *sql.DB, itemID int64) ([]model.Attachment, error) {
	rows, err := db.Query(`
		SELECT id, item_id, COALESCE(label, ''), target, type, created_at
		FROM attachments
		WHERE item_id = ?
		ORDER BY created_at ASC
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Attachment
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.ItemID, &a.Label, &a.Target, &a.Type, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

// GetAttachmentByID は単一の添付ファイルを取得します
func GetAttachmentByID(db *sql.DB, id int64) (*model.Attachment, error) {
	row := db.QueryRow(`
		SELECT id, item_id, COALESCE(label, ''), target, type, created_at
		FROM attachments
		WHERE id = ?
	`, id)
	var a model.Attachment
	if err := row.Scan(&a.ID, &a.ItemID, &a.Label, &a.Target, &a.Type, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateAttachment は添付ファイルレコードを作成します
func CreateAttachment(db *sql.DB, a *model.Attachment) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO attachments (item_id, label, target, type)
		VALUES (?, ?, ?, ?)
	`, a.ItemID, a.Label, a.Target, a.Type)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteAttachment は添付ファイルを削除します
func DeleteAttachment(db *sql.DB, id int64) (*model.Attachment, error) {
	att, err := GetAttachmentByID(db, id)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`DELETE FROM attachments WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	return att, nil
}

// Review Queries

// GetInboxItems は週次レビュー Step 1 用の未処理アイテム一覧を取得します
func GetInboxItems(db *sql.DB) ([]model.Item, error) {
	return GetItems(db, model.ItemFilter{View: "inbox"})
}

// GetActiveProjectsForReview は週次レビュー Step 2 用に、Next Action数昇順でアクティブプロジェクト一覧を取得します
func GetActiveProjectsForReview(db *sql.DB) ([]model.Project, error) {
	rows, err := db.Query(`
		SELECT 
			p.id, p.name, COALESCE(p.description, ''), p.status, 
			p.last_reviewed_at, p.created_at, p.updated_at,
			COUNT(i.id) AS next_action_count,
			(SELECT COUNT(*) FROM items it WHERE it.project_id = p.id) AS total_item_count
		FROM projects p
		LEFT JOIN items i ON p.id = i.project_id AND i.status = 'next'
		WHERE p.status = 'active'
		GROUP BY p.id
		ORDER BY next_action_count ASC, p.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		var lRev sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.Status,
			&lRev, &p.CreatedAt, &p.UpdatedAt,
			&p.NextActionCount, &p.TotalItemCount,
		); err != nil {
			return nil, err
		}
		if lRev.Valid {
			p.LastReviewedAt = &lRev.Time
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// GetStaleItemsForReview は週次レビュー Step 3 用に、7日以上棚卸しされていない Waiting / Someday アイテムを取得します
func GetStaleItemsForReview(db *sql.DB) ([]model.Item, error) {
	rows, err := db.Query(`
		SELECT 
			i.id, i.project_id, COALESCE(p.name, '') AS project_name, 
			i.title, COALESCE(i.body, '') AS body, i.status, 
			i.due_date, i.recurrence, i.last_reviewed_at, i.created_at, i.updated_at,
			(SELECT COUNT(*) FROM attachments a WHERE a.item_id = i.id) AS attach_count
		FROM items i
		LEFT JOIN projects p ON i.project_id = p.id
		WHERE i.status IN ('waiting', 'someday')
		  AND (i.last_reviewed_at IS NULL OR i.last_reviewed_at <= datetime('now', '-7 days'))
		ORDER BY i.status ASC, i.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var it model.Item
		var pID sql.NullInt64
		var dDate sql.NullString
		var lRev sql.NullTime

		if err := rows.Scan(
			&it.ID, &pID, &it.ProjectName,
			&it.Title, &it.Body, &it.Status,
			&dDate, &it.Recurrence, &lRev,
			&it.CreatedAt, &it.UpdatedAt,
			&it.AttachCount,
		); err != nil {
			return nil, err
		}

		if pID.Valid {
			it.ProjectID = &pID.Int64
		}
		if dDate.Valid {
			it.DueDate = &dDate.String
		}
		if lRev.Valid {
			it.LastReviewedAt = &lRev.Time
		}
		items = append(items, it)
	}
	return items, nil
}

// MarkItemReviewed はアイテムのレビュー日時を現在日時に更新します
func MarkItemReviewed(db *sql.DB, id int64) error {
	_, err := db.Exec(`
		UPDATE items
		SET last_reviewed_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	return err
}

