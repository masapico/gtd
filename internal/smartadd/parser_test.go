package smartadd

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	// 基準日: 2026-10-24 (土曜日)
	baseTime := time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)

	tests := []struct {
		name         string
		input        string
		wantTitle    string
		wantContexts []string
		wantStartDate *string
		wantDueDate  *string
	}{
		{
			name:         "Pure title without syntax",
			input:        "買い物に行く",
			wantTitle:    "買い物に行く",
			wantContexts: nil,
			wantStartDate: nil,
			wantDueDate:  nil,
		},
		{
			name:         "Title with context #PC",
			input:        "報告書作成 #PC",
			wantTitle:    "報告書作成",
			wantContexts: []string{"#PC"},
			wantStartDate: nil,
			wantDueDate:  nil,
		},
		{
			name:         "Title with multiple contexts and ^today",
			input:        "メール返信 #PC #5分 ^today",
			wantTitle:    "メール返信",
			wantContexts: []string{"#PC", "#5分"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-24"),
		},
		{
			name:         "Japanese keywords ^今日 and full-width #",
			input:        "急ぎの確認 ＃会議 ^今日",
			wantTitle:    "急ぎの確認",
			wantContexts: []string{"#会議"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-24"),
		},
		{
			name:         "Tomorrow ^tomorrow and ^明日",
			input:        "プレゼン準備 #仕事 ^tomorrow",
			wantTitle:    "プレゼン準備",
			wantContexts: []string{"#仕事"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-25"),
		},
		{
			name:         "Next weekday ^mon (土曜日から月曜 = 2日後: 2026-10-26)",
			input:        "定例会議の準備 ^mon #会議",
			wantTitle:    "定例会議の準備",
			wantContexts: []string{"#会議"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-26"),
		},
		{
			name:         "Japanese weekday ^金 (土曜日から次の金曜 = 6日後: 2026-10-30)",
			input:        "締め切り書類提出 ^金 #連絡",
			wantTitle:    "締め切り書類提出",
			wantContexts: []string{"#連絡"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-30"),
		},
		{
			name:         "Explicit date ^2026-11-15",
			input:        "プロジェクトマイルストーン ^2026-11-15 #重要",
			wantTitle:    "プロジェクトマイルストーン",
			wantContexts: []string{"#重要"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-11-15"),
		},
		{
			name:         "Slash date ^2026/12/31",
			input:        "年末調整提出 ^2026/12/31",
			wantTitle:    "年末調整提出",
			wantContexts: nil,
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-12-31"),
		},
		{
			name:         "Relative days ^+3d",
			input:        "フォローアップ連絡 ^+3d #電話",
			wantTitle:    "フォローアップ連絡",
			wantContexts: []string{"#電話"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-27"),
		},
		{
			name:         "Start date ~明日 and Due date ^金",
			input:        "企画立案 ~明日 ^金 #PC",
			wantTitle:    "企画立案",
			wantContexts: []string{"#PC"},
			wantStartDate: strPtr("2026-10-25"),
			wantDueDate:  strPtr("2026-10-30"),
		},
		{
			name:         "Full-width tilde 〜2026-11-01",
			input:        "契約更新 〜2026-11-01",
			wantTitle:    "契約更新",
			wantContexts: nil,
			wantStartDate: strPtr("2026-11-01"),
			wantDueDate:  nil,
		},
		{
			name:         "Ignored @ symbol for future expansion",
			input:        "連絡する @山田太郎 #連絡 ^today",
			wantTitle:    "連絡する @山田太郎",
			wantContexts: []string{"#連絡"},
			wantStartDate: nil,
			wantDueDate:  strPtr("2026-10-24"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input, baseTime)
			if got.CleanTitle != tt.wantTitle {
				t.Errorf("CleanTitle = %q, want %q", got.CleanTitle, tt.wantTitle)
			}
			if len(got.Contexts) != len(tt.wantContexts) {
				t.Fatalf("Contexts len = %d (%v), want %d (%v)", len(got.Contexts), got.Contexts, len(tt.wantContexts), tt.wantContexts)
			}
			for i := range got.Contexts {
				if got.Contexts[i] != tt.wantContexts[i] {
					t.Errorf("Contexts[%d] = %q, want %q", i, got.Contexts[i], tt.wantContexts[i])
				}
			}
			if (got.StartDate == nil && tt.wantStartDate != nil) || (got.StartDate != nil && tt.wantStartDate == nil) {
				t.Fatalf("StartDate = %v, want %v", got.StartDate, tt.wantStartDate)
			}
			if got.StartDate != nil && tt.wantStartDate != nil && *got.StartDate != *tt.wantStartDate {
				t.Errorf("StartDate = %q, want %q", *got.StartDate, *tt.wantStartDate)
			}
			if (got.DueDate == nil && tt.wantDueDate != nil) || (got.DueDate != nil && tt.wantDueDate == nil) {
				t.Fatalf("DueDate = %v, want %v", got.DueDate, tt.wantDueDate)
			}
			if got.DueDate != nil && tt.wantDueDate != nil && *got.DueDate != *tt.wantDueDate {
				t.Errorf("DueDate = %q, want %q", *got.DueDate, *tt.wantDueDate)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

