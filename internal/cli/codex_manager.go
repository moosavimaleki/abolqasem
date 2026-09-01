package cli

import (
	"abolqasem/internal/platform"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// cmCmd exposes the Codex-account capabilities that live inside Abolqasem.
// It intentionally excludes the former codex-manager service/gateway commands:
// Abolqasem owns that runtime and its settings through the local server.
var (
	cmJSON bool
	cmCmd  = &cobra.Command{
		Use:     "cm",
		Aliases: []string{"codex-manager"},
		Short:   "Manage Codex accounts, limits, and Chrome sessions",
		Long: "Manage the Codex accounts stored by Abolqasem.\n\n" +
			"Commands operate on the local Abolqasem service so account switching keeps active turns running and only refreshes idle Codex app-servers.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmList(cmd)
		},
	}
	cmListCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List managed Codex accounts and their last known limits",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmList(cmd)
		},
	}
	cmStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show Codex Manager diagnostics and managed account count",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var snapshot cmSnapshot
			if err := cmRequest(cmd.Context(), http.MethodGet, "", nil, &snapshot); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, snapshot)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Accounts: %d\n", len(snapshot.Accounts))
			fmt.Fprintf(cmd.OutOrStdout(), "Automatic selection: %t (%s)\n", snapshot.Enabled, snapshot.AutoSwitchPolicy)
			if snapshot.Diagnostics.SessionMonitor.LastError != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Chrome monitor: %s\n", snapshot.Diagnostics.SessionMonitor.LastError)
			} else if snapshot.Diagnostics.SessionMonitor.LastRun != "" && snapshot.Diagnostics.SessionMonitor.LastRun != "0001-01-01T00:00:00Z" {
				fmt.Fprintf(cmd.OutOrStdout(), "Chrome monitor: checked %s\n", snapshot.Diagnostics.SessionMonitor.LastRun)
			}
			return nil
		},
	}
	cmCheckForce bool
	cmCheckCmd   = &cobra.Command{
		Use:   "check [account]",
		Short: "Refresh quota and sign-in status for all accounts or one account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "check"
			if len(args) == 1 {
				path = "accounts/" + url.PathEscape(args[0]) + "/check"
			}
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, path, map[string]bool{"forceRefresh": cmCheckForce}, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Quota check completed.")
			return cmList(cmd)
		},
	}
	cmUseCmd = &cobra.Command{
		Use:     "use <account>",
		Aliases: []string{"activate"},
		Short:   "Make an account active for new Codex sessions",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "accounts/"+url.PathEscape(args[0])+"/activate", nil, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Active Codex account: %s\n", args[0])
			return nil
		},
	}
	cmBestCmd = &cobra.Command{
		Use:   "best",
		Short: "Activate the recommended account with available quota",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var response struct {
				Account string `json:"account"`
			}
			if err := cmRequest(cmd.Context(), http.MethodPost, "recommendation", nil, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Active Codex account: %s\n", response.Account)
			return nil
		},
	}
	cmAddActivate bool
	cmAddCmd      = &cobra.Command{
		Use:   "add <name> <auth.json>",
		Short: "Import a Codex auth.json as a managed account",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			contents, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("read auth.json: %w", err)
			}
			var credentials map[string]any
			if err := json.Unmarshal(contents, &credentials); err != nil {
				return fmt.Errorf("parse auth.json: %w", err)
			}
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "accounts", map[string]any{"name": args[0], "credentials": credentials, "activate": cmAddActivate}, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added Codex account: %s\n", args[0])
			return nil
		},
	}
	cmLoginReplace bool
	cmLoginOpen    bool
	cmLoginEmail   string
	cmLoginCmd     = &cobra.Command{
		Use:   "login <name>",
		Short: "Add or refresh an account through Codex device login",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var started cmLoginStart
			if err := cmRequest(cmd.Context(), http.MethodPost, "login", map[string]any{"name": args[0], "replace": cmLoginReplace, "expectedEmail": cmLoginEmail}, &started); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Open %s and enter code %s\n", started.VerificationURL, started.UserCode)
			if cmLoginOpen {
				if err := platform.OpenBrowser(started.VerificationURL); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Could not open browser: %v\n", err)
				}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					_, _ = cmRequestRaw(context.Background(), http.MethodDelete, "login/"+url.PathEscape(started.LoginID), nil)
					return fmt.Errorf("device login timed out or was cancelled")
				case <-time.After(2 * time.Second):
				}
				var status cmLoginStatus
				if err := cmRequest(ctx, http.MethodGet, "login/"+url.PathEscape(started.LoginID), nil, &status); err != nil {
					return err
				}
				switch status.Status {
				case "completed":
					if cmJSON {
						return cmPrintJSON(cmd, status)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Codex account %s is ready.\n", args[0])
					return nil
				case "failed":
					return fmt.Errorf("device login failed: %s", status.Error)
				}
			}
		},
	}
	cmRenameCmd = &cobra.Command{
		Use:   "rename <account> <new-name>",
		Short: "Rename a managed account",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "accounts/"+url.PathEscape(args[0])+"/rename", map[string]string{"name": args[1]}, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed %s to %s\n", args[0], args[1])
			return nil
		},
	}
	cmRemoveCmd = &cobra.Command{
		Use:     "remove <account>",
		Aliases: []string{"rm"},
		Short:   "Remove an inactive managed account",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var response any
			if err := cmRequest(cmd.Context(), http.MethodDelete, "accounts/"+url.PathEscape(args[0])+"/delete", nil, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed Codex account: %s\n", args[0])
			return nil
		},
	}
	cmMonitorEnable  bool
	cmMonitorDisable bool
	cmMonitorCmd     = &cobra.Command{
		Use:   "monitor <account>",
		Short: "Enable or disable automatic extra-session cleanup for one account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmMonitorEnable && !cmMonitorDisable {
				return fmt.Errorf("choose --enable or --disable")
			}
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "accounts/"+url.PathEscape(args[0])+"/session-monitor", map[string]bool{"disabled": cmMonitorDisable}, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			state := "enabled"
			if cmMonitorDisable {
				state = "disabled"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Automatic cleanup for %s: %s\n", args[0], state)
			return nil
		},
	}
	cmHistoryRange string
	cmHistoryLimit int
	cmHistoryCmd   = &cobra.Command{
		Use:   "history <account>",
		Short: "Show saved quota history for one account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := url.Values{"account": {args[0]}}
			if cmHistoryRange != "" {
				query.Set("range", cmHistoryRange)
			}
			if cmHistoryLimit > 0 {
				query.Set("limit", fmt.Sprint(cmHistoryLimit))
			}
			var response any
			if err := cmRequest(cmd.Context(), http.MethodGet, "history?"+query.Encode(), nil, &response); err != nil {
				return err
			}
			return cmPrintJSON(cmd, response)
		},
	}
	cmChromeCmd = &cobra.Command{
		Use:   "chrome",
		Short: "Inspect Chrome profiles and their Codex sessions",
	}
	cmChromeScanCmd = &cobra.Command{
		Use:   "scan",
		Short: "Classify Chrome profiles as signed in, partial, signed out, or failed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var response cmChromeProfiles
			if err := cmRequest(cmd.Context(), http.MethodPost, "browser/scan", nil, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			for _, profile := range response.Profiles {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", profile.Outcome, profile.Name, profile.ActiveEmail, strings.Join(profile.SavedAccounts, ","))
			}
			return nil
		},
	}
	cmChromeSessionsCmd = &cobra.Command{
		Use:   "sessions <profile-id>",
		Short: "List ChatGPT devices and Codex sessions for a Chrome profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var response any
			if err := cmRequest(cmd.Context(), http.MethodGet, "browser/devices?profileId="+url.QueryEscape(args[0]), nil, &response); err != nil {
				return err
			}
			return cmPrintJSON(cmd, response)
		},
	}
	cmChromeOpenCmd = &cobra.Command{
		Use:   "open <profile-id>",
		Short: "Open a discovered local Chrome profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "browser/profiles/open?profileId="+url.QueryEscape(args[0]), nil, &response); err != nil {
				return err
			}
			if cmJSON {
				return cmPrintJSON(cmd, response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Opened Chrome profile: %s\n", args[0])
			return nil
		},
	}
	cmChromeCleanupAccount string
	cmChromeCleanupApply   bool
	cmChromeCleanupCmd     = &cobra.Command{
		Use:   "cleanup <profile-id>",
		Short: "Preview or revoke extra Codex sessions for a Chrome profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(cmChromeCleanupAccount) == "" {
				return fmt.Errorf("--account is required")
			}
			var response any
			if err := cmRequest(cmd.Context(), http.MethodPost, "browser/cleanup?profileId="+url.QueryEscape(args[0]), map[string]any{"account": cmChromeCleanupAccount, "dryRun": !cmChromeCleanupApply}, &response); err != nil {
				return err
			}
			return cmPrintJSON(cmd, response)
		},
	}
)

