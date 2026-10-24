package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gtd/internal/db"
	"gtd/internal/model"
)

// DetectLauncherType はパスやURLから種別 ('app', 'folder', 'file', 'url') を自動推測します
func DetectLauncherType(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return "url"
	}
	if strings.HasSuffix(target, "/") || strings.HasSuffix(target, `\`) {
		return "folder"
	}
	// 実際にローカルに存在しディレクトリである場合
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		return "folder"
	}
	lower := strings.ToLower(target)
	if strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".app") {
		return "app"
	}
	ext := filepath.Ext(target)
	if ext != "" {
		return "file"
	}
	return "app"
}

// ServeLauncherModal はランチャー一覧と登録フォームを含むモーダルを描画します
func (h *Handler) ServeLauncherModal(w http.ResponseWriter, r *http.Request) {
	launchers, err := db.GetAllLaunchers(h.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Launchers    []model.Launcher
		EditLauncher *model.Launcher
	}{
		Launchers:    launchers,
		EditLauncher: nil,
	}

	h.Render(w, "launchers_modal.html", data)
}

// CreateLauncher は新しいランチャーを作成します
func (h *Handler) CreateLauncher(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "ランチャー名を入力してください", http.StatusBadRequest)
		return
	}

	target := CleanPathInput(r.FormValue("target"))
	if target == "" {
		http.Error(w, "パスまたはURLを入力してください", http.StatusBadRequest)
		return
	}

	lType := strings.TrimSpace(r.FormValue("type"))
	if lType == "" {
		lType = DetectLauncherType(target)
	}

	desc := strings.TrimSpace(r.FormValue("description"))

	launcher := &model.Launcher{
		Name:        name,
		Target:      target,
		Type:        lType,
		Description: desc,
	}

	if _, err := db.CreateLauncher(h.DB, launcher); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.TriggerToast(w, fmt.Sprintf("「%s」を登録しました", launcher.Name))
	h.ServeLauncherModal(w, r)
}

// GetLauncherEditForm は編集対象のランチャー情報を含むモーダル（または行）を描画します
func (h *Handler) GetLauncherEditForm(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効なIDです", http.StatusBadRequest)
		return
	}

	editLauncher, err := db.GetLauncherByID(h.DB, id)
	if err != nil {
		http.Error(w, "ランチャーが見つかりません", http.StatusNotFound)
		return
	}

	launchers, err := db.GetAllLaunchers(h.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Launchers    []model.Launcher
		EditLauncher *model.Launcher
	}{
		Launchers:    launchers,
		EditLauncher: editLauncher,
	}

	h.Render(w, "launchers_modal.html", data)
}

// UpdateLauncher は既存のランチャーを更新します
func (h *Handler) UpdateLauncher(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効なIDです", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "ランチャー名を入力してください", http.StatusBadRequest)
		return
	}

	target := CleanPathInput(r.FormValue("target"))
	if target == "" {
		http.Error(w, "パスまたはURLを入力してください", http.StatusBadRequest)
		return
	}

	lType := strings.TrimSpace(r.FormValue("type"))
	if lType == "" {
		lType = DetectLauncherType(target)
	}

	desc := strings.TrimSpace(r.FormValue("description"))

	launcher := &model.Launcher{
		ID:          id,
		Name:        name,
		Target:      target,
		Type:        lType,
		Description: desc,
	}

	if err := db.UpdateLauncher(h.DB, launcher); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.TriggerToast(w, fmt.Sprintf("「%s」を更新しました", launcher.Name))
	h.ServeLauncherModal(w, r)
}

// DeleteLauncher は指定されたIDのランチャーを削除します
func (h *Handler) DeleteLauncher(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効なIDです", http.StatusBadRequest)
		return
	}

	launcher, err := db.GetLauncherByID(h.DB, id)
	if err != nil {
		http.Error(w, "ランチャーが見つかりません", http.StatusNotFound)
		return
	}

	if err := db.DeleteLauncher(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.TriggerToast(w, fmt.Sprintf("「%s」を削除しました", launcher.Name))
	h.ServeLauncherModal(w, r)
}

// LaunchApp はランチャーに設定されたアプリケーション・ファイル・URLを起動します
func (h *Handler) LaunchApp(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "無効なランチャーIDです", http.StatusBadRequest)
		return
	}

	launcher, err := db.GetLauncherByID(h.DB, id)
	if err != nil {
		http.Error(w, "ランチャーが見つかりません", http.StatusNotFound)
		return
	}

	target := CleanPathInput(launcher.Target)
	if target == "" {
		http.Error(w, "起動パスが設定されていません", http.StatusBadRequest)
		return
	}

	isURL := strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")

	// ローカルパスおよび共有パス(UNC)の事前チェック (Windows / Mac / Linux)
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
					http.Error(w, fmt.Sprintf("ファイルまたはアプリケーションが見つかりません:\n%s", target), http.StatusNotFound)
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

	// OS別の起動処理を実行 (Windows: cmd /c start, macOS: open, Linux: xdg-open)
	if err := openFileFunc(target); err != nil {
		http.Error(w, fmt.Sprintf("開くことができませんでした: %v", err), http.StatusInternalServerError)
		return
	}

	h.TriggerToast(w, fmt.Sprintf("「%s」を起動しました", launcher.Name))
	w.WriteHeader(http.StatusOK)
}

