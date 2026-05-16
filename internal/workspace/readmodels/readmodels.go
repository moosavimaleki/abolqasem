package readmodels

import (
	"sort"

	"ai-agent-manager/internal/workspace/events"
)

type ProjectRecord struct {
	ID           string
	LocalPath    string
	Title        string
	SidebarTitle *string
	CreatedAt    int64
	UpdatedAt    int64
	DeletedAt    int64
}

type ChatRecord struct {
	ID              string
	ProjectID       string
	Title           string
	CreatedAt       int64
	UpdatedAt       int64
	DeletedAt       int64
	ArchivedAt      int64
	Unread          bool
	Provider        *string
	PlanMode        bool
	LastMessageAt   int64
	LastTurnOutcome *string
}

type StoreState struct {
	ProjectsByID     map[string]ProjectRecord
	ProjectIDsByPath map[string]string
	ChatsByID        map[string]ChatRecord
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
		ProjectsByID:     map[string]ProjectRecord{},
		ProjectIDsByPath: map[string]string{},
		ChatsByID:        map[string]ChatRecord{},
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

func chatsForProject(state StoreState, projectID string) []ChatRecord {
	chats := make([]ChatRecord, 0)
	for _, chat := range state.ChatsByID {
		if chat.ProjectID != projectID || chat.DeletedAt != 0 {
			continue
		}
		chats = append(chats, chat)
	}
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].UpdatedAt > chats[j].UpdatedAt
	})
	return chats
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
	update(&record)
	record.UpdatedAt = event.Timestamp
	state.ChatsByID[data.ChatID] = record
	return state
}
