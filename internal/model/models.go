package model

import (
	"fmt"
	"math"
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

// FormattedDueDate はYYYY-MM-DD形式の期日文字列を返します
func (i Item) FormattedDueDate() string {
	if i.DueDate == nil || *i.DueDate == "" {
		return ""
	}
	s := *i.DueDate
	if idx := strings.Index(s, "T"); idx != -1 {
		return s[:idx]
	}
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// DueDisplay は期日を人間が一目で把握できる相対表記（今日、明日、明後日、N日後、期限超過など）で返します
func (i Item) DueDisplay() string {
	dStr := i.FormattedDueDate()
	if dStr == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02", dStr, time.Local)
	if err != nil {
		return dStr
	}
	return CalculateDueDisplay(t, time.Now().In(time.Local))
}

// CalculateDueDisplay は基準時刻 now に対する期日 t の相対表記を算出します（テスト等で利用可能）
func CalculateDueDisplay(t, now time.Time) string {
	todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dueZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	diffDays := int(math.Round(dueZero.Sub(todayZero).Hours() / 24.0))

	switch {
	case diffDays < -1:
		return fmt.Sprintf("期限超過 (%d日前)", -diffDays)
	case diffDays == -1:
		return "期限超過 (昨日)"
	case diffDays == 0:
		return "今日"
	case diffDays == 1:
		return "明日"
	case diffDays == 2:
		return "明後日"
	case diffDays >= 3 && diffDays <= 14:
		return fmt.Sprintf("%d日後", diffDays)
	default:
		// 15日以上先
		if t.Year() == now.Year() {
			return t.Format("1月2日")
		}
		return t.Format("2006年1月2日")
	}
}

// DueTooltip はマウスホバー時に表示する完全な期日詳細文字列を返します
func (i Item) DueTooltip() string {
	dStr := i.FormattedDueDate()
	if dStr == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02", dStr, time.Local)
	if err != nil {
		return "期限日: " + dStr
	}
	weekdays := []string{"日", "月", "火", "水", "木", "金", "土"}
	weekday := weekdays[t.Weekday()]
	return fmt.Sprintf("期限日: %s (%s) [%s]", dStr, weekday, i.DueDisplay())
}

// DueDateClass は期日に応じたCSSクラスを返します（超過、今日、直近、将来）
func (i Item) DueDateClass() string {
	dStr := i.FormattedDueDate()
	if dStr == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02", dStr, time.Local)
	if err != nil {
		return "due-future"
	}
	return CalculateDueDateClass(t, time.Now().In(time.Local))
}

// CalculateDueDateClass は基準時刻 now に対する期日 t のCSSクラスを算出します
func CalculateDueDateClass(t, now time.Time) string {
	todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dueZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	diffDays := int(math.Round(dueZero.Sub(todayZero).Hours() / 24.0))

	if diffDays < 0 {
		return "due-overdue"
	} else if diffDays == 0 {
		return "due-today"
	} else if diffDays <= 2 {
		return "due-soon"
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
	DoneProjects    int
	SomedayProjects int
}

// ItemFilter は中央リスト表示時の絞り込み条件です
type ItemFilter struct {
	View      string // 'inbox', 'next', 'waiting', 'someday', 'reference', 'done', 'project'
	ProjectID *int64
	ContextID *int64
	Search    string
}

