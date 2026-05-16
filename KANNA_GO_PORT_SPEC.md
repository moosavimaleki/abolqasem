# Kanna Parity Port To Go Backend

این سند task specification برای port کردن Kanna روی پروژه `ai-agent-manager` است.

هدف این سند ساخت محصول جدید یا الهام‌گرفتن آزاد از Kanna نیست. هدف این است که Kanna تا حد ممکن با همان ظاهر، همان رفتار، همان UX و همان feature set منتقل شود؛ فقط backend به Go تبدیل شود و frontend دو زبانه و RTL شود.

فرض‌ها:

- Frontend ری‌اکتی Kanna باید تا حد ممکن مستقیم منتقل شود.
- Frontend باید bilingual شود: `fa` و `en`.
- زبان فارسی باید RTL کامل داشته باشد.
- Backend باید Go باشد، نه Bun/TypeScript.
- feature parity با Kanna مهم‌تر از طراحی feature جدید است.
- deviation از Kanna فقط وقتی مجاز است که برای Go idiomatic بودن، امنیت local server یا RTL/i18n لازم باشد.
- Gemini، hook behavior اختصاصی فعلی و viewer legacy نباید وارد core Kanna-port شوند مگر برای migration/compatibility.

## قانون‌های Port

### Rule 1: Copy First, Redesign Last

- frontend componentها، layout، interaction و naming تا حد ممکن از Kanna کپی شوند.
- backend behavior باید از Kanna mirror شود، نه اینکه feature جدید طراحی شود.
- اگر در Kanna command یا snapshot خاصی هست، در Go همان shape حفظ شود مگر دلیل جدی وجود داشته باشد.

### Rule 2: Protocol Parity

Go backend باید envelopeهای Kanna را حفظ کند:

```ts
type ClientEnvelope =
  | { v: 1; type: "subscribe"; id: string; topic: SubscriptionTopic }
  | { v: 1; type: "unsubscribe"; id: string }
  | { v: 1; type: "command"; id: string; command: ClientCommand }

type ServerEnvelope =
  | { v: 1; type: "snapshot"; id: string; snapshot: ServerSnapshot }
  | { v: 1; type: "event"; id: string; event: TerminalEvent }
  | { v: 1; type: "ack"; id: string; result?: unknown }
  | { v: 1; type: "error"; id?: string; message: string }
```

نباید یک envelope جدید مثل `{type, data}` اختراع شود مگر frontend Kanna هم به همان تغییر منتقل شود. اولویت با کمترین تغییر در frontend است.

### Rule 3: No Extra Product Scope

موارد زیر در core parity scope نیستند مگر اینکه Kanna خودش داشته باشد:

- Gemini send/control.
- mode جدید اختصاصی برای viewer/web-ui.
- auth جدید برای remote usage.
- product flow جدید که در Kanna نیست.
- abstractionهای بیش از حد عمومی برای آینده نامعلوم.

### Rule 4: Go Idiomatic, Kanna Compatible

- ساختار packageهای Go می‌تواند idiomatic باشد.
- JSON shape، command names، snapshot names و UI expectations باید Kanna-compatible بمانند.
- تست‌ها باید behavior Kanna را lock کنند.

## هدف معماری

معماری هدف با parity نسبت به Kanna:

```text
React Client
  |
  | WebSocket command/subscription
  | REST for file/upload/static/fallback APIs
  v
Go Server
  |
  +-- WS Router
  +-- Event Store
  +-- Read Models
  +-- Agent Coordinator
  +-- Provider Catalog
  +-- Codex Adapter
  +-- Claude Adapter
  +-- Git Service
  +-- Terminal Manager
  +-- Settings Service
  +-- Keybindings Service
  +-- LLM Provider Service
  +-- Skills Service
  +-- Update Manager
  +-- Share / Standalone Export
  +-- Browser Local Server Discovery
  +-- Quick Actions
  +-- External Open Service
  +-- File/Upload Service
  +-- Legacy Viewer Bridge
  |
  v
Local tools
  +-- codex app-server
  +-- claude
  +-- git
  +-- shell
  +-- filesystem
```

## Milestone 0: Parity Lock

### Task 0.1: تهیه parity map از Kanna `[done]`

شرح:

- تمام فایل‌های `src/shared`, `src/server`, `src/client` در Kanna map شوند.
- برای هر فایل Kanna مشخص شود:
  - `copy frontend`
  - `port to Go`
  - `keep as frontend utility`
  - `defer but required for final parity`
  - `not applicable because backend is Go`

Acceptance criteria:

- یک جدول parity در همین repo ثبت شود.
- هیچ فیچر Kanna بدون تصمیم explicit حذف نشود.
- هر defer باید دلیل و phase داشته باشد.

Output:

- [KANNA_PARITY_MAP.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/KANNA_PARITY_MAP.md)

### Task 0.1.1: Server module port map `[done]`

این map باید هنگام اجرا به checklist واقعی تبدیل شود. معادل‌های پیشنهادی Go فقط naming هستند؛ behavior باید از Kanna بیاید.

```text
Kanna server module                  Go target
---------------------------------------------------------------------------
agent.ts                             internal/workspace/agent_coordinator
codex-app-server.ts                  internal/providers/codex
codex-app-server-protocol.ts         internal/providers/codex/protocol
provider-catalog.ts                  internal/providers/catalog
event-store.ts                       internal/workspace/eventstore
events.ts                            internal/workspace/events
read-models.ts                       internal/workspace/readmodels
ws-router.ts                         internal/workspace/ws
diff-store.ts                        internal/git
terminal-manager.ts                  internal/terminal
uploads.ts                           internal/uploads
external-open.ts                     internal/externalopen
local-http-servers.ts                internal/browserpreview
project-quick-actions.ts             internal/quickactions
quick-response.ts                    internal/llm/quickresponse
generate-title.ts                    internal/llm/title
generate-commit-message.ts           internal/llm/commitmsg
llm-provider.ts                      internal/llm/settings
app-settings.ts                      internal/settings
keybindings.ts                       internal/keybindings
share.ts                             internal/share
standalone-export.ts                 internal/export
update-manager.ts                    internal/update
auth.ts                              internal/auth
analytics.ts                         internal/analytics
machine-name.ts                      internal/machine
paths.ts                             internal/paths
process-utils.ts                     internal/processutil
restart.ts                           internal/restart
cli-runtime.ts                       internal/cliruntime
cli-supervisor.ts                    internal/clisupervisor
discovery.ts                         internal/discovery
server.ts                            internal/server
cli.ts                               cmd/ai-agent-manager
```

Acceptance criteria:

- برای هر module بالا یک Go implementation، explicit defer، یا explicit not-applicable note وجود داشته باشد.
- testهای متناظر Kanna تا حد ممکن به Go test تبدیل شوند.
- هیچ module بدون ثبت تصمیم حذف نشود.

Output:

- [KANNA_PARITY_MAP.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/KANNA_PARITY_MAP.md)

### Task 0.1.2: Shared type parity map `[done]`

شرح:

- `src/shared/types.ts` و `src/shared/protocol.ts` باید به Go DTOها و TypeScript shared types جدید منتقل شوند.
- frontend نباید مجبور شود shapeهای متفاوت مصرف کند.

Acceptance criteria:

- `AgentProvider` فقط `claude | codex` باشد.
- `ClientCommand`ها با Kanna برابر باشند.
- `SubscriptionTopic`ها با Kanna برابر باشند.
- `ServerSnapshot`ها با Kanna برابر باشند.
- `TranscriptEntry`, `ChatSnapshot`, `SidebarData`, `ChatDiffSnapshot`, `TerminalSnapshot`, `AppSettingsSnapshot`, `KeybindingsSnapshot`, `UpdateSnapshot`, `LlmProviderSnapshot` معادل داشته باشند.

Output:

- [KANNA_PARITY_MAP.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/KANNA_PARITY_MAP.md)

### Task 0.1.3: Frontend component port map `[done]`

Componentهای کلیدی که باید با حداقل تغییر منتقل شوند:

```text
App / routing
KannaSidebar
LocalProjectsPage
ChatPage
ChatNavbar
ChatInputDock
ChatInput
ChatPreferenceControls
ChatTranscriptViewport
KannaTranscript
messages/*
GitPanel
BrowserPanel
TerminalWorkspaceShell
TerminalWorkspace
TerminalPane
SettingsPage
StandaloneShareDialog
NewProjectModal
LocalDev
ui/*
stores/*
lib/*
```

