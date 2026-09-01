package maintenance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"abolqasem/internal/codexmanager/account"
	"abolqasem/internal/codexmanager/history"
	"abolqasem/internal/codexmanager/limits"
	"abolqasem/internal/codexmanager/storage"
)

func authFixture(subject string) map[string]any {
	claims, _ := json.Marshal(map[string]any{"sub": subject, "exp": time.Now().Add(48 * time.Hour).Unix()})
	token := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
	return map[string]any{"tokens": map[string]any{"refresh_token": "redacted", "access_token": token, "id_token": token}}
}

func TestRunSkipsActiveRefreshAndWritesStatus(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"rate_limit":{"primary_window":{"used_percent":20}}}`))
	}))
	defer httpServer.Close()
	home := t.TempDir()
	repo := account.Repository{Paths: storage.Paths{Home: home}}
	ctx := context.Background()
	if err := repo.Add(ctx, "active", authFixture("one"), false); err != nil {
		t.Fatal(err)
	}
	if err := repo.Add(ctx, "other", authFixture("two"), false); err != nil {
		t.Fatal(err)
	}
	if err := repo.Activate(ctx, "active"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	service := Service{Accounts: repo, Limits: limits.Client{URL: httpServer.URL, Now: func() time.Time { return now }}, History: history.Store{Paths: storage.Paths{Home: home}}, Config: Config{Now: func() time.Time { return now }, Retention: 24 * time.Hour}}
	summary, err := service.Run(ctx)
	if err != nil || summary.Failures != 0 || len(summary.Results) != 2 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	if summary.Results[0].Account != "active" || summary.Results[0].Message != "active; skipped refresh" {
		t.Fatalf("unexpected active result: %#v", summary.Results[0])
	}
	if _, err := repo.Paths.Status("other"); err != nil {
		t.Fatal(err)
	}
	store := history.Store{Paths: storage.Paths{Home: home}}
	if _, err := store.Read("other", time.Time{}, 10); err != nil {
		t.Fatal(err)
	}
}
