# معماری tmux-first برای ابوالقاسم

این سند برنامه اجرایی تبدیل ابوالقاسم به مدل tmux-first است. ایده از پروژه `agent-tmux-web` گرفته شده، اما هدف این نیست که یک clone از آن بسازیم. هدف این است که ابوالقاسم runtime اصلی چت را به tmux منتقل کند و خودش فقط metadata، UI، index سبک و اتصال به agentهای native را مدیریت کند.

## هدف

ابوالقاسم نباید مالک transcript کامل چت‌ها باشد. اجرای واقعی باید داخل tmux انجام شود، وب باید دو نمای ساده از همان runtime بدهد:

- Web view: خروجی tmux یا transcript native به شکل خوانا، RTL-friendly و مناسب وب.
- Terminal view: raw terminal واقعی با xterm، متصل به همان tmux session.

URL اختصاصی ابوالقاسم مثل `/_/chat/chat-...` همچنان باقی می‌ماند، اما این URL به یک chat record سبک اشاره می‌کند، نه به transcript سنگین داخل `messages.jsonl`.

## اصول

- tmux منبع اصلی اجرای زنده است.
- transcript کامل نباید در cache ابوالقاسم کپی شود.
- فایل‌های native خود agentها، مثل Codex session jsonl، Claude transcript و Gemini transcript، منبع history کامل هستند.
- cache ابوالقاسم فقط metadata و index سبک نگه می‌دارد.
- باز کردن یک chat نباید full transcript replay کند.
- fork/clone نباید transcript را duplicate کند.
- search نباید `messages.jsonl` مرکزی را full-scan کند.
- UI فعلی حفظ می‌شود؛ فقط یک toggle بین Web و Terminal لازم است.
- خطای نبودن `tmux` باید واضح و قابل فهم باشد.
- هیچ سرویس جدا، database جدید یا job queue لازم نیست.

## معماری مقصد

### 1. Chat record سبک

هر chat در ابوالقاسم باید فقط این اطلاعات را نگه دارد:

- `chatId`
- `projectId`
- `provider`
- `title`
- `createdAt`
- `updatedAt`
- `tmuxSession`
- `nativeSessionId`
- `nativeTranscriptPath`
- `parentChatId`
- `status`
- `lastSummary`
- `lastSeenOutputHash`

این record می‌تواند در eventstore فعلی بماند، اما message content کامل نباید آنجا append شود.

### 2. Tmux runtime

یک لایه کوچک backend لازم است:

```text
internal/workspace/tmuxruntime
```

مسئولیت‌ها:

- ساختن tmux session با نام پایدار.
- attach کردن terminal web به tmux.
- ارسال prompt با `tmux send-keys`.
- capture گرفتن با `tmux capture-pane`.
- resize کردن window/pane.
- تشخیص status از tail خروجی.
- kill/detach کردن attach وب بدون کشتن tmux session.

این لایه نباید transcript database بسازد.

### 3. Native provider bridge

هر provider باید بداند command مناسب برای tmux چیست:

- Codex: command شروع یا resume توسط adapter ساخته شود، نه hardcode داخل UI.
- Claude: command شروع یا resume توسط adapter.
- Gemini: command شروع یا resume توسط adapter.
- Custom: command از config.

خروجی adapter:

```text
command
cwd
nativeSessionId
nativeTranscriptPath
```

اگر session native هنوز معلوم نیست، بعد از شروع agent با hook یا parser lightweight sync می‌شود.

### 4. Web view

Web view از یکی از این منابع تغذیه می‌شود:

- برای حالت زنده: `tmux capture-pane` با محدودیت خطوط.
- برای history عمیق: native transcript adapter.
- برای نمایش سریع sidebar: metadata + `lastSummary`.

Web view نباید هنگام باز شدن chat کل transcript را بخواند.

### 5. Terminal view

Terminal view همان xterm موجود را استفاده می‌کند و به همان tmux session وصل می‌شود.

