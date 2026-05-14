# TASKS-V3 - Bug Audit and Fix Plan

این سند نتیجه بازبینی پیاده سازی فعلی در برابر `PROBLEM_STATEMENT.md`، `TASKS.md` و `TASKS-V2.md` است.

## وضعیت اجرای فعلی

فرمان های اجرا شده:

```bash
go test ./...
go build ./...
make build
go vet ./...
gofmt -l .
```

نتیجه:

- در workspace فعلی، `go test ./...`، `go build ./...`، `make build` و `go vet ./...` پاس می شوند.
- در archive تمیز Git، `make build` fail می شود، چون `cmd/ai-session-viewer` در Git track نشده و توسط `.gitignore` نادیده گرفته شده است.
- `gofmt -l .` چند فایل Go را گزارش می کند؛ یعنی سورس فعلی format استاندارد Go ندارد.
- تست دستی `hook --agent codex` وقتی server خاموش است event را در pending ذخیره می کند.
- تست دستی `POST /api/hook` با `session_id` خالی موفق می شود و یک session با key خالی می سازد؛ این خلاف spec است.
- تست دستی server با `state.json` خراب باعث crash در startup می شود؛ این خلاف acceptance criteria است.

---

## مشکلات اصلی پیدا شده

### P0-001: Build واقعی release خراب است

فایل ها:

- `.gitignore`
- `Makefile`
- `cmd/ai-session-viewer/main.go`

مشکل:

`.gitignore` الگوی `ai-session-viewer` را بدون slash ریشه دارد. این الگو علاوه بر binary ریشه، دایرکتوری `cmd/ai-session-viewer` را هم ignore کرده است. در نتیجه entrypoint برنامه در Git track نشده و build روی clone/archive تمیز fail می شود:

```text
stat /tmp/.../cmd/ai-session-viewer: directory not found
```

اثر:

release، GoReleaser و هر کاربری که repo را clone کند binary قابل build ندارد.

تسک:

- `.gitignore` را به الگوی ریشه ای مثل `/ai-session-viewer` و `/codex-rtl` اصلاح کن.
- `cmd/ai-session-viewer/main.go` را به Git اضافه کن.
- CI را طوری تغییر بده که `make build` و preferably `make build-all` هم اجرا شوند، نه فقط `go test ./...`.

Acceptance:

- `git ls-files cmd/ai-session-viewer/main.go` فایل را نشان دهد.
- `git archive HEAD | tar -x ... && make build` پاس شود.
- `go test ./...` همچنان پاس شود.

---

### P0-002: معماری نهایی با README/plugin قدیمی قاطی شده است

فایل ها:

- `README.md`
- `.codex-plugin/plugin.json`
- `skills/SKILL.md`
- `scripts/render_html.py`
- `internal/cli/*`

مشکل:

مستندات و plugin manifest هنوز مدل قدیمی Skill/Python را تبلیغ می کنند، اما کد اصلی به سمت binary Go با local server رفته است. این خلاف اصل zero-token در `TASKS.md` و `TASKS-V2.md` است، چون Skill باعث تعامل مدل و اجرای دستور از مسیر prompt می شود.

اثر:

کاربر طبق README مسیر اشتباه نصب می رود، توکن مصرف می شود، و قابلیت server/hook واقعی اصلا از plugin manifest قابل کشف نیست.

تسک:

- تصمیم محصولی را نهایی کن: ابزار اصلی باید `ai-session-viewer` باشد یا `codex-rtl`.
- README را بر اساس CLI جدید بازنویسی کن: `install`, `server`, `hook`, `open`, `status`.
- Skill و Python renderer را یا حذف کن، یا واضح به عنوان legacy/unsupported جدا کن.
- `.codex-plugin/plugin.json` را با واقعیت پروژه هم راستا کن، یا اگر plugin-native path نداریم، آن را حذف/غیرفعال کن تا کاربر را گمراه نکند.

Acceptance:

