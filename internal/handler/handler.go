package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Handler はアプリケーション全体のハンドラおよび依存関係を保持します
type Handler struct {
	DB        *sql.DB
	Templates *template.Template
	ImagesDir string
}

// NewHandler はテンプレートと依存関係を初期化して Handler を作成します
func NewHandler(db *sql.DB, tmpl *template.Template, imagesDir string) *Handler {
	return &Handler{
		DB:        db,
		Templates: tmpl,
		ImagesDir: imagesDir,
	}
}

// Trigger は htmx 向けの HX-Trigger レスポンスヘッダを設定します
func (h *Handler) Trigger(w http.ResponseWriter, events ...string) {
	if len(events) == 0 {
		return
	}
	if len(events) == 1 {
		w.Header().Set("HX-Trigger", events[0])
		return
	}
	// 複数イベントの場合は JSON 形式で設定
	eventMap := make(map[string]bool)
	for _, e := range events {
		eventMap[e] = true
	}
	data, _ := json.Marshal(eventMap)
	w.Header().Set("HX-Trigger", string(data))
}

// Render は指定されたテンプレートを描画します
func (h *Handler) Render(w http.ResponseWriter, name string, data interface{}) {
	var buf bytes.Buffer
	err := h.Templates.ExecuteTemplate(&buf, name, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// TemplateFuncs はテンプレート内で利用できるユーティリティ関数群です
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"nl2br": func(text string) template.HTML {
			escaped := template.HTMLEscapeString(text)
			return template.HTML(strings.ReplaceAll(escaped, "\n", "<br>"))
		},
		"formatDate": func(t *time.Time) string {
			if t == nil || t.IsZero() {
				return ""
			}
			return t.Format("2006-01-02 15:04")
		},
		"formatShortDate": func(s *string) string {
			if s == nil || *s == "" {
				return ""
			}
			val := *s
			if idx := strings.Index(val, "T"); idx != -1 {
				return val[:idx]
			}
			if len(val) > 10 {
				return val[:10]
			}
			return val
		},
		"derefString": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"derefInt64": func(i *int64) int64 {
			if i == nil {
				return 0
			}
			return *i
		},
		"base": filepath.Base,
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}
}