رفتار:

- toggle روی همان صفحه chat است.
- وقتی Terminal view بسته می‌شود، فقط attach وب بسته می‌شود.
- tmux session زنده می‌ماند.
- scrollback terminal در حافظه backend نباید بی‌نهایت رشد کند.

### 6. Search

Search سه سطح دارد:

- Search در current view: روی capture فعلی یا پیام‌های لودشده.
- Search داخل یک chat: از native transcript adapter و index سبک.
- Search global: از index سبک native transcriptها، نه `messages.jsonl`.

بعد از این معماری، `messages.jsonl` نباید source search باشد.

### 7. Checkpoint

Checkpoint نباید `messages.jsonl.gz` transcript کامل بسازد.

Checkpoint باید این‌ها را ذخیره کند:

- `chatId`
- `tmuxSession`
- `nativeSessionId`
- `nativeTranscriptPath`
- `lastSummary`
- code checkpoint

در صورت نیاز، transcript export باید explicit و دستی باشد، نه رفتار پیش‌فرض.

## Flowهای اصلی

### باز کردن URL chat

```text
GET /_/chat/:chatId
React route
load chat metadata
subscribe chat runtime
capture tmux tail
render Web view
```

در این flow نباید full transcript load شود.

### ارسال پیام در Web view

```text
user types in composer
chat.send
backend resolves chat tmux session
tmux send-keys prompt + Enter
watch starts
capture updates
web view refreshes tail
metadata/status updates
```

در این flow نباید `message_appended` با محتوای کامل نوشته شود.

### سوییچ به Terminal view

```text
toggle terminal
TerminalPane opens
backend attaches to same tmux session
xterm streams raw output/input
toggle back
web attach closes
tmux keeps running
```

### ساخت chat جدید

```text
chat.create
create metadata record
allocate tmuxSession
create or attach tmux session
run provider command
return chatId
```

### Fork

```text
chat.fork
create child metadata record
create new tmuxSession
provider adapter builds fork/resume command
do not copy transcript
link parentChatId
```

اگر provider فورک native ندارد، fork باید به عنوان new session با reference به parent ساخته شود، نه duplicate transcript.

### Search داخل chat

```text
search query
resolve native transcript path/session
query lightweight index
fallback adapter reads native transcript
return message refs/snippets
```

## Task list

### Backend: tmux runtime

- [x] ساخت package کوچک `internal/workspace/tmuxruntime`.
- [x] انتقال منطق tmux از terminal manager به runtime جدا، بدون بزرگ کردن manager.
- [x] ساخت تابع `NormalizeSessionName(chatId)`.
- [x] ساخت تابع `EnsureSession(session, cwd, command)`.
- [x] ساخت تابع `Attach(session, cols, rows)` برای raw terminal.
- [x] ساخت تابع `Send(session, text, enter)`.
- [x] ساخت تابع `Capture(session, lines)`.
- [x] ساخت تابع `Resize(session, cols, rows)`.
- [x] ساخت تابع `Status(session)` با capture tail محدود.
- [x] خطای روشن برای نبودن binary `tmux`.
- [x] تست unit برای sanitize نام session.
- [x] تست unit برای ساختن argsهای tmux.
- [x] تست integration سبک که اگر tmux نصب نبود skip شود.

### Backend: terminal streaming

- [x] جلوگیری از رشد بی‌نهایت `session.state` در terminal manager.
- [x] نگه داشتن فقط scrollback محدود برای snapshot.
- [x] جدا کردن close attach از kill tmux session.
- [x] حفظ رفتار terminal shell فعلی برای embedded terminal معمولی.
- [x] اضافه کردن تست برای اینکه close terminal در mode tmux خود tmux session را kill نکند.

### Backend: chat runtime model