- README دیگر `pip install markdown` و `@html` را به عنوان مسیر اصلی معرفی نکند.
- هیچ مسیر اصلی محصول به LLM prompt یا Skill وابسته نباشد.
- یک کاربر با README بتواند binary را بسازد، server را بالا بیاورد، hook نصب کند و UI را باز کند.

---

### P0-003: Parser پیام های واقعی Codex/Claude/Gemini را درست normalize نمی کند

فایل ها:

- `internal/parser/parser.go`
- `scripts/render_html.py`
- `TASKS.md`
- `TASKS-V2.md`

مشکل:

`scripts/render_html.py` برای Codex دنبال schema این شکلی است:

```json
{"type":"event_msg","payload":{"type":"user_message","message":"..."}}
```

اما parser Go فقط top-levelهای `content`, `text`, `output`, `user`, `assistant` یا `event_type` را می خواند. برای transcript واقعی Codex، این کد اغلب کل event را به عنوان `raw` نمایش می دهد، نه متن user/assistant را.

برای Claude هم transcriptها معمولا nested `message.role` و `message.content` دارند، ولی parser فعلی nested object/array را flatten نمی کند. برای Gemini هم adapter defensive تعریف شده، اما parser agent-specific واقعی ندارد.

اثر:

UI به جای چت خوانا، JSON خام یا پیام ناقص نشان می دهد. هدف اصلی پروژه عملا محقق نمی شود.

تسک:

- parser را agent-aware کن: `ParseMessages(agent, sessionID, transcriptPath, opts)`.
- Codex rollout JSONL را پشتیبانی کن: `event_msg.payload.user_message`, `agent_message`, command/tool events.
- Claude JSONL را پشتیبانی کن: `message.role`, `message.content` string/array/text blocks/tool_use/tool_result.
- Gemini transcript را با fallback قابل تست پیاده کن؛ اگر transcript ندارد metadata-only session برگردان.
- unknown eventها را crash نکن، اما به صورت پیش فرض raw JSON را در chat اصلی spam نکن.
- unit test با fixture واقعی/مصنوعی برای هر agent اضافه کن.

Acceptance:

- fixture Codex با user/assistant/tool به پیام های normalize شده تبدیل شود.
- fixture Claude با content array درست flatten شود.
- transcript بدون مسیر معتبر UI را crash ندهد و حالت metadata-only بدهد.
- raw unknown فقط در حالت debug یا collapsible نمایش داده شود.

---

### P0-004: State خراب server را crash می دهد

فایل ها:

- `internal/state/store.go`
- `internal/cli/server.go`

مشکل:

اگر `state.json` خراب باشد، `LoadState` خطا برمی گرداند و `server` با `log.Fatalf` خارج می شود. در spec گفته شده server نباید با state خراب crash کند.

اثر:

یک write نیمه کاره یا edit دستی می تواند کل ابزار را از کار بیندازد.

تسک:

- `LoadState` در صورت JSON خراب، فایل خراب را به `state.json.corrupt.<timestamp>` منتقل کند.
- state خالی سالم بسازد و warning بدهد.
- مسیر corrupt recovery را test کن.

Acceptance:

- با `state.json` خراب، server بالا بیاید و `/api/state` جواب بدهد.
- فایل خراب برای debug حفظ شود.
- APIها 500 غیرضروری ندهند.

---

### P0-005: Hook/API validation ندارد و session خالی می سازد

فایل ها:

- `internal/cli/hook.go`
- `internal/server/api.go`
- `internal/state/state.go`
- `internal/state/pending.go`
- `internal/adapters/codex/codex.go`
- `internal/adapters/claude/claude.go`
- `internal/adapters/gemini/gemini.go`

مشکل:

`POST /api/hook` event بدون `session_id` را قبول می کند و در `Sessions[""]` ذخیره می کند. Agent هم در `SessionMeta` ذخیره نمی شود. `ProcessPendingEvents` هم همین مشکل را دارد. در Gemini fallback بدون داده، `unknown-session` ثابت می سازد که باعث collision می شود.

