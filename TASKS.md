بله. این را مثل یک **Specification مهندسی + تسک‌بندی قابل اجرا برای GitHub** می‌بندم؛ طوری که بشود مستقیم تبدیلش کرد به issue، milestone و roadmap.

# Specification پروژه: Codex RTL Session Viewer

نام پیشنهادی:

```text
codex-session-viewer
```

یا اگر تمرکز RTL مهم‌تر است:

```text
codex-rtl-viewer
```

هدف پروژه:

```text
یک ابزار local، سبک، صفر-توکن، قابل نصب عمومی، برای نمایش سشن‌های Codex در مرورگر با پشتیبانی درست از فارسی/RTL، lazy load، auto update، و notification هنگام تغییر سشن فعال.
```

---

# 1. مسئله اصلی

## مشکل فعلی

داخل Codex TUI وقتی فارسی، انگلیسی، path، stack trace و عدد با هم ترکیب می‌شوند، متن‌ها بد رندر می‌شوند.

مثلاً این‌ها مشکل دارند:

```text
- Postgres: total_ok=0 یعنی هیچ request موفق نیست
src/repositories/reaction_repository.py:20 مشکل اینجاست
TypeError: int() argument must be ...
```

ترمینال و TUI جهت پایه‌ی متن را خوب مدیریت نمی‌کنند.

## چیزی که نمی‌خواهیم

```text
استفاده از LLM برای باز کردن viewer
مصرف توکن
custom prompt
skill
slash commandی که prompt بفرستد
وابستگی به tmux
نیاز به نصب terminal خاص
رندر کامل همه پیام‌ها از اول
سرور سنگین همیشه روشن
```

## چیزی که می‌خواهیم

```text
Codex کار خودش را در TUI انجام دهد.
Hook بدون مصرف توکن فقط مسیر transcript فعلی را بدهد.
یک local server سبک روی 127.0.0.1:9090 بالا باشد.
مرورگر سشن‌ها را تمیز، RTL-safe و lazy-load نشان بدهد.
اگر سشن جدید فعال شد، صفحه notification بدهد، نه اینکه ناگهانی بپرد.
```

---

# 2. اصول طراحی

## اصل ۱: Zero Token

هیچ عملیاتی نباید باعث invoke شدن مدل شود.

مجاز:

```text
Codex hook
خواندن JSONL
اجرای local binary
HTTP local server
Browser UI
```

غیرمجاز:

```text
custom prompt
skill
ارسال پیام به Codex
درخواست از مدل برای export
```

---

## اصل ۲: Local Only

سرور فقط باید روی localhost گوش بدهد:

```text
127.0.0.1:9090
```

نه:

```text
0.0.0.0:9090
```

مگر کاربر صراحتاً flag بدهد.

---

## اصل ۳: سبک بودن

چون سرور همیشه بالا می‌ماند، نباید سنگین باشد.

تصمیم پیشنهادی:

```text
Backend: Go
Frontend: Vanilla HTML/CSS/JS
Database: بدون دیتابیس در MVP
State: فایل‌های JSON کوچک در ~/.cache
Transport update: SSE یا polling سبک
```

نه React، نه Next، نه Electron، نه Node server.

---

## اصل ۴: Lazy Load

در صفحه سشن:

```text
پیام‌های آخر نمایش داده شوند.
پیام‌های قدیمی‌تر با scroll به بالا load شوند.
همه transcript از اول رندر نشود.
```

مثل چت واقعی:

```text
پیام قدیمی‌تر
پیام قدیمی‌تر
پیام فعلی
```

---

## اصل ۵: UX محترمانه هنگام تغییر سشن

وقتی کاربر در حال دیدن Session A است و Session B آپدیت می‌شود:

```text
نباید ناگهانی redirect شود.
باید notification بیاید:

«سشن جدید فعال شد. تا ۵ ثانیه دیگر منتقل می‌شوی.»

[لغو] [همین الان برو]
```

اگر کاربر لغو کرد، دیگر برای همان event redirect نشود.

---

# 3. معماری کلی

