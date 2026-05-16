package readmodels

import (
	"sort"

	"ai-agent-manager/internal/providers/catalog"
	"ai-agent-manager/internal/workspace/events"
)

type ProjectRecord struct {
	ID           string  `json:"id"`
	LocalPath    string  `json:"localPath"`
	Title        string  `json:"title"`
	SidebarTitle *string `json:"sidebarTitle,omitempty"`
	CreatedAt    int64   `json:"createdAt"`
	UpdatedAt    int64   `json:"updatedAt"`
	DeletedAt    int64   `json:"deletedAt,omitempty"`
}

type ChatRecord struct {
	ID                      string  `json:"id"`
	ProjectID               string  `json:"projectId"`
	Title                   string  `json:"title"`
	CreatedAt               int64   `json:"createdAt"`
	UpdatedAt               int64   `json:"updatedAt"`
	DeletedAt               int64   `json:"deletedAt,omitempty"`
	ArchivedAt              int64   `json:"archivedAt,omitempty"`
	Unread                  bool    `json:"unread"`
	Provider                *string `json:"provider"`
	PlanMode                bool    `json:"planMode"`
	SessionToken            *string `json:"sessionToken"`
	PendingForkSessionToken *string `json:"pendingForkSessionToken,omitempty"`
	HasMessages             bool    `json:"hasMessages,omitempty"`
	LastMessageAt           int64   `json:"lastMessageAt,omitempty"`
	LastTurnOutcome         *string `json:"lastTurnOutcome"`
}

type StoreState struct {
	ProjectsByID           map[string]ProjectRecord
	ProjectIDsByPath       map[string]string
	ChatsByID              map[string]ChatRecord
	QueuedMessagesByChatID map[string][]QueuedChatMessage
}

type KannaStatus string

const (
	StatusIdle           KannaStatus = "idle"
	StatusStarting       KannaStatus = "starting"
	StatusRunning        KannaStatus = "running"
	StatusWaitingForUser KannaStatus = "waiting_for_user"
	StatusFailed         KannaStatus = "failed"
	StatusCancelled      KannaStatus = "cancelled"
)

type ChatAttachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	DisplayName  string `json:"displayName"`
	AbsolutePath string `json:"absolutePath"`
	RelativePath string `json:"relativePath"`
	ContentURL   string `json:"contentUrl"`
	MimeType     string `json:"mimeType"`
	Size         int64  `json:"size"`
}

type QueuedChatMessage struct {
	ID           string                `json:"id"`
	Content      string                `json:"content"`
	Attachments  []ChatAttachment      `json:"attachments"`
	CreatedAt    int64                 `json:"createdAt"`
	Provider     *string               `json:"provider,omitempty"`
	Model        string                `json:"model,omitempty"`
	ModelOptions *catalog.ModelOptions `json:"modelOptions,omitempty"`
	PlanMode     *bool                 `json:"planMode,omitempty"`
}

type TranscriptEntry map[string]any

type ChatRuntime struct {
	ChatID       string      `json:"chatId"`
	ProjectID    string      `json:"projectId"`
	LocalPath    string      `json:"localPath"`
	Title        string      `json:"title"`
	Status       KannaStatus `json:"status"`
	IsDraining   bool        `json:"isDraining"`
	Provider     *string     `json:"provider"`
	PlanMode     bool        `json:"planMode"`
	SessionToken *string     `json:"sessionToken"`
}

type ChatHistorySnapshot struct {
	HasOlder    bool    `json:"hasOlder"`
	OlderCursor *string `json:"olderCursor"`
	RecentLimit int     `json:"recentLimit"`
}

type ChatTranscriptSnapshot struct {
	Messages []TranscriptEntry   `json:"messages"`
	History  ChatHistorySnapshot `json:"history"`
}

