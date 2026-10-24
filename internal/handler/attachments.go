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

// CleanPathInput は入力されたパス文字列を正規化・クレンジングします
func CleanPathInput(input string) string {
	target := strings.TrimSpace(input)
	// 前後の引用符（" または '）を除去（Windowsのエクスプローラー「パスのコピー」対策）
	target = strings.Trim(target, "\"'")
	target = strings.TrimSpace(target)

	// file:// スキームの正規化
	if strings.HasPrefix(target, "file:///") {
		// file:///C:/... または file:///Users/...
		target = strings.TrimPrefix(target, "file://")
		// Windowsの file:///C:/path の場合、/C:/path -> C:/path
		if len(target) >= 3 && target[0] == '/' && target[2] == ':' {
			target = target[1:]
		}
	} else if strings.HasPrefix(target, "file://") {
		target = strings.TrimPrefix(target, "file://")
	}

	// 全角円記号 (U+FFE5) および半角円記号 (U+00A5) をバックスラッシュ (\) に統一
	target = strings.ReplaceAll(target, "\u00A5", `\`)
	target = strings.ReplaceAll(target, "\uFFE5", `\`)

	return target
}

// AddAttachment はURLまたはローカルパスの添付を登録します
func (h *Handler) AddAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	itemID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効なアイテムIDです", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := CleanPathInput(r.FormValue("target"))
	if target == "" {
		http.Error(w, "URLまたはパスを入力してください", http.StatusBadRequest)
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

	// 対象アイテムが存在するか確認
	if _, err := db.GetItemByID(h.DB, itemID); err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
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

	// 保存先ディレクトリの存在を保証
	if err := os.MkdirAll(h.ImagesDir, 0755); err != nil {
		http.Error(w, "Failed to ensure images directory", http.StatusInternalServerError)
		return
	}

	// 拡張子判定（デフォルトは .png、主要な画像形式に対応）
	ext := filepath.Ext(header.Filename)
	validExts := map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".webp": true,
		".gif":  true,
	}
	if !validExts[strings.ToLower(ext)] {
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

// openFileFunc はOS別のファイルオープン処理関数（テスト時に差し替え可能）
var openFileFunc = defaultOpenFile

func defaultOpenFile(target string) error {
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

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		output, err := cmd.CombinedOutput()
		if err != nil {
			outStr := strings.TrimSpace(string(output))
			if outStr != "" {
				return fmt.Errorf("%s", outStr)
			}
			return err
		}
		return nil
	}

	return cmd.Start()
}

// OpenAttachment はローカルファイルパス（またはURL）をOS標準アプリケーションで起動します
func (h *Handler) OpenAttachment(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効な添付IDです", http.StatusBadRequest)
		return
	}

	att, err := db.GetAttachmentByID(h.DB, id)
	if err != nil {
		http.Error(w, "添付が見つかりません", http.StatusNotFound)
		return
	}

	target := CleanPathInput(att.Target)
	if target == "" {
		http.Error(w, "パスが指定されていません", http.StatusBadRequest)
		return
	}

	isURL := strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")

	// ローカルパスおよび共有パス(UNC)の事前チェック
	if !isURL {
		isUNC := strings.HasPrefix(target, `\\`) || strings.HasPrefix(target, "//")

		if runtime.GOOS != "windows" && isUNC {
			http.Error(w, fmt.Sprintf("Windows共有パス(UNC)です。macOS/Linuxでは smb:// 等でFinderにマウントして開いてください:\n%s", target), http.StatusBadRequest)
			return
		}

		// 事前存在チェック (os.Stat)
		if _, err := os.Stat(target); err != nil {
			if os.IsNotExist(err) {
				if isUNC {
					http.Error(w, fmt.Sprintf("共有パスが見つかりません。ネットワーク接続や共有名を確認してください:\n%s", target), http.StatusNotFound)
				} else {
					http.Error(w, fmt.Sprintf("ファイルまたはフォルダーが見つかりません:\n%s", target), http.StatusNotFound)
				}
				return
			} else if os.IsPermission(err) {
				http.Error(w, fmt.Sprintf("アクセス権限がありません:\n%s", target), http.StatusForbidden)
				return
			} else {
				http.Error(w, fmt.Sprintf("パスにアクセスできません (%v):\n%s", err, target), http.StatusBadRequest)
				return
			}
		}
	}

	// OS別の起動処理を実行
	if err := openFileFunc(target); err != nil {
		http.Error(w, fmt.Sprintf("開くことができませんでした: %v", err), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "attachmentOpened")
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

	// 削除後の最新の添付一覧を再描画して返す
	attachments, err := db.GetAttachmentsByItemID(h.DB, att.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		ItemID      int64
		Attachments []model.Attachment
	}{
		ItemID:      att.ItemID,
		Attachments: attachments,
	}

	h.Render(w, "attachments.html", data)
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

