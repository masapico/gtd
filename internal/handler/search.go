package handler

import (
	"net/http"
	"strings"

	"gtd/internal/db"
)

// Search はオムニサーチのクエリ結果（パーシャルHTML）を描画します
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(""))
		return
	}

	res, err := db.SearchOmni(h.DB, q, 20, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Render(w, "search_results.html", res)
}