type ChatSnapshot struct {
	Runtime            ChatRuntime                    `json:"runtime"`
	QueuedMessages     []QueuedChatMessage            `json:"queuedMessages"`
	Messages           []TranscriptEntry              `json:"messages"`
	History            ChatHistorySnapshot            `json:"history"`
	AvailableProviders []catalog.ProviderCatalogEntry `json:"availableProviders"`
}

type DiscoveredProject struct {
	LocalPath  string
	Title      string
	ModifiedAt int64
}

type LocalProjectsSnapshot struct {
	Machine  LocalProjectsMachine `json:"machine"`
	Projects []LocalProjectRow    `json:"projects"`
}

type LocalProjectsMachine struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Platform    string `json:"platform"`
}

type LocalProjectRow struct {
	LocalPath    string `json:"localPath"`
	Title        string `json:"title"`
	Source       string `json:"source"`
	LastOpenedAt int64  `json:"lastOpenedAt"`
	ChatCount    int    `json:"chatCount"`
}

type SidebarData struct {
	ProjectGroups []SidebarProjectGroup `json:"projectGroups"`
}

type SidebarProjectGroup struct {
	GroupKey         string           `json:"groupKey"`
	Title            string           `json:"title"`
	RealTitle        string           `json:"realTitle"`
	SidebarTitle     string           `json:"sidebarTitle,omitempty"`
	LocalPath        string           `json:"localPath"`
	Chats            []SidebarChatRow `json:"chats"`
	PreviewChats     []SidebarChatRow `json:"previewChats"`
	OlderChats       []SidebarChatRow `json:"olderChats"`
	ArchivedChats    []SidebarChatRow `json:"archivedChats,omitempty"`
	DefaultCollapsed bool             `json:"defaultCollapsed"`
}

type SidebarChatRow struct {
	ID            string  `json:"_id"`
	CreationTime  int64   `json:"_creationTime"`
	ChatID        string  `json:"chatId"`
	Title         string  `json:"title"`
	Status        string  `json:"status"`
	Unread        bool    `json:"unread"`
	LocalPath     string  `json:"localPath"`
	Provider      *string `json:"provider"`
	LastMessageAt *int64  `json:"lastMessageAt,omitempty"`
	HasAutomation bool    `json:"hasAutomation"`
	CanFork       bool    `json:"canFork,omitempty"`
}

func EmptyState() StoreState {
	return StoreState{
		ProjectsByID:           map[string]ProjectRecord{},
		ProjectIDsByPath:       map[string]string{},
		ChatsByID:              map[string]ChatRecord{},
		QueuedMessagesByChatID: map[string][]QueuedChatMessage{},
	}
}

