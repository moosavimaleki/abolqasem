package codexmanager_test

import (
	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/browser"
	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/recommendation"
	"abolqasem/internal/providers/custom"
	"testing"
)

func TestDomainModelsExposeStableWireValues(t *testing.T) {
	if account.PlanFree != "free" || account.StateNeedsLogin != "needs_login" {
		t.Fatal("account values changed")
	}
	if recommendation.Best != "best" || (limits.Window{}).Label != "" {
		t.Fatal("domain values changed")
	}
	if (browser.Profile{}).ID != "" || (history.Sample{}).Account != "" || (custom.Model{}).ID != "" {
		t.Fatal("zero values changed")
	}
}
