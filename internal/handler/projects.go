package handler

import (
	"net/http"
	"strconv"
	"strings"

	"gtd/internal/db"
	"gtd/internal/model"
)

// CreateProject は新規プロジェクトを作成します
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}

	pType := r.FormValue("type")
	if pType != "sequential" {
		pType = "parallel"
	}

	p := &model.Project{
		Name:        name,
		Description: strings.TrimSpace(r.FormValue("description")),
		Status:      "active",
		Type:        pType,
	}

	_, err := db.CreateProject(h.DB, p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// UpdateProject はプロジェクトの名前や説明を更新します
func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}

	existing, err := db.GetProjectByID(h.DB, id)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = existing.Status
	}

	desc := existing.Description
	if r.Form.Has("description") {
		desc = strings.TrimSpace(r.FormValue("description"))
	}

	p := &model.Project{
		ID:          id,
		Name:        name,
		Description: desc,
		Status:      status,
	}

	if err := db.UpdateProject(h.DB, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// UpdateProjectStatus はプロジェクトのステータス（active, someday, done）を変更します
func (h *Handler) UpdateProjectStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	if status == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	if err := db.UpdateProjectStatus(h.DB, id, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// UpdateProjectType はプロジェクトの実行種別（parallel または sequential）を変更します
func (h *Handler) UpdateProjectType(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	projectType := r.FormValue("type")
	if projectType != "parallel" && projectType != "sequential" {
		http.Error(w, "Invalid project type (must be 'parallel' or 'sequential')", http.StatusBadRequest)
		return
	}

	if err := db.UpdateProjectType(h.DB, id, projectType); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

// DeleteProject はプロジェクトを削除します
func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if err := db.DeleteProject(h.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Trigger(w, "projectChanged", "itemChanged")
	w.WriteHeader(http.StatusOK)
}

