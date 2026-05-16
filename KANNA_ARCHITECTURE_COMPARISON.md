# Kanna Architecture Compared With ai-agent-manager

این سند تفاوت معماری `Kanna` با وضعیت فعلی `ai-agent-manager` را توضیح می‌دهد، مخصوصاً در بخش server architecture، مدل پیام‌ها، commandها، eventها و جریان رفت‌وبرگشت داده بین UI، backend و agentها.

## خلاصه تفاوت

`ai-agent-manager` فعلی بیشتر یک viewer است:

- sessionها را از hook یا filesystem discovery پیدا می‌کند.
- transcriptها را parse می‌کند.
- پیام‌ها را به UI نشان می‌دهد.
- یک bridge اولیه برای فرستادن پیام به Codex دارد.
- state اصلی آن metadata نشست‌هاست.

`Kanna` یک agent workbench است:

- خودش مالک chat/session runtime است.
- پیام، turn، tool approval، queue، git، project و terminal را به عنوان domain مدیریت می‌کند.
- backend آن event-driven است.
- UI با WebSocket command/subscription کار می‌کند.
- agentها از طریق coordinator مرکزی کنترل می‌شوند.

فرق اصلی این است:

```text
ai-agent-manager:
  agent transcript -> parser -> viewer UI

Kanna:
  UI command -> server coordinator -> agent runtime -> events -> read models -> UI snapshots
```

## معماری فعلی ai-agent-manager

### لایه‌های اصلی

```text
Browser
  |
  | REST + SSE
  v
Go HTTP Server
  |
  +-- state.json
  +-- transcript parser
  +-- filesystem discovery
  +-- hook event receiver
  +-- thin Codex turn endpoint
  |
  v
Codex app-server per request
```

### فایل‌های مهم

- [internal/server/api.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/api.go)
- [internal/server/agent_api.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/server/agent_api.go)
- [internal/agent/codex.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/agent/codex.go)
- [internal/state/state.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/state.go)
- [internal/state/store.go](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/internal/state/store.go)
- [web/src/api.js](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/src/api.js)
- [web/src/viewer-app.js](/home/h-mousavi/Projects/Hamed/codex-rtl-plugin/web/src/viewer-app.js)

### مدل state فعلی

state اصلی ما تقریباً این است:

```go
type AppState struct {
    Sessions         map[string]SessionMeta
    LatestSessionKey string
    LatestSessionID  string
}
```

یعنی سیستم فعلی بیشتر می‌داند:

- چه sessionهایی وجود دارند.
- هر session مربوط به چه agent است.
- transcript path چیست.
- cwd چیست.
- project name چیست.
- آخرین preview چیست.
- session name دستی چیست.

اما این‌ها را به عنوان domain کامل ندارد:

- chat event
- turn event
- queued message
- tool request
- tool response
- active runtime
- git snapshot
- branch state
- terminal state
- provider model options

### پروتکل فعلی UI

UI فعلی بیشتر REST می‌زند:

```text
GET  /api/sessions
GET  /api/session/{key}/messages
PUT  /api/session/{key}
GET  /api/search
POST /api/actions/reload-sessions
GET  /api/agent/status
POST /api/agent/turn
POST /api/agent/codex/turn
GET  /api/events
```

`/api/events` از SSE استفاده می‌کند، اما فقط برای notificationهای سبک مثل session update است، نه برای runtime کامل agent.

### جریان ارسال پیام فعلی

جریان فعلی ارسال پیام به Codex:

```text
Browser composer
  |
  | POST /api/agent/codex/turn
  v
handleAgentTurn
  |
  | load state.json
  | validate session/cwd/model
  | acquire workspace lock
  v
agent.RunCodexTurn
  |
  | start new `codex app-server`
  | initialize
  | thread/resume or thread/start
  | turn/start
  | wait until turn/completed
  | kill app-server
  v
update state.json
  |
  | broadcast SSE event
  v
Browser reloads session/messages
```

مشکل‌های این مدل:

- streaming واقعی ندارد.
- هر درخواست app-server جدا می‌سازد.
- active runtime state ندارد.
- اگر agent tool approval بخواهد، چرخه انسانی کامل نداریم.
- queue ندارد.
- cancel واقعی turn ندارد.
- plan mode و collaboration mode ندارد.
- UI فقط بعد از پایان turn نتیجه را می‌فهمد.
- transcript همچنان منبع اصلی نمایش است.

## معماری Kanna

### لایه‌های اصلی

```text
Browser React UI
  |
  | WebSocket commands/subscriptions
  v
Bun Server
  |
  +-- WSRouter
  +-- EventStore
  +-- ReadModels
  +-- AgentCoordinator
  +-- ProviderCatalog
  +-- CodexAppServerManager
  +-- Claude SDK Session Manager
  +-- DiffStore / Git workflow
  +-- TerminalManager
  +-- Settings / Keybindings / Skills
  |
  v
Local agents and tools
  |
  +-- codex app-server
  +-- Claude Agent SDK
  +-- git
  +-- shell / terminal
  +-- local filesystem
```

