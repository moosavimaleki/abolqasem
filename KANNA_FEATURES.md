# Kanna Feature Inventory

This document lists the main product, UI, backend, and agent features observed in
`/home/h-mousavi/Projects/Hamed/kanna`. It is intended as a reference for
planning the future `ai-agent-manager` Web UI.

## Product Positioning

- Local web UI for AI coding agents.
- Supports both chat-based agent control and session/history viewing.
- Project-first workflow instead of only recent-session viewing.
- Designed as an agent workbench, not only a transcript viewer.
- Runs locally and works with local project folders.
- Keeps user data on disk in local app storage.
- Supports resumable chats and persistent history.
- Supports provider-specific capabilities instead of treating all agents the same.

## Architecture Features

- React client with Zustand-style local state.
- Bun-based local server.
- WebSocket command/subscription protocol.
- Event-sourced backend state.
- JSONL append-only event logs.
- Snapshot compaction for faster startup.
- CQRS-style read models.
- Separate write events from UI snapshots.
- Runtime state for active turns.
- Provider-agnostic transcript model.
- Project, chat, message, queue, turn, and runtime entities.
- Explicit command protocol for all user actions.
- Topic-based UI subscriptions.
- Server-side diff, terminal, settings, provider, update, and agent modules.

## Storage Features

- Local data directory under `~/.kanna/data`.
- Persistent project records.
- Persistent chat records.
- Persistent message records.
- Persistent queued-message records.
- Persistent turn records.
- Snapshot file for compacted state.
- Transcript history pagination.
- Sidebar ordering persistence.
- Cleanup for stale empty chats.
- Data replay from event logs on startup.

## Project Management

- Add/open local projects.
- Create project records from local paths.
- Rename project sidebar title.
- Hide/remove project from sidebar.
- Discover local projects from agent history.
- Merge discovered projects with saved projects.
- Project-first sidebar grouping.
- Drag/drop project ordering.
- Collapse/expand project groups.
- Archived chats grouped by project.
- Project-local quick actions.
- Open project in editor.
- Open project in file manager.
- Copy project path.

## Chat And Session Management

- Create chat under a project.
- Rename chat.
- Delete chat.
- Archive chat.
- Unarchive chat.
- Protect chat from cleanup/deletion flows.
- Fork chat.
- Resume previous chat.
- Load older transcript history.
- Mark chat as read.
- Track unread state.
- Track chat provider.
- Track chat plan-mode setting.
- Track active runtime state per chat.
- Track last turn outcome.
- Track session token per provider.
- Track pending fork session token.
- Derive chat status from runtime and last turn.
- Sidebar shows recent and older chats.
- Sidebar supports active chat selection.
- Keyboard shortcuts for sidebar navigation.
- Number-jump hints for quick chat switching.

## Message And Transcript Features

- Provider-agnostic transcript entries.
- User prompt entries.
- Agent message entries.
- Tool call entries.
- Tool result entries.
- Thinking/reasoning entries.
- Error entries.
- Plan update entries.
- Command execution entries.
- File change entries.
- Web search entries.
- MCP tool call entries.
- Dynamic tool call entries.
- Transcript hydration from provider events.
- Message deduplication by entry id.
- Optimistic user prompt rendering.
- Transcript history merging.
- Scroll/follow behavior for active conversations.
- Rich markdown rendering.
- Code block rendering.
- Attachment rendering.
- Standalone transcript export.

## Agent Provider Features

- Provider abstraction for multiple agents.
- Claude provider.
- Codex provider.
- Provider catalog exposed to client.
- Provider-specific model lists.
- Provider-specific default model.
- Provider-specific model normalization.
- Provider-specific model options.
- Provider-specific capabilities.
- Provider-specific plan-mode support.
- Provider-specific context-window support.
- Provider-specific reasoning-effort support.
- Provider-specific fast/slow service tier.
- Provider-locked composer while a chat is active.
- Default provider preference.
- Per-provider default model settings.

## Model And Thinking Controls

- Model picker in chat composer.
- Reasoning effort picker.
- Claude reasoning levels.
- Codex reasoning levels.
- Claude context-window picker.
- Codex fast mode.
- Plan mode toggle where supported.
- Model option persistence per chat composer.
- Provider defaults in settings.
- Normalization of unknown model ids.
- UI hints for model/client compatibility.