اثر:

sessionها overwrite/collide می شوند، چند agent با ID مشابه همدیگر را خراب می کنند، و UI sessionهای بی معنی نشان می دهد.

تسک:

- تابع واحد `NormalizeAndValidateEvent(event)` بساز.
- اگر `session_id` خالی است، از hash پایدار `agent + transcript_path + cwd` استفاده کن.
- اگر `transcript_path` خالی یا missing است، session را invalid/metadata-only علامت بزن، نه اینکه parse کند.
- `Agent` را در همه مسیرها در `SessionMeta` ذخیره کن.
- کلید state را از `session_id` تنها به `agent + ":" + session_id` تغییر بده.
- Hook با input invalid همچنان exit 0 بدهد، ولی event نامعتبر را با ساختار امن ذخیره کند یا skip کند.

Acceptance:

- `POST /api/hook` با `session_id` خالی دیگر `Sessions[""]` نسازد.
- Codex/Claude/Gemini با session ID یکسان با هم collision نداشته باشند.
- pending eventها و online hook path دقیقا یک validation مشترک داشته باشند.

---

### P1-001: Hook به port ثابت 9090 hardcode شده و timeout ندارد

فایل:

- `internal/cli/hook.go`

مشکل:

`hook` همیشه به `http://127.0.0.1:9090/api/hook` POST می زند. اگر server با `--port` دیگری بالا باشد، hook فقط pending می نویسد. همچنین `http.Post` از client بدون timeout استفاده می کند.

اثر:

قابلیت `server --port` ناقص است و hook می تواند بیشتر از SLA صد میلی ثانیه گیر کند.

تسک:

- port/base URL را از config/state/env بخوان.
- `http.Client{Timeout: ...}` اضافه کن.
- fallback pending را برای non-200 و timeout تست کن.

Acceptance:

- `ai-session-viewer server --port 19090` و hook روی همان port کار کند.
- اگر server hang کند، hook سریع pending بنویسد و exit 0 بدهد.

---

### P1-002: Codex installer ممکن است config.toml کاربر را خراب کند

فایل:

- `internal/adapters/codex/codex.go`

مشکل:

installer بدون parse کردن TOML، یک `[features]` جدید append می کند. اگر config کاربر از قبل `[features]` داشته باشد، TOML duplicate table می تواند invalid شود. خطاهای `MkdirAll`, `WriteFile` و backup هم نادیده گرفته شده اند. uninstall اگر marker پایان missing باشد defensive نیست.

اثر:

نصب ابزار می تواند Codex config کاربر را خراب کند.

تسک:

- TOML parser اضافه کن و config را parse/merge کن.
- feature flag موجود را update کن، duplicate table نساز.
- backup errors را fatal کن.
- uninstall را marker-safe کن و اگر marker ناقص است پیام واضح بده.
- تست با config خالی، config دارای `[features]`، config دارای hooks موجود، و config invalid اضافه کن.

Acceptance:

- دوبار install duplicate نسازد.
- config موجود کاربر حفظ شود.
- invalid TOML قبل از تغییر متوقف شود.
- uninstall فقط block متعلق به خودش را حذف کند.

---

### P1-003: Claude/Gemini installer ممکن است hookهای کاربر را حذف یا duplicate کند

فایل ها:

- `internal/adapters/claude/claude.go`
- `internal/adapters/gemini/gemini.go`

مشکل:

در uninstall، اگر hook خودمان داخل یک object مشترک باشد، کل object حذف می شود و ممکن است hookهای دیگر کاربر هم حذف شوند. Gemini فقط AfterAgent را برای duplicate check بررسی می کند و ممکن است SessionEnd را duplicate کند. نصب/حذف با command absolute path یا args متفاوت هم robust نیست.

اثر:

config ابزارهای دیگر کاربر ممکن است خراب شود یا hook duplicate اجرا شود.

