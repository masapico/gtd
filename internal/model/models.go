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
	StartDate      *string      `json:"start_date"` // 開始日（Tickler / Defer Date）
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

// FormattedStartDate はYYYY-MM-DD形式の開始日文字列を返します
func (i Item) FormattedStartDate() string {
	if i.StartDate == nil || *i.StartDate == "" {
		return ""
	}
	s := *i.StartDate
	if idx := strings.Index(s, "T"); idx != -1 {
		return s[:idx]
	}
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// IsDeferred は開始日が未来の日付（明日以降）に設定されていて、まだ着手待機状態（Tickler）かを判定します
func (i Item) IsDeferred() bool {
	sStr := i.FormattedStartDate()
	if sStr == "" {
		return false
	}
	t, err := time.ParseInLocation("2006-01-02", sStr, time.Local)
	if err != nil {
		return false
	}
	now := time.Now().In(time.Local)
	todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	startZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	return startZero.After(todayZero)
}

// StartDisplay は開始日の人間向け相対表記（今日開始、明日開始、N日後開始など）を返します
func (i Item) StartDisplay() string {
	sStr := i.FormattedStartDate()
	if sStr == "" {
		return ""
	}
	t, err := time.ParseInLocation("2006-01-02", sStr, time.Local)
	if err != nil {
		return sStr
	}
	return CalculateStartDisplay(t, time.Now().In(time.Local))
}

// CalculateStartDisplay は基準時刻 now に対する開始日 t の相対表記を算出します
func CalculateStartDisplay(t, now time.Time) string {
	todayZero := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	startZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	diffDays := int(math.Round(startZero.Sub(todayZero).Hours() / 24.0))

	switch {
	case diffDays <= -1:
		return "開始日経過"
	case diffDays == 0:
		return "今日から開始可能"
	case diffDays == 1:
		return "明日から開始"
	case diffDays == 2:
		return "明後日から開始"
	case diffDays >= 3 && diffDays <= 14:
		return fmt.Sprintf("%d日後から開始", diffDays)
	default:
		if t.Year() == now.Year() {
			return fmt.Sprintf("%sから開始", t.Format("1月2日"))
		}
		return fmt.Sprintf("%sから開始", t.Format("2006年1月2日"))
	}
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
	Inbox                 int
	Next                  int
	ScheduledOverdueToday int // 期限超過または今日が期限のタスク件数（アラート用）
	ScheduledTotal        int // 期限または未来開始が設定されているタスクの総件数
	Waiting               int
	Someday               int
	Reference             int
	Done                  int
	Projects              int
	StalledProjects       int
	DoneProjects          int
	SomedayProjects       int
}

// ScheduledSection は期限・予定ビューでアイテムを時間枠別にグループ表示するための構造体です
type ScheduledSection struct {
	ID         string `json:"id"`          // 'overdue', 'today', 'tomorrow', 'week', 'later', 'deferred'
	Title      string `json:"title"`       // 表示タイトル（例: '期限超過', '今日'）
	Badge      string `json:"badge"`       // '2件' など
	BadgeClass string `json:"badge_class"` // 'badge-alert', 'badge-today' など
	Items      []Item `json:"items"`
}

// ItemFilter は中央リスト表示時の絞り込み条件です
type ItemFilter struct {
	View      string // 'inbox', 'next', 'scheduled', 'waiting', 'someday', 'reference', 'done', 'project'
	ProjectID *int64
	ContextID *int64
	Search    string
}

// Launcher はよく使うアプリやファイル、URLを開くためのショートカットを表します
type Launcher struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Target      string    `json:"target"`      // 実行可能ファイルパス、フォルダパス、URL
	Type        string    `json:"type"`        // 'app', 'folder', 'file', 'url'
	Description string    `json:"description"` // 説明または検索キーワード
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TypeLabel は種別の日本語ラベルを返します
func (l Launcher) TypeLabel() string {
	switch l.Type {
	case "app":
		return "アプリ"
	case "folder":
		return "フォルダー"
	case "url":
		return "Webリンク"
	case "file":
		return "ファイル"
	default:
		return "ランチャー"
	}
}

// CleanTargetName はターゲットの簡潔な表示名を返します
func (l Launcher) CleanTargetName() string {
	if l.Type == "url" {
		return l.Target
	}
	parts := strings.Split(strings.ReplaceAll(l.Target, "\\", "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return l.Target
}

// OmniSearchResult はオムニサーチ全体の検索結果を表します
type OmniSearchResult struct {
	Query       string                 `json:"query"`
	Launchers   []Launcher             `json:"launchers"`
	Items       []Item                 `json:"items"`
	Attachments []OmniAttachmentResult `json:"attachments"`
	TotalCount  int                    `json:"total_count"`
}

// OmniAttachmentResult は添付ファイル検索でヒットした項目を表します
type OmniAttachmentResult struct {
	ID         int64  `json:"id"`
	ItemID     int64  `json:"item_id"`
	ItemTitle  string `json:"item_title"`
	ItemStatus string `json:"item_status"`
	Label      string `json:"label"`
	Target     string `json:"target"`
	Type       string `json:"type"` // 'url', 'path', 'image'
	MatchField string `json:"match_field"` // 'label' or 'target'
}

// CleanTargetName はパスやURLの短い表示名を返します
func (a OmniAttachmentResult) CleanTargetName() string {
	if a.Label != "" {
		return a.Label
	}
	parts := strings.Split(strings.ReplaceAll(a.Target, "\\", "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return a.Target
}


