package opencode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// databasePath is intentionally replaceable in tests. OpenCode keeps one
// machine-local SQLite database for every project; reading it in read-only mode
// gives us the complete session index without starting a heavy Bun helper for
// every workspace directory.
var databasePath = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

func openDatabase() (*sql.DB, error) {
	path := strings.TrimSpace(databasePath())
	if path == "" {
		return nil, fmt.Errorf("OpenCode database path is unavailable")
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return nil, fmt.Errorf("OpenCode database is unavailable at %s", path)
	}
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(1000)")
	if err != nil {
		return nil, err
	}
	// Exporting a session nests the message and part queries. Keep a small,
	// bounded pool so an active cursor never waits on itself.
	database.SetMaxOpenConns(4)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

func listSessionsFromDatabase() ([]SessionSummary, error) {
	database, err := openDatabase()
	if err != nil {
		return nil, err
	}
	defer database.Close()
	rows, err := database.Query(`
		SELECT id, title, directory, time_updated, time_created
		FROM session
		WHERE time_archived IS NULL
		ORDER BY time_updated DESC
		LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]SessionSummary, 0, 32)
	for rows.Next() {
		var session SessionSummary
		if err := rows.Scan(&session.ID, &session.Title, &session.Directory, &session.Updated, &session.Created); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func exportSessionFromDatabase(sessionID string) ([]byte, error) {
	database, err := openDatabase()
	if err != nil {
		return nil, err
	}
	defer database.Close()
	var projectID, directory, title, agent, model, permission string
	var created, updated int64
	err = database.QueryRow(`
		SELECT project_id, directory, title, COALESCE(agent, ''), COALESCE(model, ''), COALESCE(permission, ''), time_created, time_updated
		FROM session WHERE id = ?`, strings.TrimSpace(sessionID)).Scan(
		&projectID, &directory, &title, &agent, &model, &permission, &created, &updated,
	)
	if err != nil {
		return nil, err
	}
	info := map[string]any{
		"id":        strings.TrimSpace(sessionID),
		"projectID": projectID,
		"directory": directory,
		"title":     title,
		"agent":     agent,
		"time": map[string]any{
			"created": created,
			"updated": updated,
		},
	}
	decodeJSONField(info, "model", model)
	decodeJSONField(info, "permission", permission)

	messageRows, err := database.Query(`
		SELECT id, data FROM message WHERE session_id = ? ORDER BY time_created, id`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer messageRows.Close()
	messages := make([]map[string]any, 0, 16)
	for messageRows.Next() {
		var messageID, data string
		if err := messageRows.Scan(&messageID, &data); err != nil {
			return nil, err
		}
		message := jsonMap(data)
		message["id"] = messageID
		message["sessionID"] = strings.TrimSpace(sessionID)
		partRows, err := database.Query(`
			SELECT id, data FROM part WHERE message_id = ? ORDER BY time_created, id`, messageID)
		if err != nil {
			return nil, err
		}
		parts := make([]map[string]any, 0, 4)
		for partRows.Next() {
			var partID, partData string
			if err := partRows.Scan(&partID, &partData); err != nil {
				_ = partRows.Close()
				return nil, err
			}
			part := jsonMap(partData)
			part["id"] = partID
			part["sessionID"] = strings.TrimSpace(sessionID)
			part["messageID"] = messageID
			parts = append(parts, part)
		}
		if err := partRows.Close(); err != nil {
			return nil, err
		}
		message["parts"] = parts
		messages = append(messages, map[string]any{"info": message, "parts": parts})
	}
	if err := messageRows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"info": info, "messages": messages})
}

func decodeJSONField(target map[string]any, key string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) == nil {
		target[key] = decoded
	}
}

func jsonMap(value string) map[string]any {
	result := map[string]any{}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}