```text
┌────────────────────┐
│ Codex TUI           │
│ inside any terminal │
└─────────┬──────────┘
          │
          │ Stop Hook
          ▼
┌────────────────────┐
│ codex-rtl hook      │
│ zero-token command  │
└─────────┬──────────┘
          │
          │ POST local event
          ▼
┌────────────────────────────┐
│ codex-rtl server            │
│ 127.0.0.1:9090              │
│ lightweight Go HTTP server  │
└─────────┬──────────────────┘
          │
          │ reads transcript JSONL on demand
          ▼
┌────────────────────────────┐
│ Browser UI                  │
│ sessions list               │
│ current session             │
│ lazy loaded messages        │
│ RTL/LTR rendering           │
└────────────────────────────┘
```

---

# 4. Command Line Interface

## Command اصلی

```bash
codex-rtl
```

## Subcommands

```bash
codex-rtl server
codex-rtl hook
codex-rtl install
codex-rtl uninstall
codex-rtl open
codex-rtl status
```

---

## 4.1. `codex-rtl server`

هدف:

```text
local HTTP server را بالا بیاورد.
```

نمونه:

```bash
codex-rtl server --port 9090
```

رفتار:

```text
روی 127.0.0.1:9090 گوش بدهد.
state قبلی را از ~/.cache/codex-rtl بخواند.
pending hook eventها را load کند.
UI را serve کند.
APIها را expose کند.
```

Acceptance Criteria:

```text
با اجرای command، صفحه http://127.0.0.1:9090 بازشدنی باشد.
مصرف CPU در حالت idle نزدیک صفر باشد.
اگر هیچ سشنی وجود ندارد، صفحه empty state نشان دهد.
اگر فایل state خراب است، server crash نکند.
```

---

## 4.2. `codex-rtl hook`

هدف:

```text
از Codex Hook ورودی JSON بگیرد و server را از آپدیت سشن مطلع کند.
```

Codex به hook چیزی شبیه این می‌دهد:

```json
{
  "session_id": "...",
  "transcript_path": "...",
  "cwd": "..."
}
```

رفتار:

```text
stdin را بخوان.
session_id، transcript_path، cwd را استخراج کن.
اگر server روشن است، POST /api/hook بزن.
اگر server روشن نیست، event را در pending-events.jsonl ذخیره کن.
در هر حالت exit 0 بده.
```

خیلی مهم:

```text
hook نباید Codex را fail کند.
hook نباید کند باشد.
hook نباید چیزی به مدل بفرستد.
```

Acceptance Criteria:

```text
وقتی server روشن است، سشن در UI ظاهر شود.
وقتی server خاموش است، event در pending ذخیره شود.
وقتی بعداً server روشن شد، pending eventها نمایش داده شوند.
hook در کمتر از ۱۰۰ms تمام شود، مگر network local مشکل داشته باشد.
```

---

## 4.3. `codex-rtl install`

هدف:

```text
نصب خودکار hook داخل ~/.codex/config.toml
```

رفتار:

```text
config.toml را پیدا کند.
backup بگیرد.
feature flag hooks را فعال کند.
Stop hook را اضافه کند.
اگر قبلاً نصب شده، duplicate نسازد.
```

بخش پیشنهادی در config:

```toml
[features]
codex_hooks = true

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "codex-rtl hook"
timeout = 3
```

برای جلوگیری از خراب کردن config، installer باید marker بگذارد:

```toml
# BEGIN codex-rtl-viewer
...
# END codex-rtl-viewer
```

Acceptance Criteria:

```text
install دوبار اجرا شود، config duplicate نشود.
قبل از تغییر، backup ساخته شود.
اگر config وجود ندارد، ساخته شود.
اگر TOML قابل parse نیست، نصب متوقف شود و پیام واضح بدهد.
```

---

## 4.4. `codex-rtl uninstall`

هدف:

```text
حذف hookهای نصب‌شده توسط ابزار.
```

رفتار:

```text
فقط block بین BEGIN/END خودش را حذف کند.
به بقیه config دست نزند.
```

Acceptance Criteria:

```text
بعد از uninstall، Codex دیگر hook ابزار را اجرا نکند.
config کاربر خراب نشود.
```

---

## 4.5. `codex-rtl open`

هدف:

```text
مرورگر را روی http://127.0.0.1:9090 باز کند.
```

رفتار:

```text
اگر server روشن نیست، یا پیام بدهد یا با flag خودش server را start کند.
```

نمونه:

```bash
codex-rtl open
codex-rtl open --start-server
```

Acceptance Criteria:

```text
روی Linux با xdg-open کار کند.
روی macOS با open کار کند.
روی Windows با start/rundll32 کار کند.
```

---

## 4.6. `codex-rtl status`

هدف:

```text
وضعیت نصب، server، port، تعداد سشن‌ها و آخرین سشن را نشان دهد.
```

خروجی نمونه:

```text
Codex RTL Viewer

Server: running
URL: http://127.0.0.1:9090
Hook: installed
Sessions: 12
Latest session: reaction-service / 2 seconds ago
State dir: ~/.cache/codex-rtl
```

---

# 5. API Specification

## 5.1. `GET /`

صفحه اصلی UI.

رفتار:

```text
اگر latest session وجود دارد، آخرین سشن را نمایش بده.
اگر نه، empty state.
```

---

## 5.2. `GET /sessions`

HTML یا redirect به صفحه لیست سشن‌ها.

---

## 5.3. `GET /session/{session_id}`

صفحه یک سشن مشخص.

رفتار:

```text
سشن انتخاب‌شده را باز کند.
آخرین پیام‌ها را lazy-load کند.
```

---

## 5.4. `GET /api/state`

خروجی:

```json
{
  "latest_session_id": "abc",
  "latest_updated_at": "2026-05-14T10:30:00Z",
  "session_count": 12,
  "server_time": "2026-05-14T10:30:05Z"
}
```

---

## 5.5. `GET /api/sessions`

Query params:

```text
limit
offset
project
```

خروجی:

```json
{
  "items": [
    {
      "session_id": "abc",
      "cwd": "/home/user/project",
      "project_name": "reaction-service",
      "transcript_path": "/home/user/.codex/sessions/...",
      "updated_at": "2026-05-14T10:30:00Z",
      "message_count_estimate": 120,
      "last_preview": "مشکل از TypeError است..."
    }
  ],
  "next_offset": 20
}
```

Acceptance Criteria:

```text
لیست سشن‌ها سریع load شود.
برای preview لازم نیست کل transcript parse شود.
sessionها بر اساس updated_at مرتب شوند.
```

---

## 5.6. `GET /api/session/{session_id}/messages`

برای lazy load.

Query params:

```text
cursor
limit
direction
```

نمونه:

```http
GET /api/session/abc/messages?limit=30
GET /api/session/abc/messages?before=event_80&limit=30
GET /api/session/abc/messages?after=event_110&limit=20
```

خروجی:

```json
{
  "session_id": "abc",
  "items": [
    {
      "id": "event_91",
      "role": "user",
      "kind": "message",
      "text": "ببین من نمیخوام توکن مصرف کنم!",
      "direction": "rtl",
      "created_at": null
    },
    {
      "id": "event_92",
      "role": "assistant",
      "kind": "message",
      "text": "کاملاً حق با توست...",
      "direction": "rtl",
      "created_at": null
    }
  ],
  "has_more_before": true,
  "has_more_after": false,
  "oldest_cursor": "event_91",
  "newest_cursor": "event_92"
}
```

Acceptance Criteria:

```text
اولین load فقط آخرین ۳۰ پیام را بیاورد.
scroll به بالا، ۳۰ پیام قبلی را بیاورد.
پیام‌های جدید بدون reload کامل اضافه شوند.
```

---

## 5.7. `GET /api/events`

SSE برای update سبک.

Event نمونه:

```text
event: session_updated
data: {"session_id":"abc","updated_at":"..."}
```

رفتار:

```text
وقتی hook جدید آمد، به browser خبر بده.
اگر browser در سشن دیگری است، notification نشان بده.
```