- [x] اضافه کردن fieldهای runtime به chat metadata: `tmuxSession`, `nativeSessionId`, `nativeTranscriptPath`, `parentChatId`, `lastSummary`.
- [x] تغییر `chat.create` تا tmux session پایدار بسازد.
- [x] تغییر `chat.send` تا برای chatهای tmux-first به tmux بفرستد.
- [x] حفظ مسیر قدیمی برای chatهای legacy تا migration کامل شود.
- [x] اضافه کردن command یا snapshot برای خواندن runtime status سبک.
- [x] حذف dependency باز کردن chat به `workspaceChatMessages`.

### Backend: provider commands

- [x] تعریف interface کوچک برای ساخت command شروع/resume provider.
- [x] پیاده‌سازی Codex command builder.
- [x] پیاده‌سازی Claude command builder.
- [x] پیاده‌سازی Gemini command builder.
- [x] پشتیبانی از custom command از settings.
- [x] عدم hardcode کردن command داخل frontend.
- [x] sync کردن `nativeSessionId` و `nativeTranscriptPath` از hook/parser موجود.

### Backend: eventstore و storage

- [x] متوقف کردن append محتوای کامل پیام برای chatهای tmux-first.
- [x] نگه داشتن فقط eventهای metadata/status.
- [x] تغییر compaction تا transcript کامل را داخل `chat_restored_to_checkpoint` ننویسد.
- [x] اضافه کردن migration برای chatهای موجود.
- [x] گذاشتن archive path برای `messages.jsonl` قدیمی به جای rewrite خطرناک.
- [x] اضافه کردن ابزار report حجم storage: data، checkpoints، native transcripts، index.

### Backend: history

- [x] حذف full-read از `chat.readTranscriptIndex`.
- [x] ساخت transcript index سبک از native transcriptها.
- [x] تغییر `chat.loadHistory` تا از native transcript adapter page بخواند.
- [x] تغییر `chat.loadHistoryAround` تا full transcript را slice نکند.
- [x] برای chatهای قدیمی، fallback فعلی بماند تا migration تمام شود.

### Backend: search

- [x] تغییر global search تا از native transcript index بخواند.
- [x] حذف reliance روی `messages.jsonl` برای workspace search.
- [x] پشتیبانی از restored/legacy/native transcriptها در یک مسیر واحد.
- [x] ساخت index incremental با signature سبک.
- [x] عدم rebuild index هنگام هر باز شدن chat.
- [x] تست اینکه search بعد از compaction پیام‌ها را از دست ندهد.

### Backend: checkpoint

- [x] حذف ذخیره transcript کامل در checkpointهای پیش‌فرض.
- [x] ذخیره فقط metadata و code checkpoint.
- [x] اضافه کردن `.antigravity`, `.venv`, `venv`, `.idea`, `.vscode` به ignore checkpoint filesystem.
- [x] اضافه کردن تست برای ignore list.
- [x] اضافه کردن command explicit برای export transcript در صورت نیاز کاربر.

### Frontend: ChatPage

- [x] نگه داشتن ظاهر فعلی صفحه.
- [x] اضافه کردن toggle واحد بین Web و Terminal view.
- [x] در Terminal view، همان chat area با xterm پر شود.
- [x] در Web view، transcript خوانا حفظ شود.
- [x] composer در Web view به `chat.send` وصل بماند، اما backend آن را به tmux route کند.
- [x] در Terminal view، input مستقیم داخل xterm باشد.
- [x] هنگام برگشت از Terminal به Web، capture تازه گرفته شود.
- [x] اگر tmux نصب نیست، پیام خطا واضح در همان terminal area نمایش داده شود.

### Frontend: Navbar

- [x] دکمه toggle فقط یک icon button باشد.
- [x] وقتی در Web view هستیم icon ترمینال نشان دهد.
- [x] وقتی در Terminal view هستیم icon پیام/وب نشان دهد.
- [x] tooltip فارسی/انگلیسی داشته باشد.
- [x] با دکمه embedded terminal فعلی قاطی نشود.
- [x] ظاهر navbar عوض نشود.

