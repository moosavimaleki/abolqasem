package analytics

import (
	"reflect"
	"testing"
)

func TestStaticAnalyticsNamesMatchKanna(t *testing.T) {
	expectedEvents := []string{
		"app_launch",
		"project_opened",
		"project_created",
		"project_removed",
		"chat_created",
		"chat_deleted",
		"message_sent",
		"update_checked",
		"update_installed",
		"update_failed",
		"analytics_enabled",
		"analytics_disabled",
	}
	if !reflect.DeepEqual(StaticEventNames, expectedEvents) {
		t.Fatalf("unexpected analytics events: %#v", StaticEventNames)
	}

	expectedProperties := []string{
		"current_version",
		"environment",
		"latest_version",
		"custom_port_enabled",
		"no_open_enabled",
		"password_enabled",
		"strict_port_enabled",
		"remote_enabled",
		"host_enabled",
		"share_quick_enabled",
		"share_token_enabled",
	}
	if !reflect.DeepEqual(StaticPropertyNames, expectedProperties) {
		t.Fatalf("unexpected analytics properties: %#v", StaticPropertyNames)
	}
}

func TestLaunchPropertiesMatchKanna(t *testing.T) {
	properties := LaunchProperties(LaunchOptions{
		Port:        4000,
		DefaultPort: 3210,
		Host:        "0.0.0.0",
		OpenBrowser: false,
		ShareQuick:  true,
		Password:    "secret",
		StrictPort:  true,
	})

	expected := map[string]any{
		"custom_port_enabled": true,
		"no_open_enabled":     true,
		"password_enabled":    true,
		"strict_port_enabled": true,
		"remote_enabled":      true,
		"host_enabled":        false,
		"share_quick_enabled": true,
		"share_token_enabled": false,
	}
	if !reflect.DeepEqual(properties, expected) {
		t.Fatalf("unexpected launch properties: %#v", properties)
	}
}
