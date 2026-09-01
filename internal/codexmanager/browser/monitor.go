package browser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"abolqasem/internal/codexmanager/account"
)

type CleanupPolicy struct {
	Enabled        bool
	DryRun         bool
	MaxCodexByPlan map[account.Plan]int
	DisabledFor    map[string]bool
}

type CleanupResult struct {
	Account                string   `json:"account"`
	Checked                int      `json:"checked"`
	CodexSessions          int      `json:"codexSessions"`
	CurrentDeviceProtected bool     `json:"currentDeviceProtected"`
	Targets                []Device `json:"targets"`
	Revoked                []string `json:"revoked"`
	Error                  string   `json:"error,omitempty"`
}

type DeviceClient interface {
	Devices(context.Context) ([]Device, error)
	Revoke(context.Context, Device) error
}

// Cleanup never runs unless explicitly enabled. Its deterministic ordering
// keeps the current device and most recently listed sessions first; callers can
// show Targets in UI before performing a non-dry run.
func Cleanup(ctx context.Context, managed account.Account, client DeviceClient, policy CleanupPolicy) CleanupResult {
	result := CleanupResult{Account: managed.Name}
	if !policy.Enabled {
		return result
	}
	if policy.MaxCodexByPlan[managed.Plan] <= 0 {
		return result
	}
	devices, err := client.Devices(ctx)
	if err != nil {
		result.Error = safeMonitorError(err)
		return result
	}
	result.Checked = len(devices)
	codexSessions := make([]Device, 0)
	for _, device := range devices {
		if device.HasCodex {
			codexSessions = append(codexSessions, device)
		}
	}
	result.CodexSessions = len(codexSessions)
	current := make([]Device, 0, 1)
	candidates := make([]Device, 0, len(codexSessions))
	for _, device := range codexSessions {
		if device.Current {
			current = append(current, device)
		} else {
			candidates = append(candidates, device)
		}
	}
	result.CurrentDeviceProtected = len(current) > 0
	if len(current) > 0 {
		// The Chrome profile's current Codex session is our safe keep target.
		// Every other Codex session is excess, regardless of plan type.
		result.Targets = append(result.Targets, candidates...)
	} else if len(candidates) > 1 {
		// Match the former manager's policy: prefer revoking Windows Codex
		// sessions first, then retain the oldest remaining non-Windows session.
		sort.SliceStable(candidates, func(i, j int) bool {
			leftWindows := strings.EqualFold(candidates[i].Platform, "windows")
			rightWindows := strings.EqualFold(candidates[j].Platform, "windows")
			if leftWindows != rightWindows {
				return leftWindows
			}
			return candidates[i].LastSeen < candidates[j].LastSeen
		})
		keep := 0
		for index, device := range candidates {
			if !strings.EqualFold(device.Platform, "windows") {
				keep = index
				break
			}
		}
		for index, device := range candidates {
			if index != keep {
				result.Targets = append(result.Targets, device)
			}
		}
	}
	if len(result.Targets) == 0 {
		return result
	}
	if policy.DryRun || policy.DisabledFor[managed.Name] {
		return result
	}
	for _, target := range result.Targets {
		if err := client.Revoke(ctx, target); err != nil {
			result.Error = safeMonitorError(err)
			return result
		}
		result.Revoked = append(result.Revoked, target.ID)
	}
	return result
}

func safeMonitorError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		return fmt.Sprintf("%s…", message[:239])
	}
	return message
}
