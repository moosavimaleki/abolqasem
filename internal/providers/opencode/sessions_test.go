package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestListSessionsParsesCLIPrefixAndJSON(t *testing.T) {
	previousPath := databasePath
	databasePath = func() string { return "" }
	t.Cleanup(func() { databasePath = previousPath })
	previous := execCommandContext
	execCommandContext = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("OpenCode sessions\n[{\"id\":\"ses_test\",\"title\":\"Test\",\"directory\":\"/work\",\"updated\":123}]\nOpenCode ready"), nil
	}
	t.Cleanup(func() { execCommandContext = previous })
	sessions, err := ListSessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ses_test" || sessions[0].Directory != "/work" {
		t.Fatalf("sessions=%#v err=%v", sessions, err)
	}
}

func TestListSessionsAcceptsCompleteJSONWhenCLITimesOut(t *testing.T) {
	previousPath := databasePath
	databasePath = func() string { return "" }
	t.Cleanup(func() { databasePath = previousPath })
	previous := execCommandContext
	execCommandContext = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`[{"id":"ses_complete","directory":"/work"}]`), context.DeadlineExceeded
	}
	t.Cleanup(func() { execCommandContext = previous })
	sessions, err := ListSessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ses_complete" {
		t.Fatalf("sessions=%#v err=%v", sessions, err)
	}
}

func TestOpenCodeDatabaseReadsSessionsAndExportsTranscript(t *testing.T) {
	databaseFile := filepath.Join(t.TempDir(), "opencode.db")
	database, err := sql.Open("sqlite", databaseFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
		CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT, title TEXT, agent TEXT, model TEXT, permission TEXT, time_created INTEGER, time_updated INTEGER, time_archived INTEGER);
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, time_created INTEGER, data TEXT);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT, session_id TEXT, time_created INTEGER, data TEXT);
		INSERT INTO session VALUES ('ses_db','project','/work','Database session','build','{"providerID":"opencode"}','[]',100,200,NULL);
		INSERT INTO message VALUES ('msg_db','ses_db',110,'{"role":"user","time":{"created":110}}');
		INSERT INTO part VALUES ('part_db','msg_db','ses_db',111,'{"type":"text","text":"hello"}');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	previousPath := databasePath
	databasePath = func() string { return databaseFile }
	t.Cleanup(func() { databasePath = previousPath })
	sessions, err := ListSessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ses_db" {
		t.Fatalf("sessions=%#v err=%v", sessions, err)
	}
	data, err := ExportSession(context.Background(), "ses_db")
	if err != nil || string(data) == "" || !json.Valid(data) || !strings.Contains(string(data), "hello") {
		t.Fatalf("export=%s err=%v", data, err)
	}
}