### Frontend: Web readable view

- [x] parser ساده برای تبدیل capture tmux به پیام‌های خوانا.
- [x] پشتیبانی از RTL در متن.
- [x] حفظ LTR برای code blocks و terminal commands.
- [x] نمایش status سبک: running، waiting، needs input، error.
- [x] جلوگیری از auto-scroll آزاردهنده وقتی کاربر بالا را می‌خواند.
- [x] Force sync سبک برای capture تازه.

### Frontend: Sidebar

- [x] sidebar از metadata سبک بخواند.
- [x] preview از `lastSummary` یا tail capture ساخته شود.
- [x] باز کردن sidebar نباید transcriptها را load کند.
- [x] search در sidebar از backend index استفاده کند.

### UX

- [x] کاربر حس کند یک session واحد دارد، نه دو سیستم جدا.
- [x] Web و Terminal هر دو به یک tmux session وصل باشند.
- [x] هیچ متن آموزشی اضافه داخل UI لازم نیست.
- [x] فقط errorها و statusهای ضروری نشان داده شوند.
- [x] mobile terminal soft keys در صورت نیاز بعداً اضافه شود، نه در فاز اول.

### Migration

- [x] تشخیص chatهای قدیمی eventstore-based.
- [x] نگه داشتن آنها در مسیر legacy تا وقتی کاربر آنها را باز کند.
- [x] materialize جدید نباید transcript را دوباره در `messages.jsonl` کپی کند.
- [x] ساخت migration command برای تبدیل chat metadata به tmux-first.
- [x] ساخت backup قبل از دست زدن به `messages.jsonl`.
- [x] بعد از migration، old transcriptها archive شوند نه حذف مستقیم.

### Tests

- [x] Go unit tests برای tmuxruntime.
- [x] Go tests برای `terminal.create` در mode shell و tmux.
- [x] Go tests برای chat send route در tmux-first.
- [x] Go tests برای search بدون `messages.jsonl`.
- [x] TypeScript tests برای toggle state.
- [x] TypeScript tests برای parser capture.
- [x] Playwright یا smoke test دستی برای باز کردن chat، سوییچ terminal/web و ارسال prompt.

### Bloat control

- [x] dependency جدید اضافه نشود مگر ضروری باشد.
- [x] از xterm موجود استفاده شود.
- [x] build output و generated assets commit نشوند.
- [x] `messages.jsonl` دیگر source اصلی نشود.
- [x] هر task بعد از اجرا با `git diff --stat` و `git status --ignored` چک شود.
- [x] فایل‌های helper فقط وقتی ساخته شوند که duplication واقعی کم کنند.

## ترتیب اجرا

### فاز 1: runtime درست

- tmuxruntime جدا.
- terminal attach درست و bounded buffer.
- toggle UI متصل به runtime درست.
- خطای tmux missing.

### فاز 2: chat send از مسیر tmux

- chat.create با tmux session.
- chat.send route به tmux.
- status/watch/capture.
- web view از capture زنده.

### فاز 3: storage سبک

- توقف append transcript کامل.
- metadata-only events.
- compaction جدید.
- checkpoint سبک.

### فاز 4: history و search

- native transcript adapters.
- index سبک.
- حذف full transcript replay از open/search/history.

### فاز 5: migration

- ابزار migrate.
- archive فایل‌های قدیمی.
- fallback legacy تا پایان migration.

## معیار پایان

- باز کردن chat بزرگ، `messages.jsonl` را full-read نکند.
- ارسال پیام جدید، transcript کامل را داخل cache ابوالقاسم کپی نکند.
- Terminal view و Web view یک tmux session واحد را نشان دهند.
- Fork transcript را duplicate نکند.
- Search بعد از compaction پیام‌ها را از دست ندهد.
- checkpoint چندصد مگابایت فایل بی‌ربط نسازد.
- UI فعلی جز toggle تغییر ظاهری محسوس نداشته باشد.
