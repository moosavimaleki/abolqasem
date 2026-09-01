package browser

import (
	"context"
	"errors"
	"testing"

	"abolqasem/internal/codexmanager/account"
)

type monitorClient struct {
	devices []Device
	revoked []string
	err     error
}

func (c *monitorClient) Devices(context.Context) ([]Device, error) { return c.devices, c.err }
func (c *monitorClient) Revoke(_ context.Context, device Device) error {
	if device.Current {
		return errors.New("unsafe current device")
	}
	c.revoked = append(c.revoked, device.ID)
	return nil
}

func TestCleanupRequiresOptInAndSupportsDryRun(t *testing.T) {
	client := &monitorClient{devices: []Device{{ID: "current", Current: true, HasCodex: true}, {ID: "old-a", HasCodex: true}, {ID: "old-b", HasCodex: true}}}
	managed := account.Account{Name: "plus", Plan: account.PlanPlus}
	policy := CleanupPolicy{Enabled: true, DryRun: true, MaxCodexByPlan: map[account.Plan]int{account.PlanPlus: 1}}
	result := Cleanup(context.Background(), managed, client, policy)
	if len(result.Targets) != 2 || result.Targets[0].ID != "old-a" || len(client.revoked) != 0 {
		t.Fatalf("result=%#v revoked=%v", result, client.revoked)
	}
	policy.DryRun = false
	result = Cleanup(context.Background(), managed, client, policy)
	if len(result.Revoked) != 2 || result.Revoked[0] != "old-a" || result.Revoked[1] != "old-b" {
		t.Fatalf("result=%#v", result)
	}
	if result := Cleanup(context.Background(), managed, client, CleanupPolicy{}); len(result.Targets) != 0 || len(client.revoked) != 2 {
		t.Fatalf("unexpected cleanup without opt-in: %#v", result)
	}
}