type cmQuotaWindow struct {
	Label            string  `json:"label"`
	RemainingPercent float64 `json:"remainingPercent"`
}

type cmAccount struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Plan       string `json:"plan"`
	State      string `json:"state"`
	Active     bool   `json:"active"`
	RateLimits struct {
		Limits []struct {
			Windows []cmQuotaWindow `json:"windows"`
		} `json:"limits"`
	} `json:"rateLimits"`
}

type cmSnapshot struct {
	Enabled          bool        `json:"enabled"`
	AutoSwitchPolicy string      `json:"autoSwitchPolicy"`
	Accounts         []cmAccount `json:"accounts"`
	Diagnostics      struct {
		SessionMonitor struct {
			LastRun   string `json:"lastRun"`
			LastError string `json:"lastError"`
		} `json:"sessionMonitor"`
	} `json:"diagnostics"`
}

type cmAccountsResponse struct {
	Accounts []cmAccount `json:"accounts"`
}

type cmLoginStart struct {
	LoginID         string `json:"loginId"`
	VerificationURL string `json:"verificationUrl"`
	UserCode        string `json:"userCode"`
}

type cmLoginStatus struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type cmChromeProfile struct {
	Name          string   `json:"name"`
	Outcome       string   `json:"outcome"`
	ActiveEmail   string   `json:"activeEmail"`
	SavedAccounts []string `json:"savedAccounts"`
}

