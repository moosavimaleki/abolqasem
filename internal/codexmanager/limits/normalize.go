package limits

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// Normalize converts the undocumented ChatGPT usage payload to the stable
// manager schema. Unknown fields are intentionally ignored so upstream schema
// additions cannot break settings or account rotation.
func Normalize(payload map[string]any, fetchedAt time.Time) Snapshot {
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	plan, _ := stringValue(payload["plan_type"])
	result := Snapshot{
		Plan:        plan,
		ReachedType: reachedType(payload["rate_limit_reached_type"]),
		FetchedAt:   fetchedAt.UTC(),
	}
	if limit, ok := normalizeLimit("codex", "", object(payload["rate_limit"]), object(payload["credits"]), plan, fetchedAt); ok {
		result.Limits = append(result.Limits, limit)
	}
	additional, _ := payload["additional_rate_limits"].([]any)
	for _, item := range additional {
		entry := object(item)
		name, ok := stringValue(entry["limit_name"])
		if !ok || name == "" {
			continue
		}
		id := normalizedID(name)
		if feature, ok := stringValue(entry["metered_feature"]); ok && feature != "" {
			id = normalizedID(feature)
		}
		if limit, ok := normalizeLimit(id, name, object(entry["rate_limit"]), nil, plan, fetchedAt); ok {
			result.Limits = append(result.Limits, limit)
		}
	}
	return result
}

func normalizeLimit(id, name string, detail, credits map[string]any, plan string, fetchedAt time.Time) (Limit, bool) {
	if detail == nil {
		return Limit{}, false
	}
	limit := Limit{ID: id, Name: name, Plan: plan}
	limit.Allowed, _ = boolValue(detail["allowed"])
	limit.LimitReached, _ = boolValue(detail["limit_reached"])
	for index, key := range []string{"primary_window", "secondary_window"} {
		window, ok := normalizeWindow(object(detail[key]), index, plan, fetchedAt)
		if ok {
			limit.Windows = append(limit.Windows, window)
		}
	}
	if credits != nil {
		limit.Credits = normalizeCredits(credits)
	}
	return limit, len(limit.Windows) > 0 || limit.Credits != nil
}

func normalizeWindow(value map[string]any, index int, plan string, fetchedAt time.Time) (Window, bool) {
	if value == nil {
		return Window{}, false
	}
	used, ok := number(value["used_percent"])
	if !ok {
		return Window{}, false
	}
	used = clamp(used, 0, 100)
	window := Window{UsedPercent: used, RemainingPercent: 100 - used, Reached: used >= 100}
	if seconds, ok := integer(value["limit_window_seconds"]); ok && seconds > 0 {
		minutes := seconds / 60
		window.WindowMinutes = &minutes
	}
	if seconds, ok := integer(value["reset_after_seconds"]); ok {
		window.ResetAfterSeconds = &seconds
	}
	if unix, ok := number(value["reset_at"]); ok {
		if unix > 10_000_000_000 {
			unix /= 1000
		}
		reset := time.Unix(int64(unix), 0).UTC()
		window.ResetAt = &reset
	} else if window.ResetAfterSeconds != nil {
		reset := fetchedAt.Add(time.Duration(*window.ResetAfterSeconds) * time.Second)
		window.ResetAt = &reset
	}
	window.Label = windowLabel(index, window.WindowMinutes, plan)
	return window, true
}

func normalizeCredits(value map[string]any) *Credits {
	credits := &Credits{}
	credits.HasCredits, _ = boolValue(value["has_credits"])
	credits.Unlimited, _ = boolValue(value["unlimited"])
	credits.Balance, _ = stringValue(value["balance"])
	return credits
}

func windowLabel(index int, minutes *int, plan string) string {
	if minutes == nil || *minutes <= 0 {
		return "window " + string(rune('1'+index))
	}
	if plan == "free" && *minutes >= 27*24*60 {
		return "monthly"
	}
	if *minutes >= 6*24*60 {
		return "weekly"
	}
	if *minutes%60 == 0 {
		return integerString(*minutes/60) + "h"
	}
	return "window " + string(rune('1'+index))
}

func normalizedID(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
}

func reachedType(value any) string {
	if kind, ok := stringValue(value); ok {
		return kind
	}
	if nested := object(value); nested != nil {
		kind, _ := stringValue(nested["type"])
		return kind
	}
	return ""
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func number(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func integer(value any) (int, bool) {
	number, ok := number(value)
	if !ok || math.Trunc(number) != number || number > math.MaxInt || number < math.MinInt {
		return 0, false
	}
	return int(number), true
}

func stringValue(value any) (string, bool) {
	result, ok := value.(string)
	return strings.TrimSpace(result), ok
}

func boolValue(value any) (bool, bool) {
	result, ok := value.(bool)
	return result, ok
}

func clamp(value, minimum, maximum float64) float64 {
	return math.Max(minimum, math.Min(maximum, value))
}

func integerString(value int) string {
	return strconv.Itoa(value)
}
