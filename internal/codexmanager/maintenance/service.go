package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/auth"
	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/storage"
)

type Config struct {
	IncludeActive bool
	ForceRefresh  bool
	Accounts      []string
	ProxyURL      string
	Retention     time.Duration
	Now           func() time.Time
}

type Result struct {
	Account   string `json:"account"`
	State     string `json:"state"`
	Message   string `json:"message"`
	Refreshed bool   `json:"refreshed"`
}

type Summary struct {
	Results   []Result `json:"results"`
	Refreshed int      `json:"refreshed"`
	Failures  int      `json:"failures"`
}

type Service struct {
	Accounts account.Repository
	Limits   limits.Client
	History  history.Store
	Config   Config
}

func (s Service) Run(ctx context.Context) (Summary, error) {
	now := time.Now
	if s.Config.Now != nil {
		now = s.Config.Now
	}
	active, err := s.Accounts.Active()
	if err != nil {
		return Summary{}, err
	}
	names, err := s.Accounts.List()
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Results: make([]Result, 0, len(names))}
	requested := make(map[string]struct{}, len(s.Config.Accounts))
	for _, name := range s.Config.Accounts {
		requested[name] = struct{}{}
	}
	for _, name := range names {
		if len(requested) > 0 {
			if _, ok := requested[name]; !ok {
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if name == active && !s.Config.IncludeActive {
			summary.Results = append(summary.Results, Result{Account: name, State: "ok", Message: "active; skipped refresh"})
			continue
		}
		result := s.checkOne(ctx, name, active, now())
		summary.Results = append(summary.Results, result)
		if result.Refreshed {
			summary.Refreshed++
		}
		if result.State != "ok" {
			summary.Failures++
		}
	}
	if s.Config.Retention > 0 {
		_, _ = s.History.Prune(ctx, s.Config.Retention, now())
	}
	return summary, nil
}

func (s Service) checkOne(ctx context.Context, name, active string, now time.Time) Result {
	result := Result{Account: name, State: "ok"}
	credentials, err := s.Accounts.Read(name)
	if err != nil {
		return failed(result, "needs_login", err)
	}
	limitsClient := s.Limits
	limitsClient.ProxyURL = s.Config.ProxyURL
	needed, reason := auth.ShouldRefresh(credentials, now)
	if s.Config.ForceRefresh {
		needed = true
		reason = "force refresh requested"
	}
	if needed {
		client, clientErr := auth.NewHTTPClient(s.Config.ProxyURL, 30*time.Second)
		if clientErr != nil {
			return failed(result, "error", clientErr)
		}
		refreshed, refreshErr := (auth.Refresher{Client: client, Now: func() time.Time { return now }}).Refresh(ctx, credentials)
		if refreshErr != nil {
			return failed(result, "needs_login", refreshErr)
		}
		if syncErr := s.Accounts.Sync(ctx, name, refreshed); syncErr != nil {
			return failed(result, "error", syncErr)
		}
		credentials = refreshed
		result.Refreshed = true
		result.Message = "refreshed: " + reason
	} else {
		result.Message = reason
	}
	snapshot, fetchErr := limitsClient.Fetch(ctx, name, credentials)
	if fetchErr != nil {
		return failed(result, errorState(fetchErr), fetchErr)
	}
	_, _ = s.History.Append(ctx, snapshot)
	if name == active {
		// The live auth file is owned by app-server. Maintenance intentionally
		// never overwrites it while a turn is running.
		result.Message += "; active credentials unchanged"
	}
	_ = writeStatus(s.Accounts.Paths, name, result, snapshot)
	return result
}

func writeStatus(paths storage.Paths, name string, result Result, snapshot limits.Snapshot) error {
	statusPath, err := paths.Status(name)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"state":       result.State,
		"message":     result.Message,
		"checked_at":  snapshot.FetchedAt,
		"rate_limits": snapshot,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := storage.EnsureDirs(paths); err != nil {
		return err
	}
	return os.WriteFile(statusPath, append(data, '\n'), 0600)
}

func failed(result Result, state string, err error) Result {
	result.State = state
	result.Message = safeError(err)
	return result
}

func errorState(err error) string {
	var fetchErr *limits.FetchError
	if errors.As(err, &fetchErr) && fetchErr.Kind == limits.ErrorAuth {
		return "needs_login"
	}
	return "warning"
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