Acceptance criteria:

- visual/component hierarchy با Kanna یکی بماند.
- فقط i18n/RTL و backend connection تغییر کند.
- testهای frontend Kanna تا حد ممکن حفظ شوند.

Output:

- [KANNA_PARITY_MAP.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/KANNA_PARITY_MAP.md)

### Task 0.2: بررسی license و اجازه copy مستقیم `[done]`

شرح:

- license پروژه Kanna و dependencyهای آن بررسی شود.
- اگر copy مستقیم frontend مجاز نیست، port باید component-by-component بازنویسی شود.

Acceptance criteria:

- یک note در repo ثبت شود که استفاده مستقیم از code مجاز است یا نه.
- dependencyهای frontend در `package.json` جدید مشخص باشند.

Output:

- [KANNA_LICENSE_REVIEW.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/KANNA_LICENSE_REVIEW.md)

Dependency/package metadata:

- Done. Dependency/package metadata در [web-react/package.json](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/package.json) ثبت شد.

### Task 0.3: تعیین strategy انتقال frontend بدون redesign `[done]`

شرح:

- ساختار React Kanna به مسیر frontend جدید منتقل شود.
- تغییرات فقط برای اتصال به Go backend، branding، i18n و RTL باشد.
- CSS/spacing/component hierarchy تا حد ممکن حفظ شود.

Acceptance criteria:

- UI visually نزدیک به Kanna باشد.
- snapshot/screenshot baseline از صفحات Kanna برای مقایسه گرفته شود.
- هیچ layout جدیدی بدون دلیل اضافه نشود.

Output:

- Frontend Kanna copied to [web-react](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react).
- No redesign was introduced.

### Task 0.4: انتخاب build tool با حداقل تغییر `[done]`

شرح:

- اگر Kanna با Vite/Bun است، frontend build می‌تواند همان Vite را نگه دارد.
- backend Go فقط static output را embed/serve کند.

Acceptance criteria:

- frontend مستقل build شود.
- Go embed خروجی `dist` را serve کند.
- CI بدون Bun server backend کار کند.

Output:

- `npm install` completed.
- `npm run build:client` completed.
- `npm run build:export-viewer` completed.
- Bun server backend was not copied into `web-react`.

## Milestone 1: Frontend Port Skeleton

### Task 1.1: انتقال ساختار React app `[done]`

مسیر پیشنهادی:

```text
web-react/
  package.json
  vite.config.ts
  tsconfig.json
  src/
    app/
    components/
    stores/
    lib/
    i18n/
    styles/
```

Acceptance criteria:

- app روی dev server بالا بیاید.
- production build ساخته شود.
- Go server بتواند build خروجی را serve کند.
- structure کلی Kanna حفظ شده باشد.

Output:

- [web-react/package.json](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/package.json)
- [web-react/src](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src)
- [web-react/public](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/public)

### Task 1.2: انتقال کامل shell UI Kanna

شرح:

- layout اصلی Kanna منتقل شود:
  - sidebar
  - chat page
  - chat navbar
  - composer dock
  - right sidebar
  - settings page
  - local projects page
  - terminal workspace
  - browser panel
  - standalone share dialog

Acceptance criteria:

- UI بدون backend واقعی render شود.
- empty states درست دیده شوند.
- responsive layout خراب نباشد.
- ظاهر باید با Kanna قابل مقایسه باشد، نه طراحی جدید.

### Task 1.3: i18n foundation `[done]`

شرح:

- اضافه کردن سیستم translation.
- زبان‌های اولیه:
  - `en`
  - `fa`
- locale از settings خوانده شود.
- fallback به `en`.

مسیر پیشنهادی:

```text
web-react/src/i18n/
  index.ts
  en.ts
  fa.ts
```

Acceptance criteria:

- UI بتواند بین فارسی و انگلیسی switch کند.
- متن‌های hardcoded اصلی حذف شوند.
- direction بر اساس locale تنظیم شود.

Output:

- [web-react/src/client/i18n/index.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/i18n/index.ts)
- [web-react/src/client/i18n/en.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/i18n/en.ts)
- [web-react/src/client/i18n/fa.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/i18n/fa.ts)
- Settings language selector added.
- `AppSettingsSnapshot.locale` and `AppSettingsPatch.locale` added.

### Task 1.4: RTL foundation `[done]`

شرح:

- وقتی locale فارسی است:
  - `dir="rtl"`
  - alignment مناسب در sidebar/chat/navbar/settings
  - composer و markdown با mixed direction درست کار کند.
- کد، diff، terminal و pathها باید LTR بمانند.

Acceptance criteria:

- فارسی RTL است.
- code blockها LTR هستند.
- terminal LTR است.
- git diff LTR است.
- مسیر فایل‌ها LTR هستند.

Output:

- Document `lang` and `dir` now follow `appSettings.locale`.
- Existing Kanna code/diff/terminal/path components remain copied and keep their own LTR-oriented rendering.

## Milestone 2: Go WebSocket Protocol

### Task 2.1: Port دقیق protocol.ts به Go `[done]`

شرح:

- معادل TypeScript protocol در Go تعریف شود.
- shapeها باید با Kanna یکی بمانند.

مدل envelope باید معادل این باشد:

```go
type ClientEnvelope struct {
    V       int             `json:"v"`
    Type    string          `json:"type"`
    ID      string          `json:"id"`
    Topic   *SubscriptionTopic `json:"topic,omitempty"`
    Command json.RawMessage `json:"command,omitempty"`
}

type ServerEnvelope struct {
    V        int             `json:"v"`
    Type     string          `json:"type"`
    ID       string          `json:"id,omitempty"`
    Snapshot any             `json:"snapshot,omitempty"`
    Event    any             `json:"event,omitempty"`
    Result   any             `json:"result,omitempty"`
    Message  string          `json:"message,omitempty"`
}
```

Acceptance criteria:

- WebSocket endpoint مثلاً `/api/ws` ایجاد شود.
- client بتواند connect/disconnect کند.
- ping/pong یا heartbeat وجود داشته باشد.
- command نامعتبر error استاندارد برگرداند.
- frontend Kanna socket client با حداقل تغییر به Go وصل شود.

Output:

- [protocol.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/protocol/protocol.go)
- [protocol_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/protocol/protocol_test.go)
- Kanna-compatible envelope DTOs, topic constants, command constants, snapshot/event/ack/error helpers.

Remaining for Milestone 2:

- Actual `/api/ws` handler and subscription routing are Task 2.2/2.3.

### Task 2.2: Subscription model `[in-progress]`

Topicهای اولیه:

```text
sidebar
local-projects
update
keybindings
app-settings
chat
project-git
terminal
```

Acceptance criteria:

- client بتواند subscribe/unsubscribe کند.
- snapshot اولیه بعد از subscribe ارسال شود.
- تغییر state فقط به subscriberهای مرتبط broadcast شود.

Output:

- `/ws` endpoint added.
- Initial snapshot responses implemented for `sidebar`, `local-projects`, `update`, `keybindings`, `app-settings`, `chat`, `project-git`, and `terminal`.
- `/auth/status` added so copied Kanna UI can pass auth check.

Remaining:

- Persistent subscription registry.
- Topic-specific broadcasts after state changes.

### Task 2.3: Command router `[in-progress]`

Commandهای Kanna باید mirror شوند:

```text
project.open
project.create
project.rename
project.remove
sidebar.reorderProjectGroups
project.readDiffPatch
system.ping
browser.listLocalHttpServers
browser.killLocalHttpServer
project.readQuickActions
project.writeQuickActions
update.check
update.install
settings.readKeybindings
settings.writeKeybindings
settings.readAppSettings
settings.writeAppSettings
settings.writeAppSettingsPatch
settings.readLlmProvider
settings.writeLlmProvider
settings.validateLlmProvider
skills.search
skills.install
skills.uninstall
skills.listInstalled
system.openExternal
chat.create
chat.fork
chat.rename
chat.archive
chat.unarchive
chat.delete
chat.setDraftProtection
chat.markRead
chat.send
chat.refreshDiffs
chat.initGit
chat.getGitHubPublishInfo
chat.checkGitHubRepoAvailability
chat.publishToGitHub
chat.listBranches
chat.previewMergeBranch
chat.mergeBranch
chat.syncBranch
chat.checkoutBranch
chat.createBranch
chat.generateCommitMessage
chat.commitDiffs
chat.discardDiffFile
chat.ignoreDiffFile
chat.cancel
chat.stopDraining
chat.exportStandalone
chat.loadHistory
chat.respondTool
message.enqueue
message.steer
message.dequeue
terminal.create
terminal.input
terminal.resize
terminal.close
```

