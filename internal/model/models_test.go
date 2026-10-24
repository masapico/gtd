package model_test

import (
	"testing"
	"time"

	"gtd/internal/model"
)

func TestCalculateDueDisplay(t *testing.T) {
	now := time.Date(2026, 10, 24, 14, 30, 0, 0, time.Local)

	tests := []struct {
		name      string
		target    time.Time
		wantLabel string
		wantClass string
	}{
		{
			name:      "3 days overdue",
			target:    time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local),
			wantLabel: "期限超過 (3日前)",
			wantClass: "due-overdue",
		},
		{
			name:      "yesterday (overdue)",
			target:    time.Date(2026, 10, 23, 23, 59, 59, 0, time.Local),
			wantLabel: "期限超過 (昨日)",
			wantClass: "due-overdue",
		},
		{
			name:      "today",
			target:    time.Date(2026, 10, 24, 8, 0, 0, 0, time.Local),
			wantLabel: "今日",
			wantClass: "due-today",
		},
		{
			name:      "tomorrow",
			target:    time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local),
			wantLabel: "明日",
			wantClass: "due-soon",
		},
		{
			name:      "day after tomorrow",
			target:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local),
			wantLabel: "明後日",
			wantClass: "due-soon",
		},
		{
			name:      "3 days later",
			target:    time.Date(2026, 10, 27, 0, 0, 0, 0, time.Local),
			wantLabel: "3日後",
			wantClass: "due-future",
		},
		{
			name:      "7 days later",
			target:    time.Date(2026, 10, 31, 0, 0, 0, 0, time.Local),
			wantLabel: "7日後",
			wantClass: "due-future",
		},
		{
			name:      "14 days later",
			target:    time.Date(2026, 11, 7, 0, 0, 0, 0, time.Local),
			wantLabel: "14日後",
			wantClass: "due-future",
		},
		{
			name:      "future in same year",
			target:    time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local),
			wantLabel: "11月15日",
			wantClass: "due-future",
		},
		{
			name:      "future in next year",
			target:    time.Date(2027, 1, 10, 0, 0, 0, 0, time.Local),
			wantLabel: "2027年1月10日",
			wantClass: "due-future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLabel := model.CalculateDueDisplay(tt.target, now)
			if gotLabel != tt.wantLabel {
				t.Errorf("CalculateDueDisplay() = %v, want %v", gotLabel, tt.wantLabel)
			}

			gotClass := model.CalculateDueDateClass(tt.target, now)
			if gotClass != tt.wantClass {
				t.Errorf("CalculateDueDateClass() = %v, want %v", gotClass, tt.wantClass)
			}
		})
	}
}

func TestItemDueDisplayAndTooltip(t *testing.T) {
	todayStr := time.Now().Format("2006-01-02")
	item := model.Item{
		DueDate: &todayStr,
	}

	if item.DueDisplay() != "今日" {
		t.Errorf("DueDisplay() = %v, want 今日", item.DueDisplay())
	}
	if item.DueDateClass() != "due-today" {
		t.Errorf("DueDateClass() = %v, want due-today", item.DueDateClass())
	}
	if item.DueTooltip() == "" {
		t.Errorf("DueTooltip() should not be empty")
	}

	// nil due date
	itemNil := model.Item{}
	if itemNil.DueDisplay() != "" {
		t.Errorf("DueDisplay() with nil = %v, want empty", itemNil.DueDisplay())
	}
	if itemNil.DueDateClass() != "" {
		t.Errorf("DueDateClass() with nil = %v, want empty", itemNil.DueDateClass())
	}
}

func TestCalculateStartDisplay(t *testing.T) {
	now := time.Date(2026, 10, 24, 14, 30, 0, 0, time.Local)

	tests := []struct {
		name      string
		target    time.Time
		wantLabel string
	}{
		{
			name:      "past",
			target:    time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local),
			wantLabel: "開始日経過",
		},
		{
			name:      "today",
			target:    time.Date(2026, 10, 24, 0, 0, 0, 0, time.Local),
			wantLabel: "今日から開始可能",
		},
		{
			name:      "tomorrow",
			target:    time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local),
			wantLabel: "明日から開始",
		},
		{
			name:      "day after tomorrow",
			target:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local),
			wantLabel: "明後日から開始",
		},
		{
			name:      "5 days later",
			target:    time.Date(2026, 10, 29, 0, 0, 0, 0, time.Local),
			wantLabel: "5日後から開始",
		},
		{
			name:      "same year future",
			target:    time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local),
			wantLabel: "11月20日から開始",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.CalculateStartDisplay(tt.target, now)
			if got != tt.wantLabel {
				t.Errorf("CalculateStartDisplay() = %v, want %v", got, tt.wantLabel)
			}
		})
	}
}

func TestItemIsDeferred(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	itemFuture := model.Item{StartDate: &tomorrow}
	if !itemFuture.IsDeferred() {
		t.Errorf("expected itemFuture to be deferred")
	}

	itemPast := model.Item{StartDate: &yesterday}
	if itemPast.IsDeferred() {
		t.Errorf("expected itemPast not to be deferred")
	}

	itemToday := model.Item{StartDate: &today}
	if itemToday.IsDeferred() {
		t.Errorf("expected itemToday not to be deferred")
	}
}

