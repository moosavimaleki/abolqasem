package recommendation

import (
	"testing"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/limits"
)

func pointer(value int) *int { return &value }

func TestSelectPrefersPaidAndUsesStableTieBreak(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	reset := now.Add(24 * time.Hour)
	candidates := []Candidate{
		{Name: "z-free", Plan: account.PlanFree, State: account.StateReady, Limits: limits.Snapshot{FetchedAt: now, Limits: []limits.Limit{{ID: "codex", Windows: []limits.Window{{Label: "weekly", RemainingPercent: 90, WindowMinutes: pointer(10080), ResetAt: &reset}}}}}},
		{Name: "alpha-plus", Plan: account.PlanPlus, State: account.StateReady, Limits: limits.Snapshot{FetchedAt: now, Limits: []limits.Limit{{ID: "codex", Windows: []limits.Window{{Label: "weekly", RemainingPercent: 90, WindowMinutes: pointer(10080), ResetAt: &reset}}}}}},
	}
	selection := Select(candidates, now)
	if selection.Best == nil || selection.Best.Account != "alpha-plus" || !selection.Best.Recommendable {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectProtectsExhaustedAccount(t *testing.T) {
	now := time.Now().UTC()
	selection := Select([]Candidate{{Name: "limited", Plan: account.PlanPlus, State: account.StateReady, Limits: limits.Snapshot{FetchedAt: now, Limits: []limits.Limit{{ID: "codex", Windows: []limits.Window{{Label: "weekly", RemainingPercent: 0}}}}}}}, now)
	if selection.Best != nil || selection.Results["limited"].Label != Save || selection.Results["limited"].Recommendable || selection.Results["limited"].Reason != "weekly limit reached" {
		t.Fatalf("selection = %#v", selection)
	}
}