Acceptance criteria:

- هر command handler جدا و قابل تست باشد.
- خطاها structured باشند.
- command ID در response حفظ شود.
- commandهای هنوز پیاده‌سازی‌نشده باید error سازگار بدهند، نه silently ignore.

Output:

- `system.ping` implemented.
- `settings.readAppSettings` implemented.
- `settings.writeAppSettingsPatch` supports `locale` persistence.
- Unimplemented commands return Kanna-compatible error envelopes.

Remaining:

- Dispatch all Kanna commands to real Go services as their milestones are implemented.

## Milestone 3: Event Store در Go

### Task 3.1: طراحی event model `[done]`

Eventهای پایه:

```text
project.opened
project.renamed
project.hidden

chat.created
chat.renamed
chat.deleted
chat.archived
chat.unarchived
chat.provider_set
chat.plan_mode_set
chat.read_state_set

message.appended

queued_message.enqueued
queued_message.removed

turn.started
turn.finished
turn.failed
turn.cancelled
turn.session_token_set
```

Acceptance criteria:

- eventها version داشته باشند.
- timestamp داشته باشند.
- migration path برای version بعدی در نظر گرفته شود.

Output:

- [events.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/events/events.go)
- Kanna `v: 2` event version preserved.
- Kanna-style top-level event fields preserved; domain fields are not wrapped under `data`.
- Project/chat/message/queue/turn event type constants added.

### Task 3.2: JSONL append-only store `[done]`

مسیر پیشنهادی:

```text
~/.cache/ai-agent-manager/data/
  projects.jsonl
  chats.jsonl
  messages.jsonl
  queued-messages.jsonl
  turns.jsonl
  snapshot.json
```

Acceptance criteria:

- append atomic باشد.
- corrupted line کل store را نابود نکند.
- replay از صفر ممکن باشد.
- lock فایل/فرآیند رعایت شود.

Output:

- [store.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/eventstore/store.go)
- [store_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/eventstore/store_test.go)
- Append/replay implemented for Kanna streams: `projects`, `chats`, `messages`, `queued-messages`, `turns`.

### Task 3.3: Snapshot compaction `[done]`

شرح:

- وقتی event log بزرگ شد snapshot ساخته شود.
- startup از snapshot + events بعد از snapshot انجام شود.

Acceptance criteria:

- startup با ۱۰ هزار message کند نشود.
- compact کردن باعث از بین رفتن داده نشود.
- test برای replay و snapshot وجود داشته باشد.

Output:

- `snapshot.json` shape follows Kanna `SnapshotFile`: `v`, `generatedAt`, `projects`, `chats`, `queuedMessages`.
- Compaction writes snapshot atomically with temp+rename, then truncates Kanna event streams.
- Startup path loads snapshot first and replays later JSONL events with Kanna event ordering priority.
- Tests cover snapshot writing, log truncation, and snapshot-plus-event replay.

## Milestone 4: Read Models

### Task 4.1: Sidebar read model `[done]`

خروجی:

```json
{
  "project_groups": [
    {
      "project_id": "...",
      "title": "...",
      "local_path": "...",
      "chats": [],
      "archived_chats": []
    }
  ]
}
```

Acceptance criteria:

- چت‌ها زیر پروژه درست group شوند.
- recent ordering درست باشد.
- archived جدا شود.
- active status روی rowها مشخص باشد.

Output:

- [readmodels.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/readmodels/readmodels.go)
- [readmodels_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/readmodels/readmodels_test.go)
- Sidebar data is derived from Kanna-compatible project/chat events.
- Deleted projects/chats are skipped and archived chats are separated from active chats.

### Task 4.2: Chat snapshot read model `[done]`

خروجی:

```json
{
  "runtime": {},
  "messages": [],
  "queued_messages": [],
  "history": {},
  "available_providers": []
}
```

Acceptance criteria:

- message pagination داشته باشد.
- runtime status از active turn derive شود.
- queued messages برگردند.
- provider catalog به snapshot اضافه شود.

Output:

- `DeriveChatSnapshot` mirrors Kanna `deriveChatSnapshot`.
- Runtime includes chat/project metadata, status, draining state, provider, plan mode, and session token.
- Queued messages are cloned from read model state.
- Transcript messages/history are passed through with Kanna `ChatHistorySnapshot` shape.
- Available providers include the Kanna server provider catalog for `claude` and `codex`.

### Task 4.3: Local projects read model `[done]`

شرح:

- پروژه‌های ذخیره‌شده و پروژه‌های discover شده از legacy sessions merge شوند.

Acceptance criteria:

- پروژه‌های قدیمی از transcript discovery قابل مشاهده باشند.
- پروژه جدید از UI قابل open باشد.
- duplicate path حذف شود.

Output:

- `DeriveLocalProjectsSnapshot` mirrors Kanna saved/discovered merge behavior.
- Saved project metadata wins over discovered project metadata for the same local path.
- Active non-archived chat count and last-opened ordering are derived from read model state.
- Machine snapshot uses Kanna `local` machine shape.

## Milestone 5: Provider Catalog

### Task 5.1: Port مستقیم ProviderCatalog Kanna `[done]`

Providerهای هدف:

```text
claude
codex
```

مدل:

```go
type ProviderCatalogEntry struct {
    ID               string
    Label            string
    DefaultModel     string
    Models           []ModelInfo
    SupportsPlanMode bool
    Capabilities     map[string]bool
}
```

Acceptance criteria:

- UI بتواند provider/model/reasoning را از backend بخواند.
- provider ناشناخته fallback امن داشته باشد.
- provider list با Kanna برابر باشد مگر در migration legacy.

Output:

- [catalog.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/catalog/catalog.go)
- [catalog_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/catalog/catalog_test.go)
- Kanna `claude` and `codex` provider catalog ported.
- Codex server model list matches Kanna hard-coded server catalog.
- Unknown provider/model fallback is safe and deterministic.

### Task 5.2: Model options `[done]`

برای Codex:

```text
model
reasoning_effort
fast_mode
plan_mode
```

برای Claude:

```text
model
reasoning_effort
context_window
plan_mode
```

Acceptance criteria:

- هر provider optionهای خودش را validate کند.
- UI فقط optionهای قابل پشتیبانی را نشان دهد.
- option naming با Kanna compatible باشد.

Output:

- `NormalizeServerModel` mirrors Kanna alias/default behavior.
- `NormalizeClaudeModelOptions` supports reasoning effort and context window fallback.
- `NormalizeCodexModelOptions` supports reasoning effort and fast mode fallback.
- `CodexServiceTierFromModelOptions` maps fast mode to Kanna `fast` service tier.
- `ResolveClaudeAPIModelID` mirrors Kanna `1m` context model suffix behavior.

## Milestone 6: Agent Coordinator

### Task 6.1: ساخت coordinator مرکزی `[in-progress]`

مسئولیت‌ها:

- start turn
- continue turn
- cancel turn
- queue message
- dequeue message
- track active turns
- handle tool requests
- append transcript entries
- update runtime status
- broadcast read model changes

Acceptance criteria:

- همزمان دو turn روی یک chat اجرا نشود.
- اگر chat active است، پیام جدید queue شود.
- cancel status را درست update کند.
- خطای provider باعث خراب شدن server نشود.

Progress:

- Core coordinator package added.
- Active turn map and status derivation added.
- `chat.send` behavior for existing/new chat is modeled.
- Active chat sends are queued instead of starting a second turn.
- Cancel removes active turn immediately and records cancellation.
- Provider start failure records failed turn and clears active state.
- Queue deletion and start-next-queued-message after finish are implemented.

Remaining:

- Provider event stream consumption.
- Tool request/response lifecycle.
- Draining stream behavior.
- Integration with WebSocket command router.

### Task 6.1.1: Coordinator core invariants `[done]`

شرح:

- Kanna coordinator invariants بدون adapter واقعی provider پیاده شود.
- این task نباید Claude/Codex protocol را بسازد؛ فقط core state machine را آماده می‌کند.

Acceptance criteria:

- active turn per chat فقط یکی باشد.
- send روی chat فعال queue شود.
- cancel active را پاک کند و turn را cancel کند.
- provider start error active را پاک کند و failed record بزند.

Output:

- [coordinator.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/agent/coordinator.go)
- [coordinator_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/agent/coordinator_test.go)

### Task 6.2: Active turn model `[done]`

```go
type ActiveTurn struct {
    ChatID       string
    ProjectID    string
    Provider     string
    Model        string
    Status       string
    StartedAt    time.Time
    Cancel       context.CancelFunc
    PendingTools map[string]*PendingToolRequest
}
```

Acceptance criteria:

- statusهای `idle`, `running`, `waiting_for_user`, `failed`, `cancelled` پشتیبانی شوند.
- pending tool request در snapshot دیده شود.

Output:

- `ActiveTurn` now includes `ChatID`, `ProjectID`, provider/model/options, `StartedAt`, cancel context, and pending tool state.
- Active statuses include Kanna statuses plus cancelled support for Go-side state handling.
- Pending tool snapshot exposes `toolUseId` and `toolKind`.
- Cancelling a turn cancels the provider context and removes the active turn immediately.

### Task 6.3: Queue model `[done]`

شرح:

- اگر کاربر هنگام active بودن chat پیام دهد، پیام queue شود.
- بعد از پایان turn، پیام بعدی اجرا شود.

Acceptance criteria:

- queue persisted باشد.
- refresh صفحه queue را از بین نبرد.
- کاربر بتواند queued message را حذف کند.

Output:

- Active chat sends enqueue instead of starting a second turn.
- `Finish` records turn completion and starts the next queued message.
- `Dequeue` removes queued messages by id.
- Queue persistence is delegated to the Kanna-compatible store interface/event store.

## Milestone 7: Codex Adapter در Go

### Task 7.1: بازطراحی Codex app-server client `[done]`

مشکل فعلی:

- برای هر request یک app-server جدید ساخته می‌شود.
- streaming و tool handling کامل نیست.

هدف:

```text
CodexManager
  +-- sessions map[chatID]*CodexSession
  +-- StartThread
  +-- ResumeThread
  +-- ForkThread
  +-- StartTurn
  +-- CancelTurn
  +-- RespondTool
```

Acceptance criteria:

- app-server context برای chat قابل reuse باشد.
- initialize با `experimentalApi` انجام شود.
- thread token/session token ذخیره شود.

Output:

- [manager.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/manager.go)
- [manager_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/manager_test.go)
- [protocol.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/protocol/protocol.go)
- Manager keeps reusable `chatID -> session` state.
- `initialize` sends `experimentalApi: true`.
- `StartSession` chooses `thread/fork`, `thread/resume`, or `thread/start` using Kanna order.
- Recoverable resume errors fall back to `thread/start`.

### Task 7.2: JSON-RPC routing `[done]`

شرح:

- request/response/notificationهای Codex جدا route شوند.
- concurrent pending calls با ID map مدیریت شوند.

Acceptance criteria:

- response اشتباه به call اشتباه وصل نشود.
- notificationها stream شوند.
- stderr در log ذخیره شود.

Output:

- [client.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/rpc/client.go)
- [client_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/rpc/client_test.go)
- Pending calls are tracked by JSON-RPC id.
- Out-of-order responses route to the correct caller.
- Server notifications are exposed through a notification channel.
- stderr lines are retained in a log buffer for turn failure reporting.

### Task 7.3: Codex turn streaming `[done]`

Notificationهای مهم:

```text
thread/started
turn/completed
item/agentMessage/delta
item/reasoning/*
item/tool/*
item/fileChange/*
item/plan/*
```

Acceptance criteria:

- پیام assistant زنده به transcript اضافه شود.
- reasoning/plan/file change قابل نمایش باشد.
- turn completed status درست set شود.

Output:

- [stream.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/stream.go)
- [stream_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/stream_test.go)
- Core Codex notifications map to Kanna harness events.
- `thread/started` emits `session_token`.
- command execution emits `tool_call` and `tool_result`.
- agent messages emit `assistant_text`.
- `turn/completed` emits Kanna `result`.
- token usage emits `context_window_updated`.
- `thread/compacted` emits `compact_boundary`.

Remaining:

- Full reasoning/plan/file-change visual event parity continues under Task 7.4 and later transcript rendering integration.

### Task 7.4: Codex tool and approval handling `[done]`

Server requestهای مهم:

```text
item/tool/requestUserInput
item/commandExecution/requestApproval
item/fileChange/requestApproval
```

Acceptance criteria:

- tool request به UI ارسال شود.
- UI بتواند approve/deny/respond کند.
- Codex request با پاسخ UI resume شود.

Output:

- [approvals.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/approvals.go)
- [approvals_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/codex/approvals_test.go)
- `item/tool/requestUserInput` maps to Kanna `ask_user_question` tool call and returns Codex answer shape.
- `item/commandExecution/requestApproval` returns approval decision.
- `item/fileChange/requestApproval` returns approval decision and defaults to `decline`.
- Request handling is isolated so the JSON-RPC process bridge can call it directly.

## Milestone 8: Claude Adapter

### Task 8.1: Port رفتار Claude adapter Kanna به Go `[done]`

اصل:

- هدف کپی رفتار Kanna است.
- استفاده از bridge جاوااسکریپتی دائمی نباید default باشد چون backend باید Go شود.
- اگر Claude SDK فقط در JS قابل استفاده بود، bridge موقت فقط با تصمیم explicit مجاز است.

گزینه‌های بررسی:

- استفاده مستقیم از `claude` CLI اگر protocol کافی دارد.
- استفاده از SDK معادل Go اگر موجود/مناسب باشد.
- bridge process کوچک فقط اگر بدون آن parity ممکن نبود.

Acceptance criteria:

- تصمیم فنی documented شود.
- proof of concept یک prompt ساده را stream کند.

Output:

- [CLAUDE_ADAPTER_DECISION.md](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/CLAUDE_ADAPTER_DECISION.md)
- [adapter.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/claude/adapter.go)
- [adapter_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/claude/adapter_test.go)
- The Go port uses the local `claude` CLI stream-json mode instead of a permanent JS SDK bridge.
- Command construction supports model, effort, permission mode, resume, and fork-session.
- Stream parser maps assistant/result JSON lines into Kanna transcript entries.

### Task 8.2: Claude session management `[done]`

Acceptance criteria:

- start/resume session.
- stream messages.
- model/reasoning/context window support.
- AskUserQuestion و plan approval پشتیبانی شود، اگر provider اجازه بدهد.

Output:

- [session.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/claude/session.go)
- [session_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/providers/claude/session_test.go)
- Session manager reuses existing Claude session for matching cwd/effort.
- Model and permission mode updates are applied to reusable sessions.
- Fork session or cwd/effort changes close the old session and start a new one.
- Prompt sending is routed through the reusable session handle.

Remaining:

- Full AskUserQuestion/ExitPlanMode permission bridge depends on CLI stream/control behavior and will be wired when provider lifecycle is integrated into coordinator.

## Milestone 9: Gemini Adapter

### Task 9.1: Legacy Gemini viewer compatibility only `[done]`

Acceptance criteria:

- اگر sessionهای Gemini فعلی وجود دارند، در legacy viewer از بین نروند.
- Gemini وارد provider catalog Kanna-port نشود مگر Kanna upstream اضافه کند.

Output:

- Existing legacy Gemini parser/discovery/hook code remains untouched.
- Provider catalog test locks Kanna parity providers to exclude Gemini.
- Gemini remains available only through legacy viewer compatibility.

### Task 9.2: No Gemini Web UI control in Kanna parity scope `[done]`

Acceptance criteria:

- هیچ UI control جدید برای Gemini ساخته نشود.
- هیچ مدل/option اختصاصی Gemini در Kanna workspace اضافه نشود.
- اگر بعداً نیاز شد، task جدا خارج از parity scope تعریف شود.

Output:

- No Gemini provider entry was added to Kanna Web UI provider catalog.
- No Gemini model/options were added to Kanna workspace scope.

## Milestone 10: Transcript Model

### Task 10.1: تعریف TranscriptEntry مشترک `[done]`

انواع:

```text
user_prompt
assistant_message
reasoning
tool_call
tool_result
command
file_change
plan
error
system
attachment
```

Acceptance criteria:

- همه providerها به این مدل normalize شوند.
- UI provider-specific raw payload نخواهد.

Output:

- [model.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/transcript/model.go)
- [model_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/transcript/model_test.go)
- Go-side transcript entry shape now has shared `kind`, `_id`, and `createdAt`.
- Codex and Claude adapters normalize provider events into the shared transcript model.
- Read models alias the shared transcript type so chat snapshots keep Kanna-compatible JSON.

### Task 10.2: Legacy transcript import `[done]`

شرح:

- sessionهای قدیمی از parser فعلی import شوند.
- به عنوان read-only chat یا imported chat دیده شوند.

Acceptance criteria:

- sessionهای قبلی از بین نروند.
- duplicate sessionها کنترل شوند.
- user بتواند session قدیمی را open کند.

Output:

- [importer.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/legacyimport/importer.go)
- [importer_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/legacyimport/importer_test.go)
- Legacy `state.SessionMeta` plus parser messages are converted into Kanna-compatible project/chat/transcript snapshots.
- Legacy imported chats are marked read-only at the import boundary.
- Legacy chat IDs are deterministic from agent plus transcript path, so duplicate discovered sessions collapse to the same imported chat identity.
- Recent-limit pagination metadata is produced for large legacy transcripts.

## Milestone 11: Tool Approval UI

### Task 11.1: Pending tool cards `[done]`

Frontend:

- question card
- approval card
- approve/deny buttons
- optional text input

Backend:

- pending tool state
- `chat.respondTool`

Acceptance criteria:

- agent هنگام approval معلق بماند.
- user response به provider برگردد.
- timeout/error قابل نمایش باشد.

Output:

- [coordinator.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/agent/coordinator.go)
- [coordinator_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/agent/coordinator_test.go)
- Kanna frontend already includes `AskUserQuestionMessage`, `ExitPlanModeMessage`, and `chat.respondTool` command dispatch.
- Go coordinator now stores pending tool state, exposes pending snapshots, forwards `RespondTool` to the active turn, clears pending state, and returns the chat to `running`.
- Wrong or stale tool IDs are rejected without clearing the real pending tool.

## Milestone 12: Git Service در Go

### Task 12.1: Git repository detection `[done]`

Acceptance criteria:

- project root git تشخیص داده شود.
- non-git project status مشخص داشته باشد.

Output:

- [detect.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/detect.go)
- [detect_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/detect_test.go)
- Go git service now detects repository root from nested project paths.
- Non-git directories return Kanna-compatible `status: "no_repo"` with empty files.
- Ready repositories return branch, origin remote, GitHub slug, upstream metadata, and `files: []` for Task 12.2.

### Task 12.2: Diff snapshot `[done]`

خروجی:

```json
{
  "status": "ready",
  "branch_name": "main",
  "files": [
    {
      "path": "...",
      "change_type": "modified",
      "additions": 10,
      "deletions": 2
    }
  ]
}
```

Acceptance criteria:

- untracked/modified/deleted/renamed تشخیص داده شود.
- patch lazy load شود.

Output:

- [detect.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/detect.go)
- [detect_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/detect_test.go)
- Git snapshots now populate Kanna-compatible `files` with `added`, `modified`, `deleted`, and `renamed` entries.
- Untracked files are marked with `isUntracked: true`.
- Additions/deletions are read from `git diff --numstat`; full patches remain lazy and are not embedded in the snapshot.
- Each file receives a stable `patchDigest` for frontend cache invalidation.

### Task 12.3: Commit workflow `[done]`

Acceptance criteria:

- selected files commit شوند.
- commit message دستی.
- generate commit message با helper LLM بعداً.
- push/pull/fetch پشتیبانی شود.

Output:

- [commit.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/commit.go)
- [commit_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/commit_test.go)
- Selected-file commits use explicit pathspecs and manual summary/description.
- `commit_only` and `commit_and_push` return Kanna-compatible success/failure result shapes.
- Push creates upstream with `git push -u origin <branch>` when no upstream exists.
- `fetch`, `pull --ff-only`, and `push` sync actions are available.
- Commit message generation remains intentionally deferred, matching the original task note.

### Task 12.4: Branch workflow `[done]`

Acceptance criteria:

- list branches.
- checkout.
- create branch.
- merge preview.
- merge.

Output:

- [branch.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/branch.go)
- [branch_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/branch_test.go)
- Local and remote branch lists return Kanna-compatible branch rows.
- Checkout and create branch commands return Kanna-compatible action results.
- Merge preview reports `up_to_date`, `mergeable`, `conflicts`, or `error`.
- Merge uses `git merge --no-edit` and returns snapshot-change metadata.

### Task 12.5: GitHub publish workflow مطابق Kanna `[done]`

Acceptance criteria:

- `chat.getGitHubPublishInfo` پیاده شود.
- `chat.checkGitHubRepoAvailability` پیاده شود.
- `chat.publishToGitHub` پیاده شود.
- response shape با Kanna برابر باشد.

Output:

- [github.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/github.go)
- [github_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/github_test.go)
- Go service now reports whether `gh` is installed and authenticated.
- Active GitHub login and org owners are returned when available.
- Suggested repository name is derived from the project/repository folder.
- Repo availability uses `gh repo view` and returns Kanna-compatible `available/message`.
- Publish uses `gh repo create` plus a real branch push through the Git service.

### Task 12.6: Discard/ignore workflow مطابق Kanna `[done]`

Acceptance criteria:

- `chat.discardDiffFile` پیاده شود.
- `chat.ignoreDiffFile` پیاده شود.
- ignore file/folder behavior با UI Kanna سازگار باشد.

Output:

- [discard.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/discard.go)
- [discard_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/gitservice/discard_test.go)
- Tracked files are restored with `git restore --staged --worktree`.
- Untracked files are removed only after safe in-repository path validation.
- Ignore file and ignore folder append normalized entries to `.gitignore` without duplicating existing patterns.

## Milestone 13: Terminal Manager در Go

### Task 13.1: PTY integration `[done]`

نیازمندی:

- Unix: pty package.
- Windows: ConPTY یا fallback محدود.

Acceptance criteria:

- terminal create/input/resize/close کار کند.
- output با WebSocket stream شود.
- process cleanup درست باشد.

Output:

- [manager.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/terminal/manager.go)
- [process_pty.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/terminal/process_pty.go)
- [process_windows.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/terminal/process_windows.go)
- [manager_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/terminal/manager_test.go)
- Unix terminals use `github.com/creack/pty` with real resize support.
- Windows builds use a limited pipe fallback so release builds stay portable.
- Manager supports create, input, resize, close, output events, exit events, serialized replay state, and process cleanup.

### Task 13.2: Terminal UI integration `[done]`

Acceptance criteria:

- xterm.js frontend وصل شود.
- multi terminal per project.
- terminal layout persisted شود.

Output:

- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Existing copied Kanna React UI already includes xterm.js panes, terminal layout store, multi-terminal UI, and persisted layout state.
- Go WebSocket backend now handles `terminal.create`, `terminal.input`, `terminal.resize`, and `terminal.close`.
- Terminal subscriptions now return snapshots and stream `terminal.output` / `terminal.exit` events to the matching subscription id.

## Milestone 14: File, Upload, Preview

### Task 14.1: Upload service `[done]`

Acceptance criteria:

- فایل‌ها در مسیر امن cache شوند.
- attachment metadata ذخیره شود.
- size limit و mime detection داشته باشد.

Output:

- [upload_api.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/upload_api.go)
- [upload_api_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/upload_api_test.go)
- `POST /api/projects/{projectId}/uploads` stores uploads under the application cache directory.
- Attachment metadata is persisted as a sidecar JSON file.
- Uploads enforce a 25 MiB limit and detect MIME from headers/content/extension.
- Uploaded content can be previewed through `.../content` and deleted through the attachment URL without `/content`.

### Task 14.2: Project file serving `[done]`

Acceptance criteria:

- فقط مسیرهای زیر project root قابل serve باشند.
- path traversal غیرممکن باشد.
- markdown/code/image/pdf preview کار کند.

Output:

- [project_file_api.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/project_file_api.go)
- [project_file_api_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/project_file_api_test.go)
- Project files are served only for registered project roots.
- Relative paths are normalized, unescaped, and constrained to the registered root before serving.
- Markdown, JSON/CSV/TSV, known code extensions, image/PDF, and generic binary MIME types are returned for Kanna previews.

## Milestone 15: Settings

### Task 15.1: Global settings model `[done]`

تنظیمات:

```text
app settings
locale
theme
default_provider
provider_defaults
editor
terminal
sounds
analytics
browserSettingsMigrated
standalone transcript defaults
```

Acceptance criteria:

- settings از backend خوانده و نوشته شود.
- settings روی disk persist شود.
- تغییر locale جهت UI را عوض کند.

Output:

- [settings.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/settings.go)
- [settings_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/settings_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- App settings now persist locale, theme, provider defaults, editor defaults, terminal defaults, sound settings, analytics, and browser migration state.
- `settings.writeAppSettingsPatch` applies Kanna-compatible partial patches and returns the updated snapshot.
- Locale writes are persisted through the same settings patch path used by the React UI.

### Task 15.2: App management settings ✅

موارد:

- restart server
- reload sessions
- hook notification mode
- startup mode info
- version/update info

Acceptance criteria:

- عملیات خطرناک confirmation داشته باشد. ✅
- restart server از UI ممکن باشد. ✅

Output:

- [buildinfo.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/buildinfo/buildinfo.go)
- [app_management.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/app_management.go)
- [app_management_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/app_management_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- [protocol.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/protocol/protocol.go)
- [types.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/shared/types.ts)
- [protocol.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/shared/protocol.ts)
- WebSocket now supports app management commands for reading management state, hook status, reload sessions, restart server, update check, and update install.
- Release builds inject the GitHub tag into `buildinfo.Version`; local builds use `dev`.
- Update checks read the latest GitHub release tag and keep development builds non-updateable.
- Existing REST reload/restart/hooks endpoints now share the same backend helpers.

### Task 15.3: Keybindings مطابق Kanna ✅

Acceptance criteria:

- `settings.readKeybindings` پیاده شود. ✅
- `settings.writeKeybindings` پیاده شود. ✅
- default keybindings با Kanna برابر باشد. ✅
- shortcutهای frontend بدون تغییر رفتاری کار کنند. ✅

Output:

- [keybindings.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/keybindings.go)
- [keybindings_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/keybindings_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Keybindings are persisted in `keybindings.json` under the app state directory and are created from Kanna defaults when missing.
- Invalid or empty keybinding files fall back to Kanna defaults with a warning snapshot.
- `settings.readKeybindings` and `settings.writeKeybindings` now work over the Go WebSocket backend.
- A write emits a fresh keybindings snapshot to the active subscribed connection so frontend shortcuts update without a reload.

### Task 15.4: LLM provider settings مطابق Kanna ✅

شرح:

- این بخش برای quick response، title generation و commit message generation است.

Acceptance criteria:

- `settings.readLlmProvider` پیاده شود. ✅
- `settings.writeLlmProvider` پیاده شود. ✅
- `settings.validateLlmProvider` پیاده شود. ✅
- provider kindهای Kanna حفظ شوند: `openai`, `openrouter`, `custom`. ✅

Output:

- [llm_provider.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/llm_provider.go)
- [llm_provider_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/llm_provider_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- LLM provider settings are persisted in `llm-provider.json` under the app state directory.
- Provider normalization follows Kanna: `openai`, `openrouter`, and `custom`, with the same base URLs and default models.
- `settings.readLlmProvider`, `settings.writeLlmProvider`, and `settings.validateLlmProvider` now work over the Go WebSocket backend.
- Validation sends an OpenAI-compatible `/responses` request when configuration is complete and returns Kanna-compatible config errors otherwise.

## Milestone 16: Frontend Integration With Go Protocol

### Task 16.1: اتصال Kanna socket client با کمترین تغییر ✅

شرح:

- TypeScript socket client موجود در Kanna حفظ شود.
- فقط URL، error handling لازم و type compatibility با Go تنظیم شود.
- command envelope همان Kanna بماند.

Acceptance criteria:

- sidebar snapshot از Go بیاید. ✅
- chat snapshot از Go بیاید. ✅
- settings snapshot از Go بیاید. ✅

Output:

- [workspace_snapshots.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots.go)
- [workspace_snapshots_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Kanna socket client continues to use `/ws` unchanged.
- Sidebar snapshots now come from the Go event store read model instead of a stub.
- Local project snapshots now come from the Go event store read model.
- Chat snapshots now derive runtime, queue, providers, and recent transcript entries from the Go event store.
- Settings snapshots already come from the Go backend through `settings.readAppSettings` and app-settings subscriptions.

### Task 16.2: اتصال composer ✅

Acceptance criteria:

- [x] create chat.
- [x] send message.
- [x] select provider/model/reasoning.
- [x] cancel turn.
- [x] queued message rendering.

Output:

- [workspace_composer.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer.go)
- [workspace_composer_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- [workspace_snapshots.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots.go)
- Composer socket commands now support `project.open`, `chat.create`, `chat.send`, `message.enqueue`, `message.dequeue`, `message.steer`, `chat.cancel`, and `chat.markRead`.
- Chat sending preserves provider/model/model options/effort/plan mode through the Go coordinator.
- Active runtime state and queued messages are reflected in chat snapshots.

### Task 16.3: اتصال runtime events ✅

Acceptance criteria:

- [x] assistant message به صورت live update شود.
- [x] status running/waiting/failed درست نمایش داده شود.
- [x] tool approval card ظاهر شود.

Output:

- [workspace_composer.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer.go)
- [workspace_composer_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer_test.go)
- [workspace_snapshots.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- [coordinator.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/agent/coordinator.go)
- [readmodels.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/readmodels/readmodels.go)
- Workspace runtime state changes now broadcast fresh snapshots to every subscribed WebSocket connection.
- Sidebar/chat snapshots now reflect active `starting`/`running`/`waiting_for_user` and persisted `failed` status.
- `chat.respondTool` is wired to the Go coordinator and tool call/result transcript entries are recorded for Kanna approval cards.

## Milestone 17: Legacy Viewer Bridge

### Task 17.1: نگه داشتن viewer فعلی خارج از Kanna workspace ✅

Acceptance criteria:

- [x] viewer فعلی می‌تواند پشت route legacy بماند.
- [x] نباید UI اصلی Kanna-port را با behaviorهای hook-follow فعلی آلوده کند.
- [x] installer/hook compatibility حفظ شود، اما core Kanna workspace event-driven بماند.

Output:

- [routes.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/routes.go)
- [routes_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/routes_test.go)
- [index.html](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/index.html)
- [styles.css](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/styles.css)
- [base.css](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/styles/base.css)
- [icons.css](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/styles/icons.css)
- Legacy viewer is now reachable under `/legacy/` with `/legacy` redirecting to the canonical route.
- Legacy viewer asset references are relative, so the viewer can remain isolated from the Kanna workspace root.
- Existing legacy APIs and hook compatibility endpoints remain unchanged.

### Task 17.2: import یا نمایش legacy sessions بدون تغییر UX Kanna ✅

Acceptance criteria:

- [x] sessionهای hook/discovery زیر پروژه دیده شوند.
- [x] read-only badge داشته باشند اگر قابل ادامه نیستند.
- [x] اگر Codex session قابل resume بود، گزینه continue فعال شود.
- [x] این بخش نباید sidebar اصلی Kanna را از project/chat model خارج کند.

Output:

- [workspace_legacy.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_legacy.go)
- [workspace_legacy_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_legacy_test.go)
- [workspace_snapshots.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots.go)
- [workspace_snapshots_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_snapshots_test.go)
- [workspace_composer_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer_test.go)
- [readmodels.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/workspace/readmodels/readmodels.go)
- [types.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/shared/types.ts)
- [useKannaState.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/app/useKannaState.ts)
- [ChatRow.tsx](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/components/chat-ui/sidebar/ChatRow.tsx)
- Legacy sessions are overlaid into sidebar snapshots under project groups without being copied into the Kanna event store.
- Selecting a legacy chat imports its transcript into a read-only Kanna chat snapshot.
- Sidebar rows expose `readOnly`, `canResume`, and `legacySessionKey`; the UI shows `Read-only` or `Resume` badges.

## Milestone 18: Browser Panel And Local Servers

### Task 18.1: Port local-http-servers ✅

شرح:

- Kanna local HTTP serverهای مربوط به project را پیدا و مدیریت می‌کند.

Acceptance criteria:

- [x] `browser.listLocalHttpServers` پیاده شود.
- [x] `browser.killLocalHttpServer` پیاده شود.
- [x] BrowserPanel frontend بدون redesign کار کند.

Output:

- [workspace_browser.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_browser.go)
- [workspace_browser_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_browser_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- `browser.listLocalHttpServers` now enumerates local listening ports, probes HTTP responses, extracts page titles, and marks `sameProject` from process cwd.
- `browser.killLocalHttpServer` kills the process bound to the requested listening port.
- BrowserPanel keeps using the existing socket contract and frontend code path.

### Task 18.2: Browser panel cache/state ✅

Acceptance criteria:

- [x] state مورد انتظار BrowserPanel حفظ شود.
- [x] project-specific browser preview behavior با Kanna یکی باشد.

Output:

- [browserPanelCache.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/lib/browserPanelCache.ts)
- [browserPanelCache.test.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/lib/browserPanelCache.test.ts)
- [BrowserPanel.tsx](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/components/chat-ui/BrowserPanel.tsx)
- BrowserPanel local HTTP server cache is now scoped per project.
- BrowserPanel loads, refreshes, and removes cached local server state by project.
- Existing right-sidebar browser state keeps per-project address/history/zoom behavior.

## Milestone 19: Quick Actions

### Task 19.1: Project quick actions ✅

Acceptance criteria:

- [x] `project.readQuickActions` پیاده شود.
- [x] `project.writeQuickActions` پیاده شود.
- [x] quick actionهای project روی disk persist شوند.
- [x] UI Kanna بدون تغییر رفتاری کار کند.

Output:

- [workspace_quick_actions.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_quick_actions.go)
- [workspace_quick_actions_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_quick_actions_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- `project.readQuickActions` and `project.writeQuickActions` are wired into the Go workspace WebSocket backend.
- Quick actions persist in each project at `.kanna/quick-actions.json`, matching Kanna.
- Quick action normalization matches Kanna limits and duplicate handling.

## Milestone 20: Skills

### Task 20.1: Skills list/search/install/uninstall ✅

Acceptance criteria:

- [x] `skills.search` پیاده شود.
- [x] `skills.install` پیاده شود.
- [x] `skills.uninstall` پیاده شود.
- [x] `skills.listInstalled` پیاده شود.
- [x] response shape با Kanna برابر باشد.
- [x] اگر skill backend فعلی نداریم، ابتدا adapter thin به Codex skills filesystem ساخته شود، نه UX جدید.

Output:

- [workspace_skills.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_skills.go)
- [workspace_skills_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_skills_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Skills WebSocket commands now return Kanna-compatible shapes for search, install, uninstall, and installed snapshots.
- `skills.search` calls `skills.sh/api/search` with Kanna query and limit normalization.
- `skills.install` and `skills.uninstall` use the same `npx skills` command shape as Kanna.
- `skills.listInstalled` reads the global skill lock and falls back to scanning the Codex skills filesystem.

## Milestone 21: Share And Standalone Export

### Task 21.1: Standalone transcript export ✅

Acceptance criteria:

- [x] `chat.exportStandalone` پیاده شود.
- [x] attachment modes مطابق Kanna باشد: `metadata`, `bundle`.
- [x] themeهای export مطابق Kanna باشد: `light`, `dark`.
- [x] output با viewer/export Kanna سازگار باشد.

Output:

- [workspace_standalone_export.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_standalone_export.go)
- [workspace_standalone_export_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_standalone_export_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- `chat.exportStandalone` now writes Kanna-compatible export directories under `.kanna/exports`.
- Export copies the built standalone viewer, writes `transcript.json`, supports `metadata` and `bundle` attachment modes, and rewrites local paths to `/workspace`.
- Share upload behavior and success/failure result shapes match Kanna.

### Task 21.2: Share dialog support ✅

Acceptance criteria:

- [x] StandaloneShareDialog frontend بدون redesign کار کند.
- [x] backend result shape با Kanna برابر باشد.

Output:

- [StandaloneShareDialog.tsx](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/components/chat-ui/StandaloneShareDialog.tsx)
- [useKannaState.ts](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web-react/src/client/app/useKannaState.ts)
- [workspace_standalone_export.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_standalone_export.go)
- Share dialog wiring already matches Kanna and uses the `shareUrl` returned by `chat.exportStandalone`.
- Backend success and failure payloads now match the Kanna contract used by the dialog flow.

## Milestone 22: Update Manager

### Task 22.1: update.check و update.install ✅

Acceptance criteria:

- [x] `update.check` پیاده شود.
- [x] `update.install` پیاده شود.
- [x] UpdateSnapshot با Kanna برابر باشد.
- [x] اگر release mechanism ما GoReleaser است، فقط backend implementation فرق کند، نه UI contract.

Output:

- [app_management.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/app_management.go)
- [app_management_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/app_management_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- [workspace_composer.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_composer.go)
- `update.check` reads GitHub releases for the GoReleaser release stream while preserving the Kanna `UpdateSnapshot` shape.
- `update.install` schedules the Go binary update/restart path while preserving the Kanna `UpdateInstallResult` shape.
- Update snapshots are persisted in-memory and broadcast to `update` subscribers so the frontend reacts like Kanna.

## Milestone 23: External Open

### Task 23.1: system.openExternal ✅

Actionهای Kanna:

```text
open_finder
open_terminal
open_editor
open_preview
open_default
```

Acceptance criteria:

- [x] file/path/line/column support حفظ شود.
- [x] editor preset/custom command با settings هماهنگ باشد.
- [x] path security رعایت شود.

Output:

- [workspace_external_open.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_external_open.go)
- [workspace_external_open_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_external_open_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- `system.openExternal` now supports Kanna actions: `open_finder`, `open_terminal`, `open_editor`, `open_preview`, and `open_default`.
- Editor presets/custom templates preserve file path, line, and column handling while running commands without shell expansion.
- Local path validation rejects URLs and control characters before resolving filesystem paths.

## Milestone 24: Auth, Analytics, Machine Name, CLI Runtime

### Task 24.1: Auth parity check ✅

Acceptance criteria:

- [x] behavior فایل‌های `auth.ts` و `cli-runtime.ts` در Kanna بررسی و معادل Go آن مشخص شود.
- [x] اگر auth فقط برای feature خاص Kanna است، همان feature با همان UX port شود.
- [x] auth جدید برای remote multi-user ساخته نشود.

Output:

- [routes.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/routes.go)
- [routes_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/routes_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Kanna `auth.ts` enables password auth only when CLI runtime passes a password. It uses an in-memory `kanna_session` cookie, same-origin Origin checks, and optional trusted proxy handling for secure cookies.
- Kanna `cli-runtime.ts` exposes `--password` together with host/share runtime flags. The current Go runtime is still local-only service/hook startup, so Task 24.1 keeps auth disabled instead of inventing remote multi-user auth.
- Disabled-auth parity is now explicit: `/auth/status` returns `{ enabled: false, authenticated: true }` and `/auth/logout` accepts `POST` with `{ ok: true }`, matching Kanna’s no-password UX shape.

### Task 24.2: Analytics parity ✅

Acceptance criteria:

- [x] analytics toggle و event naming با Kanna برابر باشد.
- [x] اگر analytics را disable می‌کنیم، UI و setting آن همچنان shape-compatible باشد.

Output:

- [analytics.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/analytics/analytics.go)
- [analytics_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/analytics/analytics_test.go)
- [workspace_analytics_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_analytics_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Kanna static analytics event/property names are mirrored in Go, including `analytics_enabled` and `analytics_disabled`.
- Kanna launch property derivation is mirrored as a reusable Go helper.
- External analytics transport remains a no-op reporter for this local-first Go port, while `settings.writeAppSettings` and `settings.writeAppSettingsPatch` preserve toggle semantics and event naming.

### Task 24.3: Machine name and CLI supervisor parity ✅

Acceptance criteria:

- [x] machine name behavior اگر در UI/settings استفاده شده port شود.
- [x] cli supervisor behavior اگر برای restart/update لازم است port شود.

Output:

- [workspace_machine.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_machine.go)
- [workspace_machine_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_machine_test.go)
- [workspace_ws.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/workspace_ws.go)
- Machine display name now matches Kanna behavior: macOS prefers `scutil --get ComputerName`, other platforms use hostname with `.local`/`.lan` stripped, and empty names fall back to `This Machine`.
- Platform naming remains Kanna/Node-compatible by exposing Windows as `win32`.
- Kanna’s long-running CLI supervisor restarts child processes through exit codes `75`/`76`. The Go port already uses service/hook-aware `restart` and `update` commands plus detached scheduling, so no extra supervisor process is introduced for the current startup model.

## Milestone 25: Security Hardening

### Task 25.1: Filesystem security ✅

Acceptance criteria:

- [x] project root allowlist.
- [x] no path traversal.
- [x] symlink policy مشخص.
- [x] file preview فقط داخل root.

Output:

- [project_file_api.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/project_file_api.go)
- [project_file_api_test.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/project_file_api_test.go)
- Project file serving now stores canonical registered roots and rejects broad roots such as the user home.
- Project file targets are resolved with `EvalSymlinks` and must remain under the canonical project root, so symlinks that escape the project are rejected.
- Existing file preview security remains root-scoped with the same symlink policy: symlinks are allowed only when their resolved target stays inside an allowed project/session root.

### Task 25.2: Command execution security

Acceptance criteria:

- terminal و agent commandها local-only باشند.
- approval flow برای عملیات حساس.
- logs حاوی secret نباشند.

### Task 25.3: Web server security

Acceptance criteria:

- bind default به localhost.
- CORS بسته باشد.
- WebSocket origin check داشته باشد.
- auth جدید برای remote exposure خارج از parity scope است مگر Kanna همان را داشته باشد.

## Milestone 26: Testing

### Backend tests

- Protocol parity with Kanna envelope.
- EventStore append/replay.
- Snapshot compaction.
- ReadModel derivation.
- WS command routing.
- AgentCoordinator queue/cancel.
- Codex JSON-RPC routing.
- Tool approval lifecycle.
- Git service parsing.
- File serving security.
- Settings persistence.
- Browser local servers.
- Quick actions.
- Skills commands.
- Update manager.
- Standalone export.
- External open.

### Frontend tests

- Existing Kanna frontend tests should be kept where practical.
- i18n switch.
- RTL layout smoke tests.
- composer provider/model controls.
- sidebar grouping.
- chat snapshot rendering.
- queued message rendering.
- tool approval cards.
- git panel rendering.
- settings/keybindings rendering.
- browser panel rendering.
- terminal panel rendering.

### E2E tests

- start server.
- open UI.
- create project.
- create chat.
- send mock agent message.
- approve mock tool.
- see transcript update.
- reload page and verify persistence.

## Milestone 27: CI And Release

### Task 27.1: Build pipeline

Acceptance criteria:

- frontend build.
- Go test.
- Go build.
- embed assets.
- goreleaser artifacts.

### Task 27.2: Installer compatibility

Acceptance criteria:

- installer همچنان Go/runtime لازم را آماده کند.
- service/hook mode بعد از نصب کار کند.
- restart بعد از install انجام شود.

## Suggested Implementation Order

ترتیب پیشنهادی برای کاهش ریسک:

1. Parity map کامل از Kanna.
2. Frontend copy با build موفق، بدون redesign.
3. i18n/RTL اضافه شود، بدون تغییر visual identity.
4. Go WebSocket protocol دقیقاً با envelope Kanna.
5. EventStore و ReadModels مطابق Kanna.
6. Project/chat CRUD مطابق Kanna.
7. ProviderCatalog فقط برای `claude | codex`.
8. AgentCoordinator با mock provider برای lock کردن UI/runtime contract.
9. Codex persistent adapter.
10. Live transcript streaming.
11. Tool approval.
12. Claude adapter.
13. Git/Diff/GitHub workflows.
14. Terminal backend.
15. BrowserPanel/local HTTP servers.
16. Quick actions.
17. Settings/keybindings/LLM provider.
18. Skills.
19. Share/standalone export.
20. Update manager.
21. External open.
22. Legacy viewer bridge/import.
23. Hardening, tests, release.

## Phasing Notes, Not Feature Removal

این موارد ممکن است در اولین vertical slice کامل نباشند، اما برای final parity حذف نمی‌شوند:

- Claude کامل.
- GitHub publish کامل.
- terminal روی Windows با parity کامل.
- share/export کامل.
- analytics behavior مطابق Kanna.
- skills کامل.
- update install کامل.
- BrowserPanel کامل.

Gemini از این لیست حذف شد چون در Kanna provider اصلی نیست. Gemini فقط برای legacy compatibility پروژه فعلی می‌تواند جداگانه حفظ شود، نه در Kanna parity workspace.

اولین vertical slice باید این را ثابت کند:

```text
React Kanna UI copied with minimal change
  + Go WebSocket backend
  + persistent Project/Chat/EventStore
  + Codex live send/resume
  + model/reasoning controls
  + tool approval loop
  + bilingual RTL/LTR UI
```

## Engineering Risk Register

### Risk: Codex app-server protocol instability

Mitigation:

- adapter کاملاً isolated باشد.
- raw logs نگهداری شود.
- capability negotiation داشته باشیم.

### Risk: Direct Claude SDK in Go unavailable

Mitigation:

- اول direct Go/CLI route بررسی شود.
- bridge JS فقط در صورت غیرممکن بودن parity با Go direct مجاز است و باید پشت adapter پنهان باشد.

### Risk: Frontend copy too tightly coupled to Kanna protocol

Mitigation:

- این ریسک عمداً پذیرفته می‌شود چون هدف copy کردن Kanna است.
- به جای تغییر frontend، Go backend باید Kanna protocol را پیاده کند.

### Risk: EventStore migration complexity

Mitigation:

- legacy viewer را جدا نگه داریم.
- imported legacy sessions را read-only شروع کنیم.

### Risk: RTL breaking code/diff/terminal

Mitigation:

- direction را component-level کنترل کنیم.
- code/diff/terminal/path همیشه LTR باشند.

## Definition Of Done For First Vertical Slice

Vertical slice اول زمانی کامل است که:

- UI جدید با فارسی/انگلیسی و RTL/LTR کار کند.
- پروژه جدید اضافه شود.
- chat جدید ساخته شود.
- provider Codex انتخاب شود.
- model/reasoning انتخاب شود.
- پیام ارسال شود.
- پاسخ Codex زنده در transcript دیده شود.
- اگر Codex tool approval خواست، UI آن را نشان دهد و پاسخ برگرداند.
- refresh صفحه chat را از بین نبرد.
- sidebar پروژه‌ها و chatها را درست نشان دهد.
- sessionهای legacy همچنان قابل مشاهده باشند.
- build/release/installer خراب نشود.

## Definition Of Done For Final Kanna Parity

Final parity زمانی کامل است که:

- ظاهر اصلی Kanna حفظ شده باشد.
- sidebar، local projects، chat page، settings، right sidebar، terminal و browser panel مطابق Kanna کار کنند.
- تمام commandهای `ClientCommand` در Kanna یا پیاده شده باشند یا با دلیل explicit و documented حذف شده باشند.
- تمام topicهای `SubscriptionTopic` در Kanna پشتیبانی شوند.
- تمام snapshotهای `ServerSnapshot` در Kanna پشتیبانی شوند.
- `claude` و `codex` providerها مطابق Kanna کار کنند.
- model/reasoning/context/fast/plan controls مطابق Kanna باشند.
- queue، steer، dequeue، cancel و stop draining مطابق Kanna باشند.
- tool approval و AskUserQuestion/ExitPlanMode مطابق Kanna باشند.
- Git/Diff/GitHub workflows مطابق Kanna باشند.
- terminal مطابق Kanna باشد.
- BrowserPanel/local HTTP servers مطابق Kanna باشد.
- quick actions مطابق Kanna باشد.
- skills مطابق Kanna باشد.
- settings/keybindings/LLM provider مطابق Kanna باشد.
- standalone export/share مطابق Kanna باشد.
- update/check/install مطابق Kanna UI contract باشد.
- frontend فارسی/انگلیسی داشته باشد.
- فارسی RTL باشد و code/diff/terminal/pathها LTR بمانند.
- legacy viewer فقط compatibility layer باشد، نه عامل تغییر UX اصلی.