Acceptance Criteria:

```text
بدون refresh دستی، UI از سشن جدید باخبر شود.
اگر SSE قطع شد، browser reconnect کند.
CPU idle بالا نرود.
```

---

## 5.8. `POST /api/hook`

ورودی:

```json
{
  "session_id": "abc",
  "transcript_path": "/home/user/.codex/sessions/...",
  "cwd": "/home/user/project",
  "updated_at": "2026-05-14T10:30:00Z"
}
```

رفتار:

```text
event را validate کند.
session index را update کند.
latest_session_id را update کند.
SSE notification بفرستد.
```

Acceptance Criteria:

```text
اگر transcript_path وجود ندارد، session را invalid علامت بزند ولی server crash نکند.
اگر event تکراری است، duplicate نسازد.
اگر session_id خالی است، از hash transcript_path استفاده کند.
```

---

# 6. UI Specification

## 6.1. Layout کلی

صفحه اصلی:

```text
┌──────────────────────────────────────────┐
│ Header: Codex RTL Viewer                 │
├───────────────┬──────────────────────────┤
│ Sessions list │ Chat view                │
│               │                          │
│ project A     │ user message             │
│ project B     │ assistant message        │
│ project C     │ tool output              │
└───────────────┴──────────────────────────┘
```

روی موبایل:

```text
لیست سشن‌ها collapsible باشد.
چت تمام عرض شود.
```

---

## 6.2. Session List

هر آیتم:

```text
نام پروژه
مسیر cwd کوتاه‌شده
زمان آخرین آپدیت
preview آخرین پیام
badge اگر سشن همین الان فعال شد
```

نمونه:

```text
reaction-service
~/Projects/reaction-service
همین الان
مشکل از TypeError است...
```

Tasks:

```text
UI-001: ساخت sidebar لیست سشن‌ها
UI-002: اضافه کردن search/filter
UI-003: نمایش active session
UI-004: نمایش last updated
UI-005: نمایش preview
```

---

## 6.3. Chat View

هر پیام باید شبیه چت باشد.

پیام‌ها:

```text
user
assistant
tool
system
error
```

قانون نمایش:

```text
پیام‌های user و assistant اصلی‌تر باشند.
tool outputها collapsible باشند.
command/error/code با LTR render شوند.
متن فارسی RTL render شود.
```

Tasks:

```text
UI-010: ساخت message bubble برای user
UI-011: ساخت message bubble برای assistant
UI-012: ساخت block برای tool output
UI-013: ساخت code/error block LTR
UI-014: copy button برای هر پیام
UI-015: collapse/expand برای tool output
```

---

## 6.4. Lazy Load Chat

رفتار:

```text
اولین بار آخرین ۳۰ پیام.
وقتی scroll به بالای container رسید، ۳۰ پیام قبلی load شود.
بعد از load، scroll position حفظ شود.
پیام‌های جدید پایین اضافه شوند.
```

Tasks:

```text
UI-020: پیاده‌سازی initial load آخرین پیام‌ها
UI-021: IntersectionObserver برای بالای چت
UI-022: API call برای before cursor
UI-023: حفظ scroll position بعد از prepend
UI-024: نمایش loading spinner کوچک
UI-025: جلوگیری از درخواست تکراری همزمان
```

Acceptance Criteria:

```text
سشن با ۵۰۰۰ پیام browser را قفل نکند.
DOM همزمان بیشتر از مثلاً ۲۰۰ message نگه ندارد، یا حداقل در MVP بیشتر از ۵۰۰ نشود.
scroll به بالا روان باشد.
```

---

## 6.5. New Session Notification

این بخش دقیقاً طبق خواسته تو.

سناریو:

```text
کاربر در /session/A است.
server event می‌دهد: session_updated برای B.
اگر B != A:
    notification نشان بده.
```

متن:

```text
سشن جدیدی همین الان فعال شد.
تا ۵ ثانیه دیگر به آن منتقل می‌شوی.
```

دکمه‌ها:

```text
[لغو]
[همین الان برو]
```