type cmChromeProfiles struct {
	Profiles []cmChromeProfile `json:"profiles"`
}

func init() {
	cmCmd.PersistentFlags().BoolVar(&cmJSON, "json", false, "print machine-readable JSON")
	cmCheckCmd.Flags().BoolVar(&cmCheckForce, "force-refresh", false, "refresh sign-in before checking quota")
	cmAddCmd.Flags().BoolVar(&cmAddActivate, "activate", false, "make the imported account active")
	cmLoginCmd.Flags().BoolVar(&cmLoginReplace, "replace", false, "refresh an existing account")
	cmLoginCmd.Flags().BoolVar(&cmLoginOpen, "open", true, "open the verification page in a browser")
	cmLoginCmd.Flags().StringVar(&cmLoginEmail, "email", "", "expected email when replacing an account")
	cmMonitorCmd.Flags().BoolVar(&cmMonitorEnable, "enable", false, "allow automatic cleanup for this account")
	cmMonitorCmd.Flags().BoolVar(&cmMonitorDisable, "disable", false, "only report sessions for this account")
	cmMonitorCmd.MarkFlagsMutuallyExclusive("enable", "disable")
	cmHistoryCmd.Flags().StringVar(&cmHistoryRange, "range", "", "history range: 7d, 30d, or 90d")
	cmHistoryCmd.Flags().IntVar(&cmHistoryLimit, "limit", 100, "maximum samples (1-1000)")
	cmChromeCleanupCmd.Flags().StringVar(&cmChromeCleanupAccount, "account", "", "managed account associated with the profile")
	cmChromeCleanupCmd.Flags().BoolVar(&cmChromeCleanupApply, "apply", false, "revoke instead of previewing")

	cmChromeCmd.AddCommand(cmChromeScanCmd, cmChromeOpenCmd, cmChromeSessionsCmd, cmChromeCleanupCmd)
	cmCmd.AddCommand(cmListCmd, cmStatusCmd, cmCheckCmd, cmUseCmd, cmBestCmd, cmAddCmd, cmLoginCmd, cmRenameCmd, cmRemoveCmd, cmMonitorCmd, cmHistoryCmd, cmChromeCmd)
	rootCmd.AddCommand(cmCmd)
}

func cmList(cmd *cobra.Command) error {
	var response cmAccountsResponse
	if err := cmRequest(cmd.Context(), http.MethodGet, "accounts", nil, &response); err != nil {
		return err
	}
	if cmJSON {
		return cmPrintJSON(cmd, response)
	}
	if len(response.Accounts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No managed Codex accounts. Use `abolqasem cm login <name>` or `abolqasem cm add <name> <auth.json>`.")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "ACTIVE\tNAME\tPLAN\tSTATE\tEMAIL\t5H\tWEEKLY")
	for _, account := range response.Accounts {
		active := ""
		if account.Active {
			active = "*"
		}
		fiveHour, weekly := cmQuota(account, "5"), cmQuota(account, "week")
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", active, account.Name, account.Plan, account.State, account.Email, fiveHour, weekly)
	}
	return nil
}

func cmQuota(account cmAccount, needle string) string {
	needle = strings.ToLower(needle)
	for _, limit := range account.RateLimits.Limits {
		for _, window := range limit.Windows {
			if strings.Contains(strings.ToLower(window.Label), needle) {
				return fmt.Sprintf("%.0f%%", window.RemainingPercent)
			}
		}
	}
	return "-"
}

func cmPrintJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func cmRequest(ctx context.Context, method, path string, payload any, target any) error {
	body, err := cmRequestRaw(ctx, method, path, payload)
	if err != nil {
		return err
	}
	if target == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode local Codex Manager response: %w", err)
	}
	return nil
}

func cmRequestRaw(ctx context.Context, method, path string, payload any) ([]byte, error) {
	if err := ensureServiceRunning(10 * time.Second); err != nil {
		return nil, fmt.Errorf("start local Abolqasem service: %w", err)
	}
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := strings.TrimRight(currentBaseURL(), "/") + "/api/codex-manager"
	if path != "" {
		endpoint += "/" + strings.TrimLeft(path, "/")
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 50 * time.Second}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("request local Codex Manager: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("Codex Manager: %s", message)
	}
	return body, nil
}
