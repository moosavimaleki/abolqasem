package server

import (
	"ai-agent-manager/internal/analytics"
	"reflect"
	"testing"
)

func TestTrackWorkspaceAnalyticsToggleUsesKannaEventNames(t *testing.T) {
	reporter := &recordingAnalyticsReporter{}
	previous := workspaceAnalyticsReporter
	workspaceAnalyticsReporter = reporter
	t.Cleanup(func() { workspaceAnalyticsReporter = previous })

	trackWorkspaceAnalyticsToggle(true, false)
	trackWorkspaceAnalyticsToggle(false, true)
	trackWorkspaceAnalyticsToggle(true, true)

	expected := []string{
		analytics.EventAnalyticsDisabled,
		analytics.EventAnalyticsEnabled,
	}
	if !reflect.DeepEqual(reporter.events, expected) {
		t.Fatalf("unexpected analytics events: %#v", reporter.events)
	}
}

type recordingAnalyticsReporter struct {
	events []string
}

func (reporter *recordingAnalyticsReporter) Track(eventName string, _ map[string]any) {
	reporter.events = append(reporter.events, eventName)
}

func (reporter *recordingAnalyticsReporter) TrackLaunch(options analytics.LaunchOptions) {
	reporter.Track(analytics.EventAppLaunch, analytics.LaunchProperties(options))
}