رفتار:

```text
اگر کاربر روی «همین الان برو» زد:
    navigate به /session/B

اگر کاربر روی «لغو» زد:
    notification بسته شود.
    برای همین event دیگر redirect نکند.

اگر کاربر هیچ کاری نکرد:
    بعد از countdown برود به /session/B
```

Tasks:

```text
UI-030: دریافت event از SSE
UI-031: تشخیص اینکه event مربوط به سشن فعلی هست یا نه
UI-032: ساخت notification toast
UI-033: countdown 5 ثانیه‌ای
UI-034: دکمه cancel
UI-035: دکمه go now
UI-036: جلوگیری از redirect ناگهانی
UI-037: اگر چند event پشت سر هم آمد، آخرین session هدف شود
```

Acceptance Criteria:

```text
هیچ redirect ناگهانی بدون notification رخ ندهد.
کاربر بتواند redirect را لغو کند.
اگر کاربر در همان سشن فعلی است، فقط پیام‌های جدید append شوند و notification redirect نیاید.
```

---

# 7. RTL/LTR Rendering Specification

## 7.1. تشخیص direction

برای هر message یا block:

```text
اگر اکثریت حروف فارسی/عربی است:
    direction = rtl

اگر code/path/stack trace/json/sql/command است:
    direction = ltr

اگر mixed است:
    direction = auto
```

تابع پیشنهادی:

```text
detectDirection(text):
    if isCodeLike(text): return "ltr"
    persianCount = count chars in \u0600-\u06FF
    latinCount = count A-Z/a-z
    if persianCount > latinCount: return "rtl"
    if latinCount > persianCount: return "ltr"
    return "auto"
```

---

## 7.2. CSS حیاتی

```css
.message-text {
  white-space: pre-wrap;
  word-break: break-word;
  unicode-bidi: plaintext;
}

.message-text.rtl {
  direction: rtl;
  text-align: right;
}

.message-text.ltr {
  direction: ltr;
  text-align: left;
}

.code-block {
  direction: ltr;
  text-align: left;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  white-space: pre-wrap;
}
```

Tasks:

```text
RTL-001: پیاده‌سازی detectDirection
RTL-002: تشخیص code-like content
RTL-003: CSS برای RTL/LTR/auto
RTL-004: تست با متن فارسی شروع‌شده با عدد
RTL-005: تست با path + توضیح فارسی
RTL-006: تست با stack trace
RTL-007: تست با SQL/JSON
```

---

# 8. Transcript Parser Specification

## 8.1. ورودی

فایل‌های JSONL مربوط به Codex session.

هر خط:

```text
یک JSON event
```

مشکل:

```text
schema ممکن است بین نسخه‌ها فرق کند.
پس parser باید tolerant باشد.
```

---

## 8.2. خروجی داخلی

ساختار normalize‌شده:

```go
type Message struct {
    ID        string
    SessionID string
    Role      string // user, assistant, tool, system, error
    Kind      string // message, command, output, error, reasoning, unknown
    Text      string
    Direction string // rtl, ltr, auto
    Index     int
    CreatedAt *time.Time
}
```

---

## 8.3. Parser Strategy

```text
خط‌به‌خط بخوان.
اگر JSON parse نشد، skip کن.
role را از fieldهای ممکن حدس بزن.
text را از fieldهای ممکن استخراج کن.
اگر text list بود، flatten کن.
اگر command/output بود، kind مناسب بده.
برای هر event یک ID پایدار بساز:
    session_id + line_number
```

Tasks:

```text
PARSER-001: ساخت line-by-line JSONL reader
PARSER-002: استخراج session metadata
PARSER-003: استخراج user messages
PARSER-004: استخراج assistant messages
PARSER-005: استخراج tool/command outputs
PARSER-006: تحمل schema ناشناخته
PARSER-007: ساخت stable message ID
PARSER-008: pagination بر اساس line index
PARSER-009: خواندن از انتهای فایل برای latest messages
```

Acceptance Criteria:

```text
اگر یک event ناشناخته بود، کل parser fail نشود.
اگر یک خط corrupt بود، skip شود.
برای transcript بزرگ، parser کل فایل را برای هر درخواست کامل decode نکند.
```

---

# 9. Performance Specification

چون server همیشه روشن است، باید سبک باشد.

## 9.1. Idle

هدف:

```text
CPU نزدیک صفر
RAM کمتر از ۳۰MB در حالت عادی
```

## 9.2. Transcript بزرگ

فرض:

```text
یک سشن ممکن است هزاران event داشته باشد.
```

نباید:

```text
در هر refresh کل فایل parse و کل HTML render شود.
```

باید:

```text
metadata cache شود.
برای پیام‌ها pagination انجام شود.
در MVP می‌شود ساده‌تر بود، ولی API باید از اول pagination داشته باشد.
```

Tasks:

```text
PERF-001: cache کردن session index
PERF-002: جلوگیری از parse کامل در /api/sessions
PERF-003: خواندن tail فایل برای آخرین پیام‌ها
PERF-004: pagination
PERF-005: benchmark با transcript مصنوعی ۱۰هزار event
PERF-006: اندازه‌گیری RAM/CPU
```

Acceptance Criteria:

```text
باز کردن UI با ۱۰۰ سشن زیر ۵۰۰ms باشد.
باز کردن یک سشن بزرگ browser را freeze نکند.
server در idle request loop سنگین نداشته باشد.
```

---

# 10. State Storage

## MVP

مسیرها:

```text
~/.cache/codex-rtl/state.json
~/.cache/codex-rtl/sessions.json
~/.cache/codex-rtl/pending-events.jsonl
```

ساختار session index:

```json
{
  "sessions": {
    "abc": {
      "session_id": "abc",
      "transcript_path": "...",
      "cwd": "...",
      "project_name": "reaction-service",
      "updated_at": "...",
      "last_preview": "..."
    }
  },
  "latest_session_id": "abc"
}
```

Tasks:

```text
STATE-001: تعریف state dir
STATE-002: load state on startup
STATE-003: atomic write state
STATE-004: pending events fallback
STATE-005: dedupe session events
STATE-006: cleanup missing transcript files
```

Atomic write یعنی:

```text
اول state.tmp بنویس
بعد rename کن به state.json
```

---

# 11. Security Specification

چون local server است، باید امن باشد.

## قوانین

```text
به‌صورت پیش‌فرض فقط 127.0.0.1
هیچ telemetry
هیچ upload
هیچ external request
هیچ execution از browser input
مسیر فایل فقط از hook trusted یا config local
```

Tasks:

```text
SEC-001: bind فقط روی 127.0.0.1
SEC-002: جلوگیری از path traversal در /session/{id}
SEC-003: escape کردن HTML
SEC-004: CSP header ساده
SEC-005: عدم اجرای raw HTML از transcript
SEC-006: optional token برای API hook در آینده
```

Acceptance Criteria:

```text
اگر transcript شامل <script> باشد، اجرا نشود.
اگر کاربر URL دستکاری کند، فایل arbitrary خوانده نشود.
```

---

# 12. Installation Specification

## روش نصب عمومی