## Message Sending Features

- Send message to selected provider.
- Start a new chat.
- Continue existing chat.
- Send with selected model.
- Send with selected reasoning effort.
- Send with selected plan mode.
- Send with attachments.
- Queue message when chat is already active.
- Dequeue and start next message automatically.
- Steer an active turn with a new user message.
- Cancel active turn.
- Stop draining stream.
- Preserve active provider while session is running.
- Previous-prompt awareness in composer.
- Disable composer when no project is selected.

## Codex Integration

- Starts `codex app-server`.
- Initializes app-server with experimental API capability.
- Uses `thread/start`.
- Uses `thread/resume`.
- Uses `thread/fork`.
- Uses `turn/start`.
- Keeps persistent app-server session context per chat.
- Reuses Codex session state where possible.
- Recovers from some failed resume cases by starting a new thread.
- Supports Codex model selection.
- Supports Codex reasoning effort.
- Supports Codex fast service tier.
- Supports Codex plan/default collaboration mode.
- Receives Codex JSON-RPC responses.
- Receives Codex JSON-RPC notifications.
- Handles Codex server requests.
- Handles Codex user-input tool requests.
- Handles Codex command approval requests.
- Handles Codex file-change approval requests.
- Converts Codex raw events into transcript entries.
- Tracks Codex token usage/context window snapshots.
- Emits synthetic system-init events.
- Emits session-token events.
- Emits result success/error/cancelled events.

## Claude Integration

- Uses Claude Agent SDK.
- Starts Claude sessions through SDK query stream.
- Keeps reusable Claude sessions per chat where possible.
- Supports Claude model selection.
- Supports Claude reasoning effort.
- Supports Claude context-window mode.
- Supports Claude plan mode.
- Supports Claude permission mode changes.
- Supports prompt queueing.
- Supports can-use-tool callbacks.
- Supports `AskUserQuestion`.
- Supports `ExitPlanMode`.
- Supports `EnterPlanMode`.
- Supports allowed tools list.
- Converts Claude SDK stream messages into transcript entries.
- Tracks Claude session id.
- Handles Claude result success and error states.

## Tool Approval And Human-In-The-Loop

- Runtime can enter `waiting_for_user` state.
- Tool request emitted to UI.
- User can approve/deny/respond to tool request.
- `chat.respondTool` command resumes pending tool.
- Supports user-question answer maps.
- Supports plan approval flow.
- Supports rejecting unsupported dynamic tools.
- Supports clearing/continuing context after plan approval.
- Supports command/file-change approval routing.

## Queue And Runtime Features

- Active turn tracking.
- Draining stream tracking.
- Queued messages per chat.
- Message enqueue event.
- Message dequeue/remove event.
- Turn started event.
- Turn finished event.
- Turn failed event.
- Turn cancelled event.
- Runtime status derived from active turn.
- Cancel active turn.
- Interrupt active stream.
- Continue next queued message after completion.
- Background title generation after user prompt.
- Background account/provider info refresh.

## Git And Diff Features

- Git repository detection.
- Git initialization.
- Git status refresh.
- File diff listing.
- Lazy diff patch loading.
- Unified diff view.
- Split diff view.
- Line wrapping toggle.
- File selection for commit.
- Include/exclude files from commit.
- Generate commit message with LLM.
- Commit selected files.
- Commit and push selected files.
- Push to remote.
- Pull from remote.
- Fetch remote.
- Publish branch.
- Branch list.
- Branch checkout.
- Create branch.
- Merge preview.
- Merge branch.
- Branch history.
- Ahead/behind tracking.
- Upstream tracking.
- Default branch detection.
- GitHub remote detection.
- GitHub repository availability check.
- Publish project to GitHub.
- Open changed file in editor.
- Open changed file in file manager.
- Copy file path.
- Copy relative path.
- Ignore untracked file.
- Ignore untracked folder.
- Attachment preview for changed image/PDF files.

## Terminal Features