### فایل‌های مهم Kanna

- [src/shared/protocol.ts](/home/h-mousavi/Projects/Hamed/kanna/src/shared/protocol.ts)
- [src/shared/types.ts](/home/h-mousavi/Projects/Hamed/kanna/src/shared/types.ts)
- [src/server/events.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/events.ts)
- [src/server/event-store.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/event-store.ts)
- [src/server/read-models.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/read-models.ts)
- [src/server/ws-router.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/ws-router.ts)
- [src/server/agent.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/agent.ts)
- [src/server/codex-app-server.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/codex-app-server.ts)
- [src/server/provider-catalog.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/provider-catalog.ts)
- [src/server/diff-store.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/diff-store.ts)
- [src/server/terminal-manager.ts](/home/h-mousavi/Projects/Hamed/kanna/src/server/terminal-manager.ts)

## مدل داده در Kanna

Kanna به جای یک `state.json` ساده، چند event stream دارد:

```text
projects.jsonl
chats.jsonl
messages.jsonl
queued-messages.jsonl
turns.jsonl
snapshot.json
```

هر تغییر مهم در سیستم یک event است:

```text
project.opened
project.sidebar_renamed
project.removed

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
turn.pending_fork_session_token_set
```

این یعنی backend می‌تواند state را از eventها rebuild کند و برای UI snapshot بسازد.

## Read Model در Kanna

Kanna eventهای خام را مستقیم به UI نمی‌دهد. اول از آن‌ها read model می‌سازد:

```text
EventStore state
  |
  v
ReadModels
  |
  +-- SidebarData
  +-- LocalProjectsSnapshot
  +-- ChatSnapshot
  +-- Git/Diff snapshot
  +-- Runtime status
  v
UI subscriptions
```

مزیت این مدل:

- UI snapshot آماده می‌گیرد.
- backend می‌تواند eventها را compact کند.
- active runtime از persisted state جداست.
- session/chat/project status قابل derive است.
- pagination و history loading قابل کنترل است.

## پروتکل پیام‌ها در Kanna

Kanna به جای endpointهای REST پراکنده، `ClientCommand` دارد. UI از طریق WebSocket command می‌فرستد.

### نمونه commandهای project

```ts
{ type: "project.open", localPath: string }
{ type: "project.create", localPath: string, title: string }
{ type: "project.rename", projectId: string, title: string }
{ type: "project.remove", projectId: string }
{ type: "project.readDiffPatch", projectId: string, path: string }
```

### نمونه commandهای chat

```ts
{ type: "chat.create", projectId: string }
{ type: "chat.rename", chatId: string, title: string }
{ type: "chat.archive", chatId: string }
{ type: "chat.unarchive", chatId: string }
{ type: "chat.delete", chatId: string }
{ type: "chat.fork", chatId: string }
{ type: "chat.loadHistory", chatId: string, cursor?: string }
```

### command اصلی ارسال پیام

در Kanna ارسال پیام یک command غنی است، نه فقط `message + cwd`:

```ts
{
  type: "chat.send",
  chatId?: string,
  projectId: string,
  content: string,
  provider: "claude" | "codex",
  model: string,
  modelOptions: {
    reasoningEffort?: string,
    contextWindow?: string,
    fastMode?: boolean
  },
  planMode?: boolean,
  attachments?: ChatAttachment[]
}
```

این command به server اجازه می‌دهد دقیقاً بداند:

- پیام برای کدام project است.
- ادامه کدام chat است.
- provider چیست.
- model چیست.
- reasoning mode چیست.
- plan mode روشن است یا نه.
- attachmentها کدامند.

### commandهای tool approval

```ts
{
  type: "chat.respondTool",
  chatId: string,
  toolUseId: string,
  result: unknown
}
```

این برای زمانی است که agent وسط turn از UI تأیید یا ورودی می‌خواهد.

### commandهای queue و steer

```ts
{ type: "message.dequeue", chatId: string, queuedMessageId: string }
{ type: "message.steer", chatId: string, content: string }
```

یعنی اگر agent در حال کار باشد، پیام جدید یا queue می‌شود یا turn فعلی steer/cancel می‌شود.

### commandهای git

```ts
{ type: "git.refreshDiffs", projectId: string }
{ type: "git.listBranches", projectId: string }
{ type: "git.checkoutBranch", projectId: string, branch: string }
{ type: "git.createBranch", projectId: string, name: string }
{ type: "git.previewMergeBranch", projectId: string, branch: string }
{ type: "git.mergeBranch", projectId: string, branch: string }
{ type: "git.generateCommitMessage", projectId: string, paths: string[] }
{ type: "git.commit", projectId: string, paths: string[], summary: string, body: string }
{ type: "git.sync", projectId: string, action: "fetch" | "pull" | "push" | "publish" }
```