تسک:

- برای هر agent یک شناسه marker/name پایدار داشته باش.
- uninstall فقط inner hook خودمان را حذف کند، نه کل container کاربر را.
- duplicate check هم command و هم name/args را پوشش دهد.
- اگر container بعد از حذف خالی شد، فقط همان container ساخته شده توسط خودمان حذف شود.
- unit test با settings واقعی و hookهای دیگر کاربر اضافه کن.

Acceptance:

- install دوباره duplicate نسازد.
- uninstall hookهای unrelated را حفظ کند.
- Gemini AfterAgent و SessionEnd هر دو جداگانه idempotent باشند.

---

### P1-004: API messages روی transcript نامعتبر 500 می دهد

فایل:

- `internal/server/api.go`

مشکل:

اگر `TranscriptPath` خالی یا missing باشد، `/api/session/{id}/messages` مستقیم `parser.ParseMessages` را صدا می زند و 500 می دهد.

اثر:

برای Gemini یا هر agent بدون transcript، UI error قرمز نشان می دهد به جای حالت قابل فهم.

تسک:

- برای session invalid/metadata-only پاسخ 200 با `items: []` و `status: "metadata_only"` بده.
- خطاهای file not found را به response ساختاریافته تبدیل کن.
- UI پیام واضح نمایش دهد: transcript قابل خواندن نیست.

Acceptance:

- session بدون transcript در UI crash/error خام ندهد.
- API برای missing transcript 500 ندهد مگر خطای داخلی واقعی باشد.

---

### P1-005: Pagination و performance فقط ظاهری است

فایل ها:

- `internal/parser/parser.go`
- `internal/server/api.go`
- `web/app.js`

مشکل:

parser برای هر request کل فایل را از اول scan و decode می کند. UI فقط آخرین ۳۰ پیام را load می کند و هیچ lazy-load هنگام scroll ندارد. `after`, `cursor`, `direction` مطابق spec پیاده نشده اند. `bufio.Scanner` هم buffer پیش فرض 64KB دارد و پیام های بزرگ را قطع/fail می کند، ولی `scanner.Err()` چک نمی شود.

اثر:

روی transcript بزرگ، server و browser کند می شوند و پیام های بلند ممکن است silently حذف شوند.

تسک:

- `scanner.Buffer` را افزایش بده یا reader line-based بدون محدودیت عملی بساز.
- `scanner.Err()` را handle کن.
- برای latest messages از tail/index استفاده کن یا cache line offsets بساز.
- API cursor-based واقعی بساز: `before`, `after`, `limit`, `direction`.
- UI prepend on scroll-top با حفظ scroll position پیاده کند.
- benchmark با transcript حداقل ۱۰ هزار event اضافه کن.

Acceptance:

- باز کردن session بزرگ freeze نکند.
- scroll به بالا پیام های قدیمی تر را load کند.
- پیام بزرگ بالای 64KB از دست نرود.

---

### P1-006: Markdown rendering و syntax highlighting وجود ندارد

فایل ها:

- `web/app.js`
- `web/index.html`
- `web/styles.css`

مشکل:

UI متن پیام را escape می کند و با `innerHTML` قرار می دهد، اما Markdown را render نمی کند. جدول، fenced code و inline code به شکل raw markdown نمایش داده می شوند. این خلاف نیاز اصلی پروژه است.

اثر:

خوانایی فارسی/RTL بهتر می شود، اما table/code که مشکل اصلی بودند همچنان ناقص می مانند.

تسک:

- Markdown renderer client-side یا server-side انتخاب کن.
- output را sanitize کن؛ raw HTML پیام نباید اجرا شود.
- code blockها را LTR، copyable و optionally highlighted کن.
- tableها را responsive و RTL-safe کن.

Acceptance:

- fixture دارای table به جدول HTML امن تبدیل شود.
- fenced code با direction LTR نمایش داده شود.
- raw `<script>` در پیام اجرا نشود.