func Apply(state StoreState, event events.Event) StoreState {
	switch event.Type {
	case events.TypeProjectOpened:
		var data struct {
			ProjectID string `json:"projectId"`
			LocalPath string `json:"localPath"`
			Title     string `json:"title"`
		}
		if event.DecodeData(&data) != nil || data.ProjectID == "" {
			return state
		}
		record := state.ProjectsByID[data.ProjectID]
		if record.CreatedAt == 0 {
			record.CreatedAt = event.Timestamp
		}
		record.ID = data.ProjectID
		record.LocalPath = data.LocalPath
		record.Title = data.Title
		record.UpdatedAt = event.Timestamp
		record.DeletedAt = 0
		state.ProjectsByID[record.ID] = record
		if record.LocalPath != "" {
			state.ProjectIDsByPath[record.LocalPath] = record.ID
		}
	case events.TypeProjectSidebarRenamed:
		var data struct {
			ProjectID string  `json:"projectId"`
			Title     *string `json:"title"`
		}
		if event.DecodeData(&data) != nil || data.ProjectID == "" {
			return state
		}
		record := state.ProjectsByID[data.ProjectID]
		record.SidebarTitle = data.Title
		record.UpdatedAt = event.Timestamp
		state.ProjectsByID[data.ProjectID] = record
	case events.TypeProjectRemoved:
		var data struct {
			ProjectID string `json:"projectId"`
		}
		if event.DecodeData(&data) != nil || data.ProjectID == "" {
			return state
		}
		record := state.ProjectsByID[data.ProjectID]
		record.DeletedAt = event.Timestamp
		record.UpdatedAt = event.Timestamp
		state.ProjectsByID[data.ProjectID] = record
	case events.TypeChatCreated:
		var data struct {
			ChatID    string `json:"chatId"`
			ProjectID string `json:"projectId"`
			Title     string `json:"title"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		if record.CreatedAt == 0 {
			record.CreatedAt = event.Timestamp
		}
		record.ID = data.ChatID
		record.ProjectID = data.ProjectID
		record.Title = data.Title
		record.UpdatedAt = event.Timestamp
		record.Unread = false
		state.ChatsByID[record.ID] = record
	case events.TypeChatRenamed:
		var data struct {
			ChatID string `json:"chatId"`
			Title  string `json:"title"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.Title = data.Title
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeChatDeleted:
		state = markChatTimestamp(state, event, func(record *ChatRecord) { record.DeletedAt = event.Timestamp })
	case events.TypeChatArchived:
		state = markChatTimestamp(state, event, func(record *ChatRecord) { record.ArchivedAt = event.Timestamp })
	case events.TypeChatUnarchived:
		state = markChatTimestamp(state, event, func(record *ChatRecord) { record.ArchivedAt = 0 })
	case events.TypeChatProviderSet:
		var data struct {
			ChatID   string `json:"chatId"`
			Provider string `json:"provider"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.Provider = &data.Provider
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeChatPlanModeSet:
		var data struct {
			ChatID   string `json:"chatId"`
			PlanMode bool   `json:"planMode"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.PlanMode = data.PlanMode
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeChatReadStateSet:
		var data struct {
			ChatID string `json:"chatId"`
			Unread bool   `json:"unread"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.Unread = data.Unread
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeMessageAppended:
		var data struct {
			ChatID string          `json:"chatId"`
			Entry  TranscriptEntry `json:"entry"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.HasMessages = true
		if data.Entry["kind"] == "user_prompt" {
			if createdAt, ok := numberAsInt64(data.Entry["createdAt"]); ok {
				record.LastMessageAt = createdAt
				if createdAt > record.UpdatedAt {
					record.UpdatedAt = createdAt
				}
			}
		}
		state.ChatsByID[data.ChatID] = record
	case events.TypeQueuedMessageEnqueued:
		var data struct {
			ChatID  string            `json:"chatId"`
			Message QueuedChatMessage `json:"message"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		state.QueuedMessagesByChatID[data.ChatID] = append(state.QueuedMessagesByChatID[data.ChatID], cloneQueuedMessage(data.Message))
		record := state.ChatsByID[data.ChatID]
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeQueuedMessageRemoved:
		var data struct {
			ChatID          string `json:"chatId"`
			QueuedMessageID string `json:"queuedMessageId"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		existing := state.QueuedMessagesByChatID[data.ChatID]
		next := existing[:0]
		for _, message := range existing {
			if message.ID != data.QueuedMessageID {
				next = append(next, message)
			}
		}
		if len(next) == 0 {
			delete(state.QueuedMessagesByChatID, data.ChatID)
		} else {
			state.QueuedMessagesByChatID[data.ChatID] = next
		}
		record := state.ChatsByID[data.ChatID]
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypeTurnStarted:
		state = markChatTimestamp(state, event, nil)
	case events.TypeTurnFinished:
		outcome := "success"
		state = markChatTimestamp(state, event, func(record *ChatRecord) {
			record.Unread = true
			record.LastTurnOutcome = &outcome
		})
	case events.TypeTurnFailed:
		outcome := "failed"
		state = markChatTimestamp(state, event, func(record *ChatRecord) {
			record.Unread = true
			record.LastTurnOutcome = &outcome
		})
	case events.TypeTurnCancelled:
		outcome := "cancelled"
		state = markChatTimestamp(state, event, func(record *ChatRecord) {
			record.LastTurnOutcome = &outcome
		})
	case events.TypeSessionTokenSet:
		var data struct {
			ChatID       string  `json:"chatId"`
			SessionToken *string `json:"sessionToken"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.SessionToken = data.SessionToken
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	case events.TypePendingForkSessionTokenSet:
		var data struct {
			ChatID                  string  `json:"chatId"`
			PendingForkSessionToken *string `json:"pendingForkSessionToken"`
		}
		if event.DecodeData(&data) != nil || data.ChatID == "" {
			return state
		}
		record := state.ChatsByID[data.ChatID]
		record.PendingForkSessionToken = data.PendingForkSessionToken
		record.UpdatedAt = event.Timestamp
		state.ChatsByID[data.ChatID] = record
	}
	return state
}

func Replay(eventsList []events.Event) StoreState {
	state := EmptyState()
	for _, event := range eventsList {
		state = Apply(state, event)
	}
	return state
}

func DeriveSidebarData(state StoreState) SidebarData {
	projects := make([]ProjectRecord, 0, len(state.ProjectsByID))
	for _, project := range state.ProjectsByID {
		if project.DeletedAt != 0 {
			continue
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].UpdatedAt > projects[j].UpdatedAt
	})

	groups := make([]SidebarProjectGroup, 0, len(projects))
	for _, project := range projects {
		title := project.Title
		sidebarTitle := ""
		if project.SidebarTitle != nil && *project.SidebarTitle != "" {
			title = *project.SidebarTitle
			sidebarTitle = *project.SidebarTitle
		}
		group := SidebarProjectGroup{
			GroupKey:         project.ID,
			Title:            title,
			RealTitle:        project.Title,
			SidebarTitle:     sidebarTitle,
			LocalPath:        project.LocalPath,
			Chats:            []SidebarChatRow{},
			PreviewChats:     []SidebarChatRow{},
			OlderChats:       []SidebarChatRow{},
			ArchivedChats:    []SidebarChatRow{},
			DefaultCollapsed: false,
		}
		for _, chat := range chatsForProject(state, project.ID) {
			row := sidebarRow(project, chat)
			if chat.ArchivedAt != 0 {
				group.ArchivedChats = append(group.ArchivedChats, row)
				continue
			}
			group.Chats = append(group.Chats, row)
		}
		groups = append(groups, group)
	}
	return SidebarData{ProjectGroups: groups}
}

func DeriveStatus(chat ChatRecord, activeStatus KannaStatus) KannaStatus {
	if activeStatus != "" {
		return activeStatus
	}
	if chat.LastTurnOutcome != nil && *chat.LastTurnOutcome == "failed" {
		return StatusFailed
	}
	return StatusIdle
}

func DeriveChatSnapshot(
	state StoreState,
	activeStatuses map[string]KannaStatus,
	drainingChatIDs map[string]bool,
	chatID string,
	transcript ChatTranscriptSnapshot,
) *ChatSnapshot {
	chat, ok := state.ChatsByID[chatID]
	if !ok || chat.DeletedAt != 0 {
		return nil
	}
	project, ok := state.ProjectsByID[chat.ProjectID]
	if !ok || project.DeletedAt != 0 {
		return nil
	}

	queuedMessages := state.QueuedMessagesByChatID[chat.ID]
	clonedQueued := make([]QueuedChatMessage, 0, len(queuedMessages))
	for _, message := range queuedMessages {
		clonedQueued = append(clonedQueued, cloneQueuedMessage(message))
	}

	return &ChatSnapshot{
		Runtime: ChatRuntime{
			ChatID:       chat.ID,
			ProjectID:    project.ID,
			LocalPath:    project.LocalPath,
			Title:        chat.Title,
			Status:       DeriveStatus(chat, activeStatuses[chat.ID]),
			IsDraining:   drainingChatIDs[chat.ID],
			Provider:     chat.Provider,
			PlanMode:     chat.PlanMode,
			SessionToken: chat.SessionToken,
		},
		QueuedMessages:     clonedQueued,
		Messages:           transcript.Messages,
		History:            transcript.History,
		AvailableProviders: catalog.ServerProviders(),
	}
}

func DeriveLocalProjectsSnapshot(
	state StoreState,
	discoveredProjects []DiscoveredProject,
	machineName string,
	platform string,
) LocalProjectsSnapshot {
	projects := map[string]LocalProjectRow{}

	for _, project := range discoveredProjects {
		if project.LocalPath == "" {
			continue
		}
		projects[project.LocalPath] = LocalProjectRow{
			LocalPath:    project.LocalPath,
			Title:        project.Title,
			Source:       "discovered",
			LastOpenedAt: project.ModifiedAt,
			ChatCount:    0,
		}
	}

	for _, project := range state.ProjectsByID {
		if project.DeletedAt != 0 {
			continue
		}
		chatCount := 0
		lastOpenedAt := project.UpdatedAt
		for _, chat := range state.ChatsByID {
			if chat.ProjectID != project.ID || chat.DeletedAt != 0 || chat.ArchivedAt != 0 {
				continue
			}
			chatCount++
			if getSidebarChatSortTimestamp(chat) > lastOpenedAt {
				lastOpenedAt = getSidebarChatSortTimestamp(chat)
			}
		}
		projects[project.LocalPath] = LocalProjectRow{
			LocalPath:    project.LocalPath,
			Title:        project.Title,
			Source:       "saved",
			LastOpenedAt: lastOpenedAt,
			ChatCount:    chatCount,
		}
	}

	rows := make([]LocalProjectRow, 0, len(projects))
	for _, project := range projects {
		rows = append(rows, project)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].LastOpenedAt > rows[j].LastOpenedAt
	})

	return LocalProjectsSnapshot{
		Machine: LocalProjectsMachine{
			ID:          "local",
			DisplayName: machineName,
			Platform:    platform,
		},
		Projects: rows,
	}
}

func chatsForProject(state StoreState, projectID string) []ChatRecord {
	chats := make([]ChatRecord, 0)
	for _, chat := range state.ChatsByID {
		if chat.ProjectID != projectID || chat.DeletedAt != 0 {
			continue
		}
		chats = append(chats, chat)
	}
	sort.Slice(chats, func(i, j int) bool {
		return getSidebarChatSortTimestamp(chats[i]) > getSidebarChatSortTimestamp(chats[j])
	})
	return chats
}

func getSidebarChatSortTimestamp(chat ChatRecord) int64 {
	if chat.LastMessageAt != 0 {
		return chat.LastMessageAt
	}
	return chat.CreatedAt
}

func sidebarRow(project ProjectRecord, chat ChatRecord) SidebarChatRow {
	var lastMessageAt *int64
	if chat.LastMessageAt != 0 {
		lastMessageAt = &chat.LastMessageAt
	}
	return SidebarChatRow{
		ID:            chat.ID,
		CreationTime:  chat.CreatedAt,
		ChatID:        chat.ID,
		Title:         chat.Title,
		Status:        "idle",
		Unread:        chat.Unread,
		LocalPath:     project.LocalPath,
		Provider:      chat.Provider,
		LastMessageAt: lastMessageAt,
		HasAutomation: false,
		CanFork:       chat.Provider != nil,
	}
}

func markChatTimestamp(state StoreState, event events.Event, update func(*ChatRecord)) StoreState {
	var data struct {
		ChatID string `json:"chatId"`
	}
	if event.DecodeData(&data) != nil || data.ChatID == "" {
		return state
	}
	record := state.ChatsByID[data.ChatID]
	if update != nil {
		update(&record)
	}
	record.UpdatedAt = event.Timestamp
	state.ChatsByID[data.ChatID] = record
	return state
}

func cloneQueuedMessage(message QueuedChatMessage) QueuedChatMessage {
	cloned := message
	cloned.Attachments = append([]ChatAttachment(nil), message.Attachments...)
	return cloned
}

func numberAsInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}