### commandهای terminal

```ts
{ type: "terminal.create", projectId: string, terminalId: string, cols: number, rows: number }
{ type: "terminal.input", terminalId: string, data: string }
{ type: "terminal.resize", terminalId: string, cols: number, rows: number }
{ type: "terminal.close", terminalId: string }
```

### commandهای settings

```ts
{ type: "settings.readAppSettings" }
{ type: "settings.writeAppSettingsPatch", patch: AppSettingsPatch }
{ type: "settings.readLlmProvider" }
{ type: "settings.writeLlmProvider", ... }
{ type: "settings.validateLlmProvider", ... }
```

## پیام‌هایی که server به UI برمی‌گرداند

Kanna فقط پاسخ request/response ساده ندارد. UI به topicها subscribe می‌شود و server snapshot یا event می‌فرستد.

Topicهای اصلی:

```text
sidebar
local-projects
chat
project-git
terminal
app-settings
keybindings
update
```

### نمونه snapshot برای sidebar

```ts
{
  projectGroups: [
    {
      groupKey: string,
      title: string,
      localPath: string,
      chats: SidebarChatRow[],
      archivedChats: SidebarChatRow[],
      defaultCollapsed: boolean
    }
  ]
}
```

### نمونه snapshot برای chat

```ts
{
  runtime: {
    chatId: string,
    projectId: string,
    localPath: string,
    title: string,
    status: "idle" | "running" | "waiting_for_user" | "failed",
    provider: "claude" | "codex",
    model?: string,
    planMode?: boolean,
    sessionToken?: string
  },
  messages: TranscriptEntry[],
  queuedMessages: QueuedChatMessage[],
  history: {
    hasOlder: boolean,
    olderCursor?: string
  },
  availableProviders: ProviderCatalogEntry[]
}
```

### نمونه terminal event

```ts
{ type: "terminal.output", terminalId: string, data: string }
{ type: "terminal.exit", terminalId: string, exitCode: number, signal?: number }
```

## جریان ارسال پیام در Kanna

```text
Browser ChatInput
  |
  | WebSocket command: chat.send
  v
WSRouter
  |
  v
AgentCoordinator
  |
  | validate project/chat/provider/model/options
  | create chat if needed
  | append optimistic user message
  | record turn.started
  | update runtime status
  v
Provider adapter
  |
  +-- CodexAppServerManager
  |     |
  |     +-- reuse persistent codex app-server context
  |     +-- thread/start or thread/resume or thread/fork
  |     +-- turn/start
  |     +-- receive raw notifications/requests
  |
  +-- Claude SDK session
        |
        +-- stream SDK messages
        +-- canUseTool callbacks
  |
  v
Normalize provider events into TranscriptEntry
  |
  v
EventStore append
  |
  v
ReadModels derive ChatSnapshot/SidebarData
  |
  v
WSRouter broadcasts updated snapshots
  |
  v
Browser updates live
```

## Codex تفاوت مهم

### مدل فعلی ما

```text
POST /api/agent/codex/turn
  -> start codex app-server
  -> initialize
  -> thread/start or thread/resume
  -> turn/start
  -> wait for turn/completed
  -> kill app-server
  -> update session metadata
```

### مدل Kanna

```text
chat.send
  -> AgentCoordinator
  -> CodexAppServerManager
  -> persistent app-server context per chat
  -> initialize with experimentalApi
  -> thread/start/resume/fork
  -> turn/start with model/effort/serviceTier/collaborationMode
  -> stream notifications
  -> handle tool requests
  -> handle approval requests
  -> append transcript entries live
  -> finish/fail/cancel turn event
```

تفاوت کلیدی:

- Kanna app-server را برای هر request نمی‌کشد و نابود نمی‌کند.
- Kanna raw eventها را می‌فهمد.
- Kanna tool request را به UI می‌آورد.
- Kanna approval را از UI برمی‌گرداند.
- Kanna plan mode را به collaboration mode وصل می‌کند.
- Kanna status زنده دارد.

## AgentCoordinator چه کاری می‌کند؟

در Kanna، `AgentCoordinator` مغز runtime است.

مسئولیت‌ها:

- start turn
- resume/fork session
- append user message
- append assistant/tool messages
- manage active turns
- manage draining streams
- manage queue
- cancel turn
- steer active turn
- respond to tool request
- update chat provider
- update plan mode
- generate title
- call provider-specific adapter
- broadcast state changes

در پروژه ما این نقش هنوز وجود ندارد. منطق ارسال پیام مستقیم داخل HTTP handler و `RunCodexTurn` پخش شده است.

## ProviderCatalog تفاوت مهم

