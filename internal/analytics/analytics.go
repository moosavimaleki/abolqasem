package analytics

const Endpoint = "https://abolqasem.sh/api/t"

const (
	EventAppLaunch         = "app_launch"
	EventProjectOpened     = "project_opened"
	EventProjectCreated    = "project_created"
	EventProjectRemoved    = "project_removed"
	EventChatCreated       = "chat_created"
	EventChatDeleted       = "chat_deleted"
	EventMessageSent       = "message_sent"
	EventUpdateChecked     = "update_checked"
	EventUpdateInstalled   = "update_installed"
	EventUpdateFailed      = "update_failed"
	EventAnalyticsEnabled  = "analytics_enabled"
	EventAnalyticsDisabled = "analytics_disabled"
)

var StaticEventNames = []string{
	EventAppLaunch,
	EventProjectOpened,
	EventProjectCreated,
	EventProjectRemoved,
	EventChatCreated,
	EventChatDeleted,
	EventMessageSent,
	EventUpdateChecked,
	EventUpdateInstalled,
	EventUpdateFailed,
	EventAnalyticsEnabled,
	EventAnalyticsDisabled,
}

var StaticPropertyNames = []string{
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

type Reporter interface {
	Track(eventName string, properties map[string]any)
	TrackLaunch(options LaunchOptions)
}

type LaunchOptions struct {
	Port        int
	DefaultPort int
	Host        string
	OpenBrowser bool
	ShareQuick  bool
	ShareToken  bool
	Password    string
	StrictPort  bool
}

type NoopReporter struct{}

func (NoopReporter) Track(string, map[string]any) {}

func (reporter NoopReporter) TrackLaunch(options LaunchOptions) {
	reporter.Track(EventAppLaunch, LaunchProperties(options))
}

func LaunchProperties(options LaunchOptions) map[string]any {
	defaultPort := options.DefaultPort
	if defaultPort == 0 {
		defaultPort = 3210
	}
	return map[string]any{
		"custom_port_enabled": options.Port != 0 && options.Port != defaultPort,
		"no_open_enabled":     !options.OpenBrowser,
		"password_enabled":    options.Password != "",
		"strict_port_enabled": options.StrictPort,
		"remote_enabled":      options.Host == "0.0.0.0",
		"host_enabled":        options.Host != "" && options.Host != "0.0.0.0" && options.Host != "127.0.0.1" && options.Host != "localhost",
		"share_quick_enabled": options.ShareQuick,
		"share_token_enabled": options.ShareToken,
	}
}
