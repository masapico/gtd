package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gtd/internal/db"
	"gtd/internal/model"
)

// AddAttachment はURLまたはローカルパスの添付を登録します
func (h *Handler) AddAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := strings.TrimSpace(r.FormValue("target"))
	if target == "" {
		http.Error(w, "Target URL or Path is required", http.StatusBadRequest)
		return
	}

	label := strings.TrimSpace(r.FormValue("label"))
	attType := r.FormValue("type")

	// 種別の自動判定
	if attType == "" {
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			attType = "url"
		} else {
			attType = "path"
		}
	}

	att := &model.Attachment{
		ItemID: itemID,
		Label:  label,
		Target: target,
		Type:   attType,
	}

	_, err = db.CreateAttachment(h.DB, att)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	h.GetAttachments(w, r)
}

// PasteImage はクリップボードから送信された画像データを ./data/images/ に保存します
func (h *Handler) PasteImage(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// 20MBまでのマルチパートフォームを許容
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Image file not found in request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 拡張子判定（デフォルトは .png）
	ext := filepath.Ext(header.Filename)
	if ext == "" || (!strings.EqualFold(ext, ".png") && !strings.EqualFold(ext, ".jpg") && !strings.EqualFold(ext, ".jpeg")) {
		ext = ".png"
	}

	// ユニークなファイル名生成: screenshot_20060102_150405_random.png
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	filename := fmt.Sprintf("clip_%s_%s%s", time.Now().Format("20060102_150405"), hex.EncodeToString(randBytes), ext)

	destPath := filepath.Join(h.ImagesDir, filename)
	destFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "Failed to create image file", http.StatusInternalServerError)
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		http.Error(w, "Failed to write image data", http.StatusInternalServerError)
		return
	}

	// DB登録
	att := &model.Attachment{
		ItemID: itemID,
		Label:  "スクショ " + time.Now().Format("01/02 15:04"),
		Target: filename,
		Type:   "image",
	}

	_, err = db.CreateAttachment(h.DB, att)
	if err != nil {
		http.Error(w, "Failed to save attachment record", http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// GetAttachments はアイテムの添付一覧パーシャルを描画します
func (h *Handler) GetAttachments(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	attachments, err := db.GetAttachmentsByItemID(h.DB, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		ItemID      int64
		Attachments []model.Attachment
	}{
		ItemID:      itemID,
		Attachments: attachments,
	}

	h.Render(w, "attachments.html", data)
}

// OpenAttachment はローカルファイルパス（またはURL）をOS標準アプリケーションで起動します
func (h *Handler) OpenAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	att, err := db.GetAttachmentByID(h.DB, id)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	target := att.Target

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Windows: cmd /c start "" "target"
		cmd = exec.Command("cmd", "/c", "start", "", target)
	case "darwin":
		// macOS: open "target"
		cmd = exec.Command("open", target)
	default:
		// Linux: xdg-open "target"
		cmd = exec.Command("xdg-open", target)
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to open path: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteAttachment は添付ファイルを削除します
func (h *Handler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	att, err := db.DeleteAttachment(h.DB, id)
	if err != nil {
		http.Error(w, "Failed to delete attachment", http.StatusInternalServerError)
		return
	}

	// 画像実体ファイルのクリーンアップ
	if att.Type == "image" && att.Target != "" {
		filePath := filepath.Join(h.ImagesDir, filepath.Base(att.Target))
		_ = os.Remove(filePath)
	}

	h.Trigger(w, "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// ServeImage は保存されたスクリーンショット画像を配信します
func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.PathValue("filename"))
	filePath := filepath.Join(h.ImagesDir, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, filePath)
}

