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
	"time"

	"gtd/internal/backup"
	"gtd/internal/db"
	"gtd/internal/handler"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// Version is the application version, injected at build time via -ldflags "-X main.Version=..."
var Version = "dev"

func printUsage() {
	fmt.Println("GTD - Getting Things Done Task Manager")
	if Version != "" {
		fmt.Printf("Version: %s\n", Version)
	}
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gtd              Start GTD web server (default)")
	fmt.Println("  gtd backup       Create an online backup archive (.zip) in ./backup")
	fmt.Println("  gtd version      Show version information")
	fmt.Println("  gtd help         Show this help message")
}

func main() {
	// データベースおよび画像保存先パス
	dbPath := filepath.Join("data", "gtd.db")
	imagesDir := filepath.Join("data", "images")
	backupDir := "backup"

	// CLI サブコマンド処理
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			fmt.Println("📦 Starting GTD online backup...")
			res, err := backup.Run(dbPath, imagesDir, backupDir)
			if err != nil {
				log.Fatalf("❌ Backup failed: %v", err)
			}
			fmt.Println("✅ Backup completed successfully!")
			fmt.Printf("   Archive: %s\n", res.ZipPath)
			fmt.Printf("   Size:    %.2f KB\n", float64(res.FileSize)/1024)
			fmt.Printf("   Images:  %d files\n", res.ImageCount)
			fmt.Printf("   Time:    %v\n", res.Duration.Round(time.Millisecond))
			return
		case "version", "-v", "--version":
			fmt.Printf("GTD %s\n", Version)
			return
		case "help", "-h", "--help":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

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
	mux := setupRouter(h)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	addr := host + ":" + port
	fmt.Printf("\n✅ GTD (%s) is running at http://%s:%s\n\n", Version, host, port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// setupRouter はアプリケーションの HTTP ルーティングを構築します
func setupRouter(h *handler.Handler) *http.ServeMux {
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

	// Favicon ダイレクト配信 (ブラウザ標準リクエスト対応)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile("static/favicon.ico")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})
	mux.HandleFunc("GET /apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile("static/apple-touch-icon.png")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})

	// 保存されたスクショ画像配信
	mux.HandleFunc("GET /data/images/{filename}", h.ServeImage)

	// Views
	mux.HandleFunc("GET /", h.ServeIndex)
	mux.HandleFunc("GET /partials/sidebar", h.ServeSidebar)
	mux.HandleFunc("GET /partials/items", h.ServeItemList)
	mux.HandleFunc("GET /search", h.Search)

	// Items
	mux.HandleFunc("POST /items", h.CreateItem)
	mux.HandleFunc("GET /items/{id}/detail", h.GetItemDetail)
	mux.HandleFunc("PUT /items/{id}", h.UpdateItem)
	mux.HandleFunc("PATCH /items/{id}/status", h.UpdateItemStatus)
	mux.HandleFunc("DELETE /items/{id}", h.DeleteItem)
	mux.HandleFunc("POST /items/{id}/move", h.MoveItem)
	mux.HandleFunc("POST /items/{id}/contexts/{contextId}/toggle", h.ToggleItemContext)
	mux.HandleFunc("POST /items/{id}/attachments", h.AddAttachment)
	mux.HandleFunc("POST /items/{id}/paste-image", h.PasteImage)
	mux.HandleFunc("GET /items/{id}/attachments", h.GetAttachments)

	// Projects
	mux.HandleFunc("POST /projects", h.CreateProject)
	mux.HandleFunc("PUT /projects/{id}", h.UpdateProject)
	mux.HandleFunc("PATCH /projects/{id}/status", h.UpdateProjectStatus)
	mux.HandleFunc("PATCH /projects/{id}/type", h.UpdateProjectType)
	mux.HandleFunc("DELETE /projects/{id}", h.DeleteProject)

	// Contexts
	mux.HandleFunc("POST /contexts", h.CreateContext)
	mux.HandleFunc("PUT /contexts/{id}", h.UpdateContext)
	mux.HandleFunc("DELETE /contexts/{id}", h.DeleteContext)

	// Attachments (詳細・名称編集・OS起動・削除)
	mux.HandleFunc("GET /attachments/{id}", h.GetAttachmentItem)
	mux.HandleFunc("GET /attachments/{id}/edit", h.GetAttachmentEditForm)
	mux.HandleFunc("PUT /attachments/{id}", h.UpdateAttachment)
	mux.HandleFunc("POST /attachments/{id}/open", h.OpenAttachment)
	mux.HandleFunc("DELETE /attachments/{id}", h.DeleteAttachment)

	// Launchers (CRUD & OS起動)
	mux.HandleFunc("GET /partials/launchers", h.ServeLauncherModal)
	mux.HandleFunc("POST /launchers", h.CreateLauncher)
	mux.HandleFunc("GET /launchers/{id}/edit", h.GetLauncherEditForm)
	mux.HandleFunc("PUT /launchers/{id}", h.UpdateLauncher)
	mux.HandleFunc("DELETE /launchers/{id}", h.DeleteLauncher)
	mux.HandleFunc("POST /launchers/{id}/launch", h.LaunchApp)

	// Weekly Review
	mux.HandleFunc("GET /review", h.StartReview)
	mux.HandleFunc("GET /review/step/{step}", h.ReviewStep)
	mux.HandleFunc("POST /review/projects/{id}/reviewed", h.MarkProjectReviewed)
	mux.HandleFunc("POST /review/projects/{id}/items", h.AddProjectItemInReview)
	mux.HandleFunc("POST /review/items/{id}/reviewed", h.MarkItemReviewed)

	return mux
}
