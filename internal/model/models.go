package model

import (
	"strings"
	"time"
)

// Project は複数の行動を要するゴールを表します
type Project struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Status          string     `json:"status"` // 'active', 'someday', 'done'
	LastReviewedAt  *time.Time `json:"last_reviewed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	NextActionCount int        `json:"next_action_count"`
	TotalItemCount  int        `json:"total_item_count"`
}

// IsStalled は、アクティブなのにNext Actionが0件のプロジェクトかどうかを判定します
func (p Project) IsStalled() bool {
	return p.Status == "active" && p.NextActionCount == 0
}

// Context は場所・状況・相手のタグを表します（例: @PC, @会議）
type Context struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ItemCount int    `json:"item_count,omitempty"`
}

// Attachment は関連ファイル、URL、スクリーンショット画像を表します
type Attachment struct {
	ID        int64     `json:"id"`
	ItemID    int64     `json:"item_id"`
	Label     string    `json:"label"`
	Target    string    `json:"target"` // URL、ファイルパス、画像ファイル名
	Type      string    `json:"type"`   // 'url', 'path', 'image'
	CreatedAt time.Time `json:"created_at"`
}

// Item は Inbox / タスク / メモを統合管理するモデルです
type Item struct {
	ID             int64        `json:"id"`
	ProjectID      *int64       `json:"project_id"`
	ProjectName    string       `json:"project_name"`
	Title          string       `json:"title"`
	Body           string       `json:"body"`
	Status         string       `json:"status"` // 'inbox', 'next', 'waiting', 'someday', 'reference', 'done'
	DueDate        *string      `json:"due_date"`
	Recurrence     string       `json:"recurrence"` // 'none', 'daily', 'weekdays', 'weekly', 'monthly', 'yearly'
	LastReviewedAt *time.Time   `json:"last_reviewed_at"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Contexts       []Context    `json:"contexts"`
	Attachments    []Attachment `json:"attachments"`
	AttachCount    int          `json:"attach_count"`
}

// RecurrenceLabel は繰り返し設定の日本語表示ラベルを返します
func (i Item) RecurrenceLabel() string {
	switch i.Recurrence {
	case "daily":
		return "毎日"
	case "weekdays":
		return "平日(月〜金)"
	case "weekly":
		return "毎週"
	case "monthly":
		return "毎月"
	case "yearly":
		return "毎年"
	default:
		return ""
	}
}

// StatusLabel はステータスの日本語表示ラベルを返します
func (i Item) StatusLabel() string {
	switch i.Status {
	case "inbox":
		return "Inbox"
	case "next":
		return "Next Action"
	case "waiting":
		return "Waiting For"
	case "someday":
		return "Someday"
	case "reference":
		return "Reference"
	case "done":
		return "Done"
	default:
		return i.Status
	}
}

// HasContext は指定されたContextIDが含まれているかを判定します
func (i Item) HasContext(ctxID int64) bool {
	for _, c := range i.Contexts {
		if c.ID == ctxID {
			return true
		}
	}
	return false
}

// DueDateClass は期日に応じたCSSクラスを返します（超過、今日、将来）
func (i Item) DueDateClass() string {
	if i.DueDate == nil || *i.DueDate == "" {
		return ""
	}
	today := time.Now().Format("2006-01-02")
	if *i.DueDate < today {
		return "due-overdue"
	} else if *i.DueDate == today {
		return "due-today"
	}
	return "due-future"
}

// CleanTargetName はパスやURLの短い表示名を返します
func (a Attachment) CleanTargetName() string {
	if a.Label != "" {
		return a.Label
	}
	parts := strings.Split(strings.ReplaceAll(a.Target, "\\", "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return a.Target
}

// SidebarCounts は左ペインの各ビューおよびバッジ件数を保持します
type SidebarCounts struct {
	Inbox           int
	Next            int
	Waiting         int
	Someday         int
	Reference       int
	Done            int
	Projects        int
	StalledProjects int
}

// ItemFilter は中央リスト表示時の絞り込み条件です
type ItemFilter struct {
	View      string // 'inbox', 'next', 'waiting', 'someday', 'reference', 'done', 'project'
	ProjectID *int64
	ContextID *int64
	Search    string
}