---

### P1-007: XSS محلی از session list ممکن است

فایل:

- `web/app.js`

مشکل:

`session.project_name` با template string داخل `innerHTML` قرار می گیرد و escape نمی شود. project name از `cwd` می آید و ممکن است شامل HTML باشد.

اثر:

یک مسیر پروژه crafted می تواند JavaScript در viewer local اجرا کند.

تسک:

- session list را با `textContent` و DOM node بساز.
- هرجایی که data خارجی وارد DOM می شود rule واحد escape/sanitize داشته باش.
- تست UI/DOM برای project name شامل `<img onerror=...>` اضافه کن.

Acceptance:

- project name با HTML به صورت متن نمایش داده شود، نه اجرا.

---

### P1-008: Notification رفتار cancel را کامل رعایت نمی کند

فایل:

- `web/app.js`

مشکل:

spec می گوید اگر کاربر redirect event را cancel کرد، برای همان event دیگر redirect نشود. کد فعلی event ID ندارد و فقط timer را clear می کند. اگر event مشابه دوباره برسد، toast دوباره redirect می کند. بعد از redirect با toast، active item sidebar هم sync نمی شود.

اثر:

UX تغییر session قابل اعتماد نیست.

تسک:

- SSE event id یا ترکیب `session_id + updated_at` را به عنوان event key نگه دار.
- canceled eventها را در حافظه tab ignore کن.
- بعد از redirect، sidebar active state را sync کن.

Acceptance:

- cancel برای همان event redirect بعدی ایجاد نکند.
- بعد از "همین الان برو" sidebar هم session جدید را active نشان دهد.

---

### P2-001: Commands ناقص مانده اند

فایل ها:

- `internal/cli/open.go`
- `internal/cli/status.go`

مشکل:

`open --start-server` فقط پیام "not yet integrated" چاپ می کند. `status` فقط header چاپ می کند و هیچ وضعیت واقعی نشان نمی دهد.

تسک:

- `status` را پیاده کن: server health, URL, state dir, session count, latest session, hook install status per agent.
- `open --start-server` را یا کامل پیاده کن، یا flag را حذف کن تا وعده دروغ ندهد.
- port/base URL مشترک را با server/hook/open هماهنگ کن.

Acceptance:

- `ai-session-viewer status` خروجی مفید و قابل script داشته باشد.
- `open` اگر server down است پیام دقیق بدهد.

---

### P2-002: UI empty/mobile/accessibility ناقص است

فایل ها:

- `web/index.html`
- `web/styles.css`
- `web/app.js`

مشکل:

empty state واقعی برای نبود session ضعیف است، layout موبایل collapsible نیست، copy/collapse tool output وجود ندارد، و sidebar فقط اطلاعات کم نشان می دهد.

تسک:

- empty state واضح با راهنمای `server`, `install`, `hook` اضافه کن.
- mobile layout با sidebar collapsible بساز.
- tool output را collapsible کن.
- copy button برای پیام ها اضافه کن.
- session list شامل cwd کوتاه، agent badge، last preview و invalid badge باشد.

Acceptance:

- روی موبایل chat قابل استفاده باشد.
- session metadata مطابق spec دیده شود.

---

### P2-003: gofmt رعایت نشده است

فایل ها:

- خروجی فعلی `gofmt -l .` چند فایل Go را نشان می دهد.

تسک:

- `gofmt -w` روی سورس Go اجرا کن.
- CI را با check format سخت گیر کن.

Acceptance:

- `gofmt -l .` خروجی خالی بدهد.

---

## Milestoneهای پیشنهادی برای اصلاح

### Milestone 1: Build and Repository Hygiene

Tasks:

- اصلاح `.gitignore` برای ignore نکردن `cmd/ai-session-viewer`.
- track کردن entrypoint binary.
- اضافه کردن `make build` به CI.
- اجرای `gofmt`.
- حذف/نادیده گرفتن binaryهای local مثل `dist/` و root binary.

