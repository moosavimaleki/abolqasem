# Kanna Parity Map

This file tracks what must be copied, ported, deferred, or excluded while porting Kanna to `ai-agent-manager` with a Go backend.

Status legend:

- `[todo]`: Not started.
- `[in-progress]`: Work started.
- `[done]`: Completed.
- `[defer]`: Required for final parity, intentionally later.
- `[n/a]`: Not applicable to the Go port.

## Port Rules

- Copy Kanna frontend structure and behavior first.
- Do not redesign UX or invent product flows.
- Preserve Kanna WebSocket protocol shapes where possible.
- Port backend behavior to Go.
- Add only required changes for Go backend, branding, Persian i18n, and RTL.

## Server Modules

| Kanna module | Go target | Status | Notes |
| --- | --- | --- | --- |
| `agent.ts` | `internal/workspace/agent_coordinator` | `[todo]` | Central runtime owner. |
| `codex-app-server.ts` | `internal/providers/codex` | `[todo]` | Persistent Codex app-server sessions. |
| `codex-app-server-protocol.ts` | `internal/providers/codex/protocol` | `[todo]` | JSON-RPC DTOs and event normalization. |
| `provider-catalog.ts` | `internal/providers/catalog` | `[todo]` | Only `claude` and `codex` for Kanna parity. |
| `event-store.ts` | `internal/workspace/eventstore` | `[in-progress]` | JSONL append/replay implemented; snapshot compaction remains. |
| `events.ts` | `internal/workspace/events` | `[done]` | Kanna v2 top-level event shape preserved. |
| `read-models.ts` | `internal/workspace/readmodels` | `[in-progress]` | Sidebar model implemented; chat/local project models remain. |
| `ws-router.ts` | `internal/workspace/ws` | `[in-progress]` | Kanna envelope-compatible WS endpoint and initial snapshots implemented. |
| `diff-store.ts` | `internal/git` | `[todo]` | Diff, branch, commit, GitHub workflows. |
| `terminal-manager.ts` | `internal/terminal` | `[todo]` | PTY/process management. |
| `uploads.ts` | `internal/uploads` | `[todo]` | Attachments and uploaded files. |
| `external-open.ts` | `internal/externalopen` | `[todo]` | Finder/editor/terminal/preview/default open. |
| `local-http-servers.ts` | `internal/browserpreview` | `[todo]` | BrowserPanel local server discovery/kill. |
| `project-quick-actions.ts` | `internal/quickactions` | `[todo]` | Per-project quick actions. |
| `quick-response.ts` | `internal/llm/quickresponse` | `[todo]` | Helper LLM responses. |
| `generate-title.ts` | `internal/llm/title` | `[todo]` | Chat title generation. |
| `generate-commit-message.ts` | `internal/llm/commitmsg` | `[todo]` | Diff commit message generation. |
| `llm-provider.ts` | `internal/llm/settings` | `[todo]` | OpenAI/OpenRouter/custom helper settings. |
| `app-settings.ts` | `internal/settings` | `[todo]` | App settings snapshot/patch. |
| `keybindings.ts` | `internal/keybindings` | `[todo]` | Global keybindings. |
| `share.ts` | `internal/share` | `[todo]` | Share/export helper behavior. |
| `standalone-export.ts` | `internal/export` | `[todo]` | Standalone transcript export. |
| `update-manager.ts` | `internal/update` | `[todo]` | Update check/install contract. |
| `auth.ts` | `internal/auth` | `[todo]` | Port only Kanna behavior, no new auth product. |
| `analytics.ts` | `internal/analytics` | `[todo]` | Keep Kanna-compatible setting/shape. |
| `machine-name.ts` | `internal/machine` | `[todo]` | Machine label where needed. |
| `paths.ts` | `internal/paths` | `[todo]` | Kanna data path equivalents in Go. |
| `process-utils.ts` | `internal/processutil` | `[todo]` | Process helpers. |
| `restart.ts` | `internal/restart` | `[todo]` | Restart behavior. |
| `cli-runtime.ts` | `internal/cliruntime` | `[todo]` | CLI runtime detection/metadata. |
| `cli-supervisor.ts` | `internal/clisupervisor` | `[todo]` | CLI supervision if required by update/restart. |
| `discovery.ts` | `internal/discovery` | `[todo]` | Local project discovery. |
| `server.ts` | `internal/server` | `[todo]` | Go HTTP/static/WS server. |
| `cli.ts` | `cmd/ai-agent-manager` | `[todo]` | Existing Go CLI should be extended, not replaced blindly. |

