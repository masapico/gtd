package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gtd/internal/db"
	"gtd/internal/handler"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

func main() {
	// データベースおよび画像保存先パス
	dbPath := filepath.Join("data", "gtd.db")
	imagesDir := filepath.Join("data", "images")

	// DB初期化（マイグレーション・初期コンテキストシード）
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// テンプレート解析 (embedされたFSから読み込み)
	tmpl, err := template.New("").Funcs(handler.TemplateFuncs()).ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/partials/*.html",
	)
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// ハンドラインスタンス生成
	h := handler.NewHandler(database, tmpl, imagesDir)

	// ルーター構築 (Go 1.22+ Standard HTTP Routing)
	mux := http.NewServeMux()

	// 静的アセット配信 (static/)
	subStatic, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(subStatic))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		fileServer.ServeHTTP(w, r)
	})))

	// 保存されたスクショ画像配信
	mux.HandleFunc("GET /data/images/{filename}", h.ServeImage)

	// Views
	mux.HandleFunc("GET /", h.ServeIndex)
	mux.HandleFunc("GET /partials/sidebar", h.ServeSidebar)
	mux.HandleFunc("GET /partials/items", h.ServeItemList)

	// Items
	mux.HandleFunc("POST /items", h.CreateItem)
	mux.HandleFunc("GET /items/{id}/detail", h.GetItemDetail)
	mux.HandleFunc("PUT /items/{id}", h.UpdateItem)
	mux.HandleFunc("PATCH /items/{id}/status", h.UpdateItemStatus)
	mux.HandleFunc("DELETE /items/{id}", h.DeleteItem)
	mux.HandleFunc("POST /items/{id}/contexts/{contextId}/toggle", h.ToggleItemContext)
	mux.HandleFunc("POST /items/{id}/attachments", h.AddAttachment)
	mux.HandleFunc("POST /items/{id}/paste-image", h.PasteImage)
	mux.HandleFunc("GET /items/{id}/attachments", h.GetAttachments)

	// Projects
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("PUT /projects/{id}", h.UpdateProject)
	mux.HandleFunc("PATCH /projects/{id}/status", h.UpdateProjectStatus)
	mux.HandleFunc("DELETE /projects/{id}", h.DeleteProject)

	// Contexts
	mux.HandleFunc("POST /contexts", h.CreateContext)
	mux.HandleFunc("PUT /contexts/{id}", h.UpdateContext)
	mux.HandleFunc("DELETE /contexts/{id}", h.DeleteContext)

	// Attachments (OS起動・削除)
	mux.HandleFunc("POST /attachments/{id}/open", h.OpenAttachment)
	mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)

	// Weekly Review
	mux.HandleFunc("GET /review", h.StartReview)
	mux.HandleFunc("GET /review/step/{step}", h.ReviewStep)
	mux.HandleFunc("POST /review/projects/{id}/reviewed", h.MarkProjectReviewed)
	mux.HandleFunc("POST /review/items/{id}/reviewed", h.MarkItemReviewed)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	fmt.Printf("\n✅ GTD is running at http://localhost:%s\n\n", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
