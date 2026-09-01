package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const lastRefreshMaxAge = 72 * time.Hour

func Claims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(data, &claims) != nil {
		return nil
	}
	return claims
}

func AccessExpiry(raw map[string]any) *time.Time {
	tokens, _ := raw["tokens"].(map[string]any)
	access, _ := tokens["access_token"].(string)
	claims := Claims(access)
	if claims == nil {
		return nil
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil
	}
	value := time.Unix(int64(exp), 0).UTC()
	return &value
}

func Metadata(raw map[string]any) map[string]string {
	tokens, _ := raw["tokens"].(map[string]any)
	info := tokenClaims(tokens["id_token"])
	authNS, _ := info["https://api.openai.com/auth"].(map[string]any)
	profileNS, _ := info["https://api.openai.com/profile"].(map[string]any)
	return map[string]string{"email": firstString(info["email"], profileNS["email"], raw["email"]), "account_id": firstString(tokens["account_id"], info["chatgpt_account_id"], authNS["chatgpt_account_id"], raw["account_id"]), "plan": firstString(info["chatgpt_plan_type"], authNS["chatgpt_plan_type"], raw["plan_type"], raw["plan"])}
}

func Identity(raw map[string]any) map[string]string {
	tokens, _ := raw["tokens"].(map[string]any)
	info := tokenClaims(tokens["id_token"])
	meta := Metadata(raw)
	out := map[string]string{}
	for key, value := range map[string]string{"account_id": meta["account_id"], "email": meta["email"], "subject": firstString(info["sub"])} {
		if value != "" {
			out[key] = value
		}
	}
	return out
}

func SameIdentity(expected, candidate map[string]any) (bool, string) {
	a, b := Identity(expected), Identity(candidate)
	if len(a) == 0 || len(b) == 0 {
		return false, "could not verify account identity"
	}
	shared := 0
	for key, value := range a {
		if other, ok := b[key]; ok {
			shared++
			if value != other {
				return false, "identity mismatch on " + key
			}
		}
	}
	if shared == 0 {
		return false, "no shared identity fields to compare"
	}
	return true, "identity matched"
}

func ShouldRefresh(raw map[string]any, now time.Time) (bool, string) {
	exp := AccessExpiry(raw)
	if exp != nil {
		if exp.Sub(now) <= 12*time.Hour {
			return true, "access token expires soon"
		}
		return false, "access token is still valid"
	}
	lastRefresh, ok := LastRefresh(raw)
	if !ok {
		return true, "missing last_refresh and unreadable token expiry"
	}
	if now.Sub(lastRefresh) >= lastRefreshMaxAge {
		return true, "last refresh is older than 72 hours"
	}
	return false, "last refresh is still recent"
}

// LastRefresh reads the bookkeeping timestamp written by the OAuth refresher.
// It is deliberately tolerant of older auth files that predate this field.
func LastRefresh(raw map[string]any) (time.Time, bool) {
	text, _ := raw["last_refresh"].(string)
	value, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, false
	}
	return value.UTC(), true
}

// PreferCredential reports whether candidate is demonstrably safer than
// baseline for the same Codex identity. It deliberately returns false for a
// tie: callers can then keep the currently-live file, whose refresh token may
// have been rotated by a running Codex app-server.
func PreferCredential(candidate, baseline map[string]any, now time.Time) (bool, string) {
	if same, reason := SameIdentity(candidate, baseline); !same {
		return false, reason
	}
	candidateExpiry, baselineExpiry := AccessExpiry(candidate), AccessExpiry(baseline)
	candidateValid := candidateExpiry != nil && candidateExpiry.After(now)
	baselineValid := baselineExpiry != nil && baselineExpiry.After(now)
	if candidateValid != baselineValid {
		if candidateValid {
			return true, "candidate access token is still valid"
		}
		return false, "baseline access token is still valid"
	}
	if candidateValid && baselineValid && candidateExpiry.After(*baselineExpiry) {
		return true, "candidate access token expires later"
	}

	candidateRefreshAt, candidateHasRefreshAt := LastRefresh(candidate)
	baselineRefreshAt, baselineHasRefreshAt := LastRefresh(baseline)
	if candidateHasRefreshAt != baselineHasRefreshAt {
		if candidateHasRefreshAt {
			return true, "candidate was refreshed more recently"
		}
		return false, "baseline was refreshed more recently"
	}
	if candidateHasRefreshAt && candidateRefreshAt.After(baselineRefreshAt) {
		return true, "candidate was refreshed more recently"
	}
	if !candidateValid && !baselineValid && candidateExpiry != nil && baselineExpiry != nil && candidateExpiry.After(*baselineExpiry) {
		return true, "candidate access token expired later"
	}
	return false, "credentials have equal freshness"
}

// ShouldPromoteLive decides whether a live Codex auth file is a safer source
// of truth than the stored copy for the same identity. Codex may rotate a
// refresh token while it is running, so ties deliberately prefer the live
// copy. A stored credential that is demonstrably fresher wins instead.
func ShouldPromoteLive(stored, live map[string]any) (bool, string) {
	now := time.Now().UTC()
	if preferred, reason := PreferCredential(live, stored, now); preferred {
		return true, "live " + reason
	}
	if preferred, reason := PreferCredential(stored, live, now); preferred {
		return false, "stored " + reason
	}
	storedTokens, _ := stored["tokens"].(map[string]any)
	liveTokens, _ := live["tokens"].(map[string]any)
	storedRefresh, _ := storedTokens["refresh_token"].(string)
	liveRefresh, _ := liveTokens["refresh_token"].(string)
	if storedRefresh != "" && liveRefresh != "" && storedRefresh != liveRefresh {
		return true, "live auth has a newer refresh token"
	}
	return false, "live auth is not newer than stored account"
}

func tokenClaims(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	if token, ok := value.(string); ok {
		return Claims(token)
	}
	return map[string]any{}
}
func firstString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}