- Embedded terminal panel.
- Multiple terminal panes.
- Terminal create/input/resize/close commands.
- Terminal output events over WebSocket.
- Terminal exit events.
- Per-project terminal layout.
- Terminal split layout.
- Terminal show/hide animation.
- Terminal scrollback setting.
- Terminal minimum column width setting.
- Default shell integration.
- Open terminal for project.
- Pending command injection into terminal.

## File And Attachment Features

- Drag/drop files into chat.
- Clipboard image paste.
- Attachment upload queue.
- Concurrent upload limit.
- Attachment draft persistence.
- Image attachment cards.
- File attachment cards.
- Attachment preview modal.
- MIME type detection.
- Content URL for project files.
- Relative/absolute attachment path metadata.
- Attachment cleanup and failure state.

## Browser And External Open Features

- Open local path in editor.
- Open local path in file manager.
- Open default external target.
- Open browser preview.
- Open file with line/column information.
- List local browser HTTP servers.
- Kill local browser HTTP servers.
- Editor preset settings.
- Custom editor command template.

## Settings Features

- General settings page.
- Theme setting.
- Chat sound preference.
- Chat sound picker.
- Analytics toggle.
- Provider settings.
- Default provider setting.
- Per-provider default model/options.
- LLM provider API settings for quick responses.
- LLM provider validation.
- Keybindings editor.
- Skills management section.
- Changelog section.
- Update check/install controls.
- Terminal preferences.
- Editor preferences.
- Migration from legacy browser localStorage settings.

## Skills Features

- Installed skills snapshot.
- Skill search.
- Skill install.
- Skill uninstall.
- Skill lock file awareness.
- Skills section in settings.

## Update And Release Features

- Update status snapshot.
- Check for updates.
- Install update.
- Restart-pending status.
- Changelog loaded from GitHub releases.
- Release cache with TTL.

## UI And UX Features

- Project-first sidebar.
- Resizable sidebar.
- Collapsible sidebar.
- Mobile sidebar behavior.
- Add project modal.
- Archived chat modal/list.
- Chat navbar.
- Bottom fixed chat composer.
- Right sidebar for git/diff.
- Resizable right sidebar.
- Mobile right sidebar overlay.
- Terminal workspace layout.
- Empty-state typing animation.
- File drag overlay.
- Context-window meter.
- Provider/model/reasoning popovers.
- Rich icons for providers.
- Tooltips and context menus.
- Keyboard shortcuts.
- Sound notifications.
- Responsive layout.

## Sharing And Export Features

- Standalone transcript export.
- Export viewer release asset build.
- Share chat command support.
- Public/share-oriented transcript rendering path.

## Quick Response And Title Generation

- Quick response adapter.
- Configurable LLM provider for helper tasks.
- OpenAI/OpenRouter/custom helper provider options.
- Generate chat title from prompt/context.
- Generate commit message from selected diffs.
- Structured generation helper through Codex temp sessions.

## Security And Safety Features

- Local-first storage.
- Provider command execution remains local.
- Tool approval callbacks for risky operations.
- Approval request handling for command execution.
- Approval request handling for file changes.
- Explicit editor/open-external command routing.
- Per-project local path handling.
- Unsupported tool requests fail closed.

## Testing And Quality Features

- Unit tests for shared protocol/state helpers.
- Tests for chat input behavior.
- Tests for chat preference controls.
- Tests for Git panel helpers.
- Tests for transcript parsing.
- Tests for sidebar/right-sidebar behavior.
- Tests for event-store/read-model behavior.
- Tests for provider catalog behavior.
- Tests for Codex app-server adapter behavior.
- Tests for settings and keybindings behavior.
- Build checks for client and export viewer.

## Key Difference From Current ai-agent-manager

- Kanna owns the chat runtime; current `ai-agent-manager` mostly observes existing transcripts.
- Kanna persists domain events; current `ai-agent-manager` persists session metadata.
- Kanna streams live provider events; current `ai-agent-manager` mostly reloads parsed transcript data.
- Kanna has an agent coordinator; current `ai-agent-manager` has a thin Codex turn endpoint.
- Kanna has provider-specific model/options abstractions; current `ai-agent-manager` has simpler model inputs.
- Kanna has tool approval loops; current `ai-agent-manager` rejects or cannot handle most tool requests.
- Kanna has project/git/terminal workflows; current `ai-agent-manager` is primarily session/message viewing.