Definition of Done:

- clean archive build پاس شود.
- CI فقط با `go test` محدود نباشد.

### Milestone 2: Product Alignment

Tasks:

- انتخاب نام نهایی: `ai-session-viewer` یا `codex-rtl`.
- بازنویسی README براساس Go CLI.
- تعیین تکلیف plugin/skill/python legacy.
- مستندسازی نصب برای Codex/Claude/Gemini.

Definition of Done:

- هیچ مسیر اصلی استفاده باعث مصرف توکن نشود.
- مستندات با binary فعلی همخوان باشد.

### Milestone 3: Safe Hook and State Core

Tasks:

- ساخت validation/fallback مشترک برای HookEvent.
- پشتیبانی از corrupt state recovery.
- ذخیره agent و invalid/metadata-only status در SessionMeta.
- استفاده از key مرکب agent/session.
- اضافه کردن HTTP timeout و configurable port.

Definition of Done:

- event خالی session خالی نسازد.
- state خراب server را نخواباند.
- hook همیشه زیر timeout مشخص exit 0 بدهد.

### Milestone 4: Installer Hardening

Tasks:

- Codex TOML merge امن.
- Claude/Gemini JSON merge امن.
- uninstall بدون حذف hookهای دیگر.
- idempotency test برای install/uninstall.

Definition of Done:

- هیچ config معتبر کاربر بعد از install/uninstall خراب نشود.
- backup فقط وقتی موفق و قابل اتکا است تغییر اصلی انجام شود.

### Milestone 5: Real Transcript Parsing

Tasks:

- fixtureهای Codex/Claude/Gemini اضافه کن.
- parser agent-aware بساز.
- content array و nested payload را پشتیبانی کن.
- scanner limit و خطای scanner را fix کن.
- raw unknown را کنترل شده و collapsible کن.

Definition of Done:

- آخرین ۳۰ پیام واقعی هر agent به صورت چت readable نمایش داده شود.
- missing transcript حالت metadata-only بدهد.

### Milestone 6: API and Performance

Tasks:

- `/api/sessions` با sort, limit, offset, project filter, preview.
- `/api/session/{id}/messages` با cursor واقعی.
- index/cache سبک برای session metadata.
- benchmark transcript بزرگ.

Definition of Done:

- session list سریع load شود.
- transcript بزرگ باعث parse کامل مکرر نشود.

### Milestone 7: UI Correctness and RTL

Tasks:

- Markdown امن با sanitize.
- code/table rendering صحیح.
- lazy load هنگام scroll.
- XSS fix در session list.
- mobile responsive و metadata-only/error states.
- toast cancel با event key.

Definition of Done:

- متن فارسی RTL، کد LTR، table خوانا و safe باشد.
- UI بدون refresh دستی session جدید را درست اطلاع دهد.

### Milestone 8: Test Matrix

Tasks:

- unit test parser برای هر agent.
- unit test installer با temp HOME/project.
- integration test server + hook + pending.
- test corrupt state recovery.
- test missing transcript.
- test clean archive build.

Definition of Done:

- `go test ./...` coverage مسیرهای اصلی را پوشش دهد.
- CI روی Linux/macOS/Windows حداقل build/test را اجرا کند.

---

## فرمان های نهایی که باید بعد از اصلاح پاس شوند

```bash
gofmt -l .
go vet ./...
go test ./...
make build
make build-all
```

برای build تمیز:

```bash
tmp=$(mktemp -d)
git archive HEAD | tar -x -C "$tmp"
cd "$tmp"
make build
```

برای smoke test hook/server:

```bash
tmp=$(mktemp -d)
HOME="$tmp" ./dist/ai-session-viewer hook --agent codex <<'JSON'
{"session_id":"s1","transcript_path":"/tmp/fake.jsonl","cwd":"/tmp/project"}
JSON

HOME="$tmp" ./dist/ai-session-viewer server --port 9090
```
