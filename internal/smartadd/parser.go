package smartadd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParsedTask はスマート記法（Smart Add）によって解析されたタスク情報です
type ParsedTask struct {
	RawTitle   string
	CleanTitle string
	Contexts   []string // 例: ["#PC", "#会議"]
	StartDate  *string  // YYYY-MM-DD (開始日 ~)
	DueDate    *string  // YYYY-MM-DD (期日 ^)
}

var (
	// 全角記号の正規化用
	fullWidthReplacer = strings.NewReplacer(
		"＃", "#",
		"＾", "^",
		"〜", "~",
		"～", "~", // 波ダッシュ
		"　", " ", // 全角スペースを半角スペースへ
	)

	// 日付フォーマットパターン
	isoDateRegex   = regexp.MustCompile(`^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$`)
	shortDateRegex = regexp.MustCompile(`^(\d{1,2})[-/](\d{1,2})$`)
	plusDaysRegex  = regexp.MustCompile(`^\+(\d+)([dw])$`)
)

// Parse は入力文字列から期日（^）、開始日（~）、コンテキスト（#）を抽出し、クリーンなタイトルと属性を返します
func Parse(input string, now time.Time) ParsedTask {
	normalized := fullWidthReplacer.Replace(input)
	tokens := strings.Fields(normalized)

	var titleTokens []string
	var contexts []string
	var startDate *string
	var dueDate *string

	for _, token := range tokens {
		// 1. 期日判定 (^)
		if strings.HasPrefix(token, "^") && len(token) > 1 {
			if d, ok := parseDateToken(strings.TrimPrefix(token, "^"), now); ok {
				dueDate = d
				continue
			}
		}

		// 2. 開始日判定 (~)
		if strings.HasPrefix(token, "~") && len(token) > 1 {
			if d, ok := parseDateToken(strings.TrimPrefix(token, "~"), now); ok {
				startDate = d
				continue
			}
		}

		// 3. コンテキスト判定 (#)
		if strings.HasPrefix(token, "#") && len(token) > 1 {
			ctxName := token
			if !strings.HasPrefix(ctxName, "#") {
				ctxName = "#" + ctxName
			}
			exists := false
			for _, c := range contexts {
				if strings.EqualFold(c, ctxName) {
					exists = true
					break
				}
			}
			if !exists {
				contexts = append(contexts, ctxName)
			}
			continue
		}

		// それ以外はタイトルトークン
		titleTokens = append(titleTokens, token)
	}

	cleanTitle := strings.Join(titleTokens, " ")
	if cleanTitle == "" && len(tokens) > 0 {
		cleanTitle = input
	}

	return ParsedTask{
		RawTitle:   input,
		CleanTitle: cleanTitle,
		Contexts:   contexts,
		StartDate:  startDate,
		DueDate:    dueDate,
	}
}

// parseDateToken はプレフィックスを除去したトークン文字列を日付（YYYY-MM-DD）に変換します
func parseDateToken(tokenWithoutPrefix string, now time.Time) (*string, bool) {
	tokenLower := strings.ToLower(tokenWithoutPrefix)

	weekdayMap := map[string]time.Weekday{
		"mon": time.Monday, "月": time.Monday,
		"tue": time.Tuesday, "火": time.Tuesday,
		"wed": time.Wednesday, "水": time.Wednesday,
		"thu": time.Thursday, "木": time.Thursday,
		"fri": time.Friday, "金": time.Friday,
		"sat": time.Saturday, "土": time.Saturday,
		"sun": time.Sunday, "日": time.Sunday,
	}

	switch tokenLower {
	case "today", "今日":
		d := now.Format("2006-01-02")
		return &d, true
	case "tomorrow", "明日":
		d := now.AddDate(0, 0, 1).Format("2006-01-02")
		return &d, true
	default:
		// 曜日判定 (e.g. fri, 金)
		if wd, ok := weekdayMap[tokenLower]; ok {
			d := nextWeekday(now, wd).Format("2006-01-02")
			return &d, true
		}
		// YYYY-MM-DD or YYYY/MM/DD
		if m := isoDateRegex.FindStringSubmatch(tokenWithoutPrefix); len(m) == 4 {
			y, _ := strconv.Atoi(m[1])
			mon, _ := strconv.Atoi(m[2])
			day, _ := strconv.Atoi(m[3])
			d := fmt.Sprintf("%04d-%02d-%02d", y, mon, day)
			return &d, true
		}
		// MM-DD or MM/DD (今年、過ぎていれば来年)
		if m := shortDateRegex.FindStringSubmatch(tokenWithoutPrefix); len(m) == 3 {
			mon, _ := strconv.Atoi(m[1])
			day, _ := strconv.Atoi(m[2])
			year := now.Year()
			targetDate := time.Date(year, time.Month(mon), day, 0, 0, 0, 0, now.Location())
			if targetDate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
				targetDate = time.Date(year+1, time.Month(mon), day, 0, 0, 0, 0, now.Location())
			}
			d := targetDate.Format("2006-01-02")
			return &d, true
		}
		// +1d, +2w
		if m := plusDaysRegex.FindStringSubmatch(tokenLower); len(m) == 3 {
			num, _ := strconv.Atoi(m[1])
			unit := m[2]
			var targetDate time.Time
			if unit == "d" {
				targetDate = now.AddDate(0, 0, num)
			} else {
				targetDate = now.AddDate(0, 0, num*7)
			}
			d := targetDate.Format("2006-01-02")
			return &d, true
		}
	}

	return nil, false
}

// nextWeekday は次回指定曜日の日付を算出します（当日と同じ曜日の場合は7日後）
func nextWeekday(now time.Time, target time.Weekday) time.Time {
	daysAhead := int(target - now.Weekday())
	if daysAhead <= 0 {
		daysAhead += 7
	}
	return now.AddDate(0, 0, daysAhead)
}

