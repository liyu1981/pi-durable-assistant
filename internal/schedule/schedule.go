// Package schedule holds pure validation and time arithmetic for scheduled jobs.
package schedule

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Schedule is a normalized schedule request.
type Schedule struct {
	Text         string  `json:"text"`
	At           int64   `json:"at"`
	EveryMinutes float64 `json:"everyMinutes"`
}

// Normalize validates a schedule request and computes its first fire time (epoch milliseconds).
func Normalize(input map[string]any, now int64) (Schedule, error) {
	text, _ := input["text"].(string)
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 1000 {
		return Schedule{}, fmt.Errorf("任务内容必须是 1～1000 字")
	}
	delay, err := number(input["minutesFromNow"])
	if err != nil || delay < 0.25 || delay > 525600 {
		return Schedule{}, fmt.Errorf("首次执行时间必须在 0.25～525600 分钟内")
	}
	repeat, err := number(input["everyMinutes"])
	if err != nil || repeat < 0 || (repeat > 0 && (repeat < 1 || repeat > 525600)) {
		return Schedule{}, fmt.Errorf("重复间隔须为 0 或 1～525600 分钟")
	}
	return Schedule{Text: text, At: now + int64(math.Round(delay*60000)), EveryMinutes: repeat}, nil
}

func number(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case jsonNumber:
		return v.Float64()
	default:
		return 0, fmt.Errorf("not a number")
	}
}

type jsonNumber interface{ Float64() (float64, error) }

// NextOccurrence advances a periodic schedule past now. It skips missed periods instead of building a backlog.
func NextOccurrence(previousAt int64, everyMinutes float64, now int64) *int64 {
	if everyMinutes <= 0 || math.IsNaN(everyMinutes) || math.IsInf(everyMinutes, 0) {
		return nil
	}
	interval := int64(math.Round(everyMinutes * 60000))
	if interval <= 0 {
		return nil
	}
	steps := (now-previousAt)/interval + 1
	if steps < 1 {
		steps = 1
	}
	next := previousAt + steps*interval
	return &next
}

// Matches reports whether a due task still represents the current schedule row.
func Matches(itemStatus string, itemNextAt int64, itemRevision int, taskAt int64, taskRevision int) bool {
	return itemStatus == "active" && itemNextAt == taskAt && itemRevision == taskRevision
}

// NormalizeRequestId validates an idempotency key and returns nil for an absent one.
func NormalizeRequestId(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("requestId 必须是不超过 100 字的字符串")
	}
	if s == "" {
		return nil, nil
	}
	if len([]rune(s)) > 100 {
		return nil, fmt.Errorf("requestId 必须是不超过 100 字的字符串")
	}
	if s == "__proto__" || s == "prototype" || s == "constructor" {
		return nil, fmt.Errorf("requestId 使用了保留名称")
	}
	return &s, nil
}

// NowMillis returns the current epoch-millisecond clock.
func NowMillis() int64 { return time.Now().UnixMilli() }