در Kanna provider فقط یک string نیست. هر provider این‌ها را دارد:

- id
- label
- supported models
- default model
- reasoning options
- plan mode support
- context window support
- fast/slow mode support
- model normalization
- model option normalization

در پروژه ما فعلاً model بیشتر یک input آزاد است و agentهای غیر Codex هم read-only هستند.

## Tool Approval تفاوت مهم

Kanna می‌تواند وسط اجرای agent این چرخه را اجرا کند:

```text
agent wants approval/input
  |
  v
server creates pending tool request
  |
  v
UI shows approval/question
  |
  v
user approves/denies/responds
  |
  v
chat.respondTool
  |
  v
server resumes provider request
```

در پروژه ما فعلاً tool requestها یا رد می‌شوند یا اصلاً runtime کافی برای مدیریتشان نداریم.

## Git/Diff تفاوت مهم

در Kanna git بخشی از محصول است، نه یک feature جانبی.

backend می‌تواند:

- repo را detect کند.
- diffها را بخواند.
- branchها را list کند.
- merge preview بسازد.
- commit message تولید کند.
- commit/push/pull/fetch/publish انجام دهد.
- GitHub availability را check کند.

UI هم right sidebar کامل برای review تغییرات دارد.

در پروژه ما هنوز git domain وجود ندارد.

## Terminal تفاوت مهم

Kanna terminal را از طریق WebSocket کنترل می‌کند:

```text
terminal.create
terminal.input
terminal.resize
terminal.close

server -> terminal.output
server -> terminal.exit
```

در پروژه ما terminal runtime وجود ندارد.

## چرا این تفاوت‌ها مهم هستند؟

اگر فقط بخواهیم viewer باشیم، معماری فعلی قابل ادامه است.

اما اگر بخواهیم Web UI واقعی برای Codex/Claude/Gemini بسازیم، معماری فعلی به مشکل می‌خورد چون:

- REST request/response برای agent stream کافی نیست.
- transcript parser برای active chat source of truth خوبی نیست.
- بدون EventStore نمی‌توان queue، status، approval، retry، fork و history درست داشت.
- بدون AgentCoordinator کنترل چند provider و active turn سخت می‌شود.
- بدون ProviderCatalog مدل و reasoning mode تبدیل به inputهای شکننده می‌شود.
- بدون ReadModel UI مجبور می‌شود domain logic زیادی داشته باشد.

## معماری پیشنهادی برای نزدیک شدن به Kanna

### فاز ۱: Domain و EventStore

اضافه کردن event store کنار state فعلی:

```text
projects.jsonl
chats.jsonl
messages.jsonl
turns.jsonl
queued-messages.jsonl
snapshot.json
```

entityهای لازم:

```text
Project
Chat
TranscriptEntry
Turn
QueuedMessage
ProviderProfile
ToolRequest
RuntimeStatus
```

### فاز ۲: WebSocket Protocol

اضافه کردن protocol جدید:

```text
chat.send
chat.cancel
chat.respondTool
chat.loadHistory
project.open
project.rename
settings.patch
```

REST فعلی می‌تواند برای viewer بماند.

### فاز ۳: AgentCoordinator

ساخت coordinator مرکزی:

```text
AgentCoordinator
  +-- activeTurns
  +-- queuedMessages
  +-- provider adapters
  +-- tool requests
  +-- runtime broadcasts
```

### فاز ۴: Codex Persistent Adapter

جایگزینی `RunCodexTurn` فعلی با adapter پایدار:

```text
CodexAppServerManager
  +-- start/reuse app-server
  +-- initialize experimentalApi
  +-- thread start/resume/fork
  +-- turn start
  +-- stream notifications
  +-- approval request handling
```

### فاز ۵: UI جدید Chat Workspace

UI باید از viewer-only به workspace تبدیل شود:

```text
Project sidebar
Chat transcript
Composer with provider/model/reasoning/plan
Runtime status
Tool approval cards
Queued messages
Right sidebar for git/diff
Terminal panel
```

## جمع‌بندی

Kanna از اول برای کنترل agent طراحی شده است.

`ai-agent-manager` فعلی از اول برای دیدن sessionها طراحی شده و حالا کم‌کم کنترل agent به آن اضافه شده است.

برای رسیدن به هدفی که مدنظر است، باید `ai-agent-manager` دو لایه داشته باشد:

```text
Legacy Viewer Layer
  - hook discovery
  - filesystem discovery
  - transcript reader
  - old session browsing

Agent Workspace Layer
  - event store
  - websocket protocol
  - agent coordinator
  - provider adapters
  - live chat runtime
  - git/diff/terminal/tool approval
```

این دو لایه می‌توانند هم‌زمان وجود داشته باشند، اما نباید Web UI واقعی را روی transcript parser فعلی بنا کنیم.