## Shared Modules

| Kanna module | Go/TS target | Status | Notes |
| --- | --- | --- | --- |
| `protocol.ts` | Go DTOs + frontend shared TS | `[done]` | Envelope, command, topic, snapshot constants ported to Go. |
| `types.ts` | Go DTOs + frontend shared TS | `[in-progress]` | Locale setting added; remaining snapshot/message DTOs are still copied TS-side and being ported Go-side. |
| `branding.ts` | frontend shared + Go constants | `[todo]` | Rename branding only where product requires. |
| `ports.ts` | Go config/constants | `[todo]` | Match app server port policy. |
| `dev-ports.ts` | frontend/dev config | `[todo]` | Needed only in dev. |
| `analytics.ts` | frontend shared + Go analytics | `[todo]` | Keep Kanna event shapes if enabled. |
| `share.ts` | frontend shared + Go export/share | `[todo]` | Standalone export data shapes. |
| `tools.ts` | frontend shared + Go tool DTOs | `[todo]` | Tool render/approval compatibility. |

## Frontend App Modules

Copy status:

- `[done]` Kanna frontend source copied to `web-react/src/client`, `web-react/src/shared`, and `web-react/src/export-viewer`.
- `[done]` Kanna public assets copied to `web-react/public`.
- `[done]` Kanna frontend build config copied and adjusted for npm/Vite.
- `[done]` i18n/RTL foundation added.
- `[in-progress]` Go WebSocket integration has initial Kanna-compatible endpoint and snapshots.

| Kanna path | Status | Notes |
| --- | --- | --- |
| `app/App.tsx` | `[todo]` | Copy with routing updates only if needed. |
| `app/ChatPage/index.tsx` | `[todo]` | Main workspace. |
| `app/ChatPage/ChatInputDock.tsx` | `[todo]` | Copy. |
| `app/ChatPage/ChatTranscriptViewport.tsx` | `[todo]` | Copy. |
| `app/ChatPage/TerminalWorkspaceShell.tsx` | `[todo]` | Copy. |
| `app/ChatPage/useChatPageSidebarActions.ts` | `[todo]` | Copy, adapt socket actions. |
| `app/KannaSidebar.tsx` | `[todo]` | Copy, add i18n/RTL. |
| `app/KannaTranscript.tsx` | `[todo]` | Copy. |
| `app/LocalProjectsPage.tsx` | `[todo]` | Copy. |
| `app/PageHeader.tsx` | `[todo]` | Copy. |
| `app/SettingsPage.tsx` | `[todo]` | Copy, add locale settings. |
| `app/socket.ts` | `[todo]` | Keep protocol shape; point to Go WS. |
| `app/useKannaState.ts` | `[todo]` | Copy, adapt backend calls only where unavoidable. |
| `components/NewProjectModal.tsx` | `[todo]` | Copy. |
| `components/LocalDev.tsx` | `[todo]` | Copy if still useful. |
| `components/chat-ui/*` | `[todo]` | Copy. |
| `components/chat-ui/sidebar/*` | `[todo]` | Copy. |
| `components/messages/*` | `[todo]` | Copy. |
| `components/ui/*` | `[todo]` | Copy. |
| `components/open-external-menu.tsx` | `[todo]` | Copy. |
| `components/editor-icons.tsx` | `[todo]` | Copy. |
| `hooks/*` | `[todo]` | Copy. |
| `lib/*` | `[todo]` | Copy. |
| `stores/*` | `[todo]` | Copy. |

## Required Frontend Changes

| Change | Status | Notes |
| --- | --- | --- |
| Add i18n dictionaries: `en`, `fa` | `[done]` | No UX redesign. |
| Add locale setting | `[done]` | Integrated into Kanna settings UI. |
| Apply `dir=rtl` for Persian | `[done]` | Document direction follows locale. |
| Keep code/diff/terminal/path LTR | `[in-progress]` | Existing Kanna components preserve LTR-oriented rendering; full audit remains. |
| Brand text update to `ai-agent-manager` | `[todo]` | Do not disturb layout. |

## Compatibility Notes

- Existing legacy viewer/session discovery remains compatibility-only.
- Gemini remains legacy-viewer-only unless Kanna upstream adds Gemini provider support.
- No new remote auth or multi-user product flow is part of Kanna parity.