برای GitHub release:

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/codex-rtl/main/install.sh | bash
```

یا:

```bash
go install github.com/OWNER/codex-rtl@latest
```

## install.sh

Tasks:

```text
INSTALL-001: تشخیص OS/ARCH
INSTALL-002: دانلود binary مناسب
INSTALL-003: نصب در ~/.local/bin
INSTALL-004: چک کردن PATH
INSTALL-005: اجرای codex-rtl install
INSTALL-006: پیشنهاد اجرای codex-rtl server
```

Acceptance Criteria:

```text
روی Linux نصب شود.
اگر ~/.local/bin در PATH نیست، پیام واضح بدهد.
نصب بدون sudo ممکن باشد.
```

---

# 13. Release Plan

## Milestone 0: Skeleton

هدف:

```text
پروژه compile شود و command پایه کار کند.
```

Tasks:

```text
M0-001: ساخت repo
M0-002: انتخاب نام نهایی
M0-003: ساخت Go module
M0-004: ساخت CLI skeleton
M0-005: ساخت README اولیه
M0-006: ساخت Makefile
```

Definition of Done:

```text
go build کار کند.
codex-rtl --help خروجی بدهد.
```

---

## Milestone 1: Hook + State

هدف:

```text
hook eventها ثبت شوند.
```

Tasks:

```text
M1-001: پیاده‌سازی codex-rtl hook
M1-002: خواندن stdin JSON
M1-003: validate کردن transcript_path
M1-004: ساخت session_id fallback
M1-005: نوشتن state.json
M1-006: نوشتن pending-events وقتی server خاموش است
M1-007: تست دستی با echo JSON | codex-rtl hook
```

Definition of Done:

```text
بدون server هم hook event ذخیره شود.
هیچ crash روی input خراب رخ ندهد.
```

---

## Milestone 2: Local Server

هدف:

```text
سرور 127.0.0.1:9090 بالا بیاید.
```

Tasks:

```text
M2-001: پیاده‌سازی codex-rtl server
M2-002: route /
M2-003: route /api/state
M2-004: route /api/sessions
M2-005: route /api/hook
M2-006: load pending events on startup
M2-007: graceful shutdown
```

Definition of Done:

```text
curl http://127.0.0.1:9090/api/state جواب بدهد.
POST /api/hook session index را update کند.
```

---

## Milestone 3: Transcript Parser

هدف:

```text
پیام‌های JSONL خوانده و normalize شوند.
```

Tasks:

```text
M3-001: ساخت package parser
M3-002: خواندن JSONL line-by-line
M3-003: extractText tolerant
M3-004: guessRole tolerant
M3-005: detect kind
M3-006: pagination اولیه
M3-007: API /api/session/{id}/messages
M3-008: unit test با JSONL fake
```

Definition of Done:

```text
آخرین ۳۰ پیام یک transcript برگردد.
before cursor کار کند.
```

---

## Milestone 4: UI اولیه

هدف:

```text
browser UI سشن‌ها و پیام‌ها را نشان دهد.
```

Tasks:

```text
M4-001: ساخت static index.html
M4-002: ساخت styles.css
M4-003: ساخت app.js
M4-004: fetch /api/sessions
M4-005: render session list
M4-006: fetch messages
M4-007: render chat bubbles
M4-008: empty state
M4-009: error state
```

Definition of Done:

```text
با باز کردن http://127.0.0.1:9090 آخرین سشن دیده شود.
متن فارسی خوانا باشد.
code/path LTR باشد.
```

---

## Milestone 5: Lazy Load

هدف:

```text
سشن‌های بزرگ روان نمایش داده شوند.
```

Tasks:

```text
M5-001: initial load آخرین ۳۰ پیام
M5-002: load older on scroll top
M5-003: حفظ scroll position
M5-004: جلوگیری از duplicate messages
M5-005: loading indicator
M5-006: تست با transcript بزرگ مصنوعی
```

Definition of Done:

```text
سشن ۱۰هزار event بدون freeze باز شود.
scroll به بالا پیام‌های قبلی را load کند.
```

---

## Milestone 6: Live Update + Notification

هدف:

```text
وقتی سشن جدید آمد، notification با cancel/go now نشان داده شود.
```

Tasks:

```text
M6-001: پیاده‌سازی /api/events با SSE
M6-002: ارسال event هنگام POST /api/hook
M6-003: اتصال browser به SSE
M6-004: اگر event برای session فعلی بود، پیام‌های جدید append شوند
M6-005: اگر event برای session جدید بود، toast نشان بده
M6-006: countdown 5s
M6-007: دکمه لغو
M6-008: دکمه همین الان برو
M6-009: اگر چند event آمد، آخرین event هدف باشد
```

Definition of Done:

```text
هیچ redirect ناگهانی رخ ندهد.
notification دقیقاً دو دکمه داشته باشد.
بعد از countdown، اگر cancel نشده بود، redirect شود.
```

---

## Milestone 7: Installer

هدف:

```text
نصب عمومی آسان شود.
```

Tasks:

```text
M7-001: codex-rtl install
M7-002: backup config.toml
M7-003: اضافه کردن hook config
M7-004: جلوگیری از duplicate
M7-005: codex-rtl uninstall
M7-006: install.sh
M7-007: README نصب
```

Definition of Done:

```text
کاربر جدید با یک command ابزار را نصب کند.
بعد از نصب، Codex hook eventها را ارسال کند.
```

---

## Milestone 8: Polish

هدف:

```text
ابزار قابل انتشار شود.
```

Tasks:

```text
M8-001: dark theme تمیز
M8-002: light theme اختیاری
M8-003: copy message
M8-004: collapse tool output
M8-005: search داخل سشن
M8-006: filter by project
M8-007: تنظیم port
M8-008: status page
M8-009: logs سبک
```

Definition of Done:

```text
ابزار برای استفاده روزمره آماده باشد.
README کامل باشد.
```

---

# 14. ساختار فایل پروژه

پیشنهادی برای Go:

```text
codex-rtl/
├── cmd/
│   └── codex-rtl/
│       └── main.go
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── server.go
│   │   ├── hook.go
│   │   ├── install.go
│   │   └── status.go
│   ├── server/
│   │   ├── server.go
│   │   ├── routes.go
│   │   ├── sse.go
│   │   └── api.go
│   ├── state/
│   │   ├── state.go
│   │   ├── store.go
│   │   └── pending.go
│   ├── parser/
│   │   ├── parser.go
│   │   ├── direction.go
│   │   └── parser_test.go
│   ├── installer/
│   │   ├── codex_config.go
│   │   └── backup.go
│   └── platform/
│       ├── open_linux.go
│       ├── open_darwin.go
│       └── open_windows.go
├── web/
│   ├── index.html
│   ├── app.js
│   └── styles.css
├── scripts/
│   └── install.sh
├── testdata/
│   └── sample-session.jsonl
├── README.md
├── Makefile
└── go.mod
```

---

# 15. اولویت اجرای واقعی

اگر بخواهیم واقعاً بسازیم، ترتیب درست این است:

```text
۱. CLI skeleton
۲. hook command
۳. state storage
۴. server ساده
۵. parser ساده
۶. UI ساده
۷. lazy load
۸. SSE notification
۹. installer
۱۰. polish
```

نه برعکس.

اشتباه خطرناک این است که اول برویم UI خیلی قشنگ بسازیم. اول باید زنجیره‌ی اصلی کار کند:

```text
Codex Hook
→ codex-rtl hook
→ local server/state
→ browser sees session
```

وقتی این زنجیره درست شد، UI را قشنگ می‌کنیم.

---

# 16. MVP نهایی دقیق

نسخه `v0.1.0` باید فقط این‌ها را داشته باشد:

```text
codex-rtl server
codex-rtl hook
codex-rtl install
UI با لیست سشن‌ها
نمایش پیام‌های یک سشن
RTL/LTR درست
lazy load ساده
notification هنگام سشن جدید
```

نباید در v0.1.0 دنبال این‌ها برویم:

```text
search پیشرفته
theme زیاد
SQLite
plugin واقعی
Electron
sync cloud
multi-user
authentication پیچیده
```

---

# 17. Definition of Done کل پروژه v0.1

پروژه وقتی v0.1 آماده است که:

```text
کاربر repo را نصب کند.
codex-rtl install را بزند.
codex-rtl server را اجرا کند.
برود به http://127.0.0.1:9090
داخل IDE/terminal با Codex کار کند.
بعد از هر پاسخ Codex، سشن در مرورگر آپدیت شود.
اگر سشن جدید آمد، notification با «لغو» و «همین الان برو» ظاهر شود.
پیام‌ها شبیه چت نمایش داده شوند.
پیام‌های قدیمی lazy-load شوند.
متن فارسی درست و code/path/error چپ‌به‌راست نمایش داده شود.
هیچ توکنی مصرف نشود.
server در idle سبک بماند.
```

این می‌شود specification تمیز پروژه.
