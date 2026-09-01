# برنامه انتقال Codex Manager به Abolqasem

این فایل برنامه اجرایی و منبع پیگیری کار است. هر تسک فقط وقتی تیک می‌خورد که معیار پذیرش و تست‌های همان تسک کامل شده باشند.

## Phase OpenCode (افزوده‌شده در 1.5.0)

- [x] OC-001 — شناسایی executable، کشف مدل‌ها و نمایش وضعیت نصب OpenCode در provider picker.
- [x] OC-002 — اجرای turn از CLI با model/variant، resume، fork و cancellation.
- [x] OC-003 — خواندن `session list` و `export`، cache امن transcript و import پیام‌های user/assistant.
- [x] OC-004 — اتصال runtime و تنظیمات UI؛ OpenCode کنار Codex و Claude قابل انتخاب است.
- [x] OC-005 — fork و تبدیل transcript بین providerها، آزمون‌های Go/TypeScript، build و نصب سرویس محلی.

## تصمیم‌های قطعی معماری

- Rust gateway به‌صورت sidecar باینری در کنار Abolqasem توزیع و توسط خود Abolqasem مدیریت می‌شود.
- هیچ runtime پایتونی همراه محصول نخواهد بود؛ منطق فعلی `codex_manager/*.py` با تست برابری رفتار به Go منتقل می‌شود.
- حالت «Codex Manager» یک گزینه ساده است: کاربر با یک تیک آن را فعال می‌کند، مدل‌های Codex بدون alias یا mapping عبور می‌کنند و تنظیمات mapping در این حالت نمایش داده نمی‌شود.
- حالت «Custom Provider» مستقل است و تنظیمات base URL، احراز هویت، header، کشف مدل و mapping اختیاری مدل/قابلیت‌ها را ارائه می‌کند.
- `codex app-server` همچنان agent runtime، ابزارها، sandbox، thread و transcript را مدیریت می‌کند؛ Rust sidecar فقط مسیر Responses API و انتخاب حساب را مدیریت می‌کند.
- Go مالک account store، token refresh، limits، recommendation، history، device login، Chrome discovery و session cleanup است.
- secretها و cookieها هرگز به frontend یا log ارسال نمی‌شوند. frontend فقط وضعیت redacted را می‌بیند.
- account binding هر chat باید پایدار باشد؛ تغییر حساب وسط یک turn ممنوع است و retry/switch فقط در مرز request و بر اساس policy انجام می‌شود.

## تجربه کاربری مورد انتظار

### حالت Codex Manager

1. کاربر گزینه «مدیریت هوشمند حساب‌های Codex» را روشن می‌کند.
2. Abolqasem وجود sidecar را بررسی، secret محلی را تولید، gateway را اجرا و health check می‌کند.
3. app-serverهای جدید از provider داخلی `codex_manager` استفاده می‌کنند؛ sessionهای در حال اجرا قطع نمی‌شوند و از turn بعدی/بازگشایی session مهاجرت می‌کنند.
4. مدل و reasoning effort همان catalog بومی Codex است و هیچ فرم model mapping نمایش داده نمی‌شود.
5. UI حساب انتخاب‌شده، علت انتخاب یا switch، زمان آخرین limit refresh و سلامت gateway را نشان می‌دهد.

### حالت Custom Provider

1. کاربر provider را با نام، base URL، wire API، روش auth و headerها تعریف می‌کند.
2. کشف `/models` اختیاری است و مدل‌ها می‌توانند بدون mapping به upstream ارسال شوند.
3. mapping اختیاری فقط برای تغییر شناسه upstream، عنوان نمایشی و capabilityهای مدل مانند reasoning effort و input modality است.
4. تست اتصال و preview نگاشت پیش از ذخیره اجباری است؛ secret بعد از ذخیره مجدداً به UI برگردانده نمی‌شود.

## Phase 0: تثبیت قرارداد و ساخت

### [x] TASK-001 - ثبت ماتریس برابری Python به Go

**Dependencies**: None  
**Files**: `docs/codex-manager-parity.md`, `/home/h-mousavi/Projects/Hamed/codex-manager/codex_manager/**/*.py`  
**Criteria**: تمام رفتارهای auth، account CRUD، limits، recommendation، history، device login، Chrome profile/session و maintenance به ورودی/خروجی/خطاهای قابل تست تبدیل شده باشند؛ اجزای صرفاً TUI/رنگ/چاپ به‌عنوان خارج از scope ثبت شوند.

### [x] TASK-002 - واردکردن Rust gateway به‌عنوان sidecar داخلی پروژه

**Dependencies**: TASK-001  
**Files**: `sidecars/codex-manager-gateway/Cargo.toml`, `sidecars/codex-manager-gateway/Cargo.lock`, `sidecars/codex-manager-gateway/src/**`, `THIRD_PARTY_NOTICES.md`  
**Criteria**: gateway از مخزن codex-manager با تاریخچه منبع/مجوز مشخص منتقل شود، مستقل build شود و هیچ کد Python را import یا اجرا نکند.

### [x] TASK-003 - تعریف پروتکل و lifecycle بین Go و sidecar

**Dependencies**: TASK-002  
**Files**: `docs/codex-manager-sidecar-protocol.md`, `internal/codexmanager/sidecar/protocol.go`  
**Criteria**: envها، آرگومان‌ها، health/readiness، version handshake، shutdown، exit codeها، مسیر فایل‌ها، secret delivery و سازگاری نسخه‌ها مستند و machine-testable باشند.

### [x] TASK-004 - افزودن build چندسکویی sidecar

**Dependencies**: TASK-002, TASK-003  
**Files**: `Makefile`, `.goreleaser.yaml`, `.github/workflows/test.yml`, `.github/workflows/release.yml`, `scripts/build-from-source.sh`, `scripts/build-from-source.ps1`  
**Criteria**: Go binary و Rust sidecar برای Linux/macOS/Windows روی amd64/arm64 ساخته شوند؛ archive هر پلتفرم sidecar متناظر را داشته باشد و checksum/release preflight آن را تأیید کند.

### [x] TASK-005 - تعریف راهبرد مهاجرت و rollback داده‌ها

**Dependencies**: TASK-001  
**Files**: `docs/codex-manager-migration.md`  
**Criteria**: import از `~/.codex-manager`، مالکیت فایل‌ها، نسخه schema، backup، اجرای مجدد امن، rollback و عدم دست‌کاری `~/.codex/auth.json` در زمان turn فعال دقیقاً مشخص شده باشند.

## Phase 1: مدل‌ها، storage و قراردادهای پایه

### [x] TASK-006 - افزودن تنظیمات دو حالت Manager و Custom Provider

**Dependencies**: TASK-003, TASK-005  
**Files**: `internal/state/settings.go`, `internal/state/settings_test.go`, `web-react/src/shared/types.ts`  
**Criteria**: تنظیمات شامل mode=`native|manager|custom`، enable flag، endpointهای manager، auto-switch policy و providerهای سفارشی باشد؛ default برابر native و migration تنظیمات قبلی بدون از دست‌رفتن داده انجام شود.

### [x] TASK-007 - پیاده‌سازی secret store محلی

**Dependencies**: TASK-006  
**Files**: `internal/secrets/store.go`, `internal/secrets/store_test.go`, `internal/state/settings.go`  
**Criteria**: gateway key، custom provider key و credentialهای حساس با permission امن و atomic write ذخیره شوند؛ snapshot تنظیمات فقط `configured: true/false` برگرداند و هیچ secretی وارد JSON frontend یا log نشود.

### [x] TASK-008 - تعریف domain model حساب و هویت

**Dependencies**: TASK-001  
**Files**: `internal/codexmanager/account/model.go`, `internal/codexmanager/account/model_test.go`  
**Criteria**: account identity، email، account ID، plan، token expiry، state، active/pinned metadata و خطاهای typed تعریف و JSON schema داخلی تثبیت شوند.

### [x] TASK-009 - تعریف domain model محدودیت، توصیه و تاریخچه [P]

**Dependencies**: TASK-001  
**Files**: `internal/codexmanager/limits/model.go`, `internal/codexmanager/recommendation/model.go`, `internal/codexmanager/history/model.go`  
**Criteria**: windowهای 5-hour/weekly/monthly، remaining/reset، credits، recommendation reason/score و chart series بدون وابستگی به UI تعریف شوند.

### [x] TASK-010 - تعریف مدل Chrome profile و session [P]

**Dependencies**: TASK-001  
**Files**: `internal/codexmanager/browser/model.go`, `internal/codexmanager/browser/model_test.go`  
**Criteria**: profile، browser account، device/session، current-device protection و revoke result با وضعیت‌های partial/error تعریف شوند.

### [x] TASK-011 - تعریف provider سفارشی و model mapping

**Dependencies**: TASK-006  
**Files**: `internal/providers/custom/model.go`, `internal/providers/custom/model_test.go`, `web-react/src/shared/types.ts`  
**Criteria**: provider ID/name/base URL/wire API/auth/header/model discovery و mapping اختیاری `uiModelId -> upstreamModelId` با display name، reasoning efforts و modalities تعریف شوند؛ manager mode عمداً mapping field نداشته باشد.

### [x] TASK-012 - همگام‌سازی پروتکل Go با modelProvider بومی app-server

**Dependencies**: TASK-006  
**Files**: `internal/providers/codex/protocol/protocol.go`, `internal/providers/codex/protocol/protocol_test.go`  
**Criteria**: `modelProvider` در thread start/resume/fork و response metadata پشتیبانی شود و fixtureهای schema نسخه نصب‌شده Codex را پاس کند.

### [x] TASK-013 - ساخت fixtureهای golden از رفتار Python

**Dependencies**: TASK-001, TASK-008, TASK-009, TASK-010  
**Files**: `internal/codexmanager/testdata/**`, `internal/codexmanager/parity_test.go`  
**Criteria**: داده‌های بدون secret برای JWT claims، auth refresh، limits payload، recommendation، history و Chrome/session به golden fixture تبدیل شوند و امکان مقایسه خروجی Go با رفتار فعلی Python فراهم باشد.

## Phase 2: انتقال منطق Python به Go و تقویت Rust

### [x] TASK-014 - انتقال paths، atomic storage، backup و lock به Go

**Dependencies**: TASK-008, TASK-013  
**Files**: `internal/codexmanager/storage/paths.go`, `internal/codexmanager/storage/store.go`, `internal/codexmanager/storage/lock.go`, `internal/codexmanager/storage/*_test.go`  
**Criteria**: ایجاد directory، sanitize نام، read/write JSON، backup rotation و process lock با permission امن و سازگاری داده‌های قبلی انجام شوند.

### [x] TASK-015 - انتقال JWT parsing و قواعد هویت حساب به Go

**Dependencies**: TASK-008, TASK-013, TASK-014  
**Files**: `internal/codexmanager/auth/jwt.go`, `internal/codexmanager/auth/identity.go`, `internal/codexmanager/auth/*_test.go`  
**Criteria**: expiry، email/account/plan extraction، same-identity و promotion safety با fixtureهای Python برابر باشند و JWT malformed باعث panic نشود.

### [x] TASK-016 - انتقال refresh token و HTTP auth client به Go

**Dependencies**: TASK-007, TASK-015  
**Files**: `internal/codexmanager/auth/refresh.go`, `internal/codexmanager/auth/client.go`, `internal/codexmanager/auth/refresh_test.go`  
**Criteria**: refresh threshold، proxy، timeout، cancellation، identity revalidation، redaction و atomic persistence پیاده شوند؛ token هرگز log نشود.

### [x] TASK-017 - انتقال account repository و CRUD به Go

**Dependencies**: TASK-014, TASK-015, TASK-016  
**Files**: `internal/codexmanager/account/repository.go`, `internal/codexmanager/account/service.go`, `internal/codexmanager/account/*_test.go`  
**Criteria**: import/add/list/rename/delete/activate/sync با lock و backup کار کنند؛ حذف حساب pinned یا استفاده‌شده توسط turn فعال fail-safe باشد.

### [x] TASK-018 - انتقال دریافت و normalize کردن rate limits به Go [P]

**Dependencies**: TASK-009, TASK-013, TASK-016  
**Files**: `internal/codexmanager/limits/client.go`, `internal/codexmanager/limits/normalize.go`, `internal/codexmanager/limits/*_test.go`  
**Criteria**: payloadهای Codex، windowها، resetها، credits و reached state با golden fixture برابر و خطاهای auth/network قابل تشخیص باشند.

### [x] TASK-019 - انتقال recommendation و انتخاب بهترین حساب به Go

**Dependencies**: TASK-017, TASK-018  
**Files**: `internal/codexmanager/recommendation/service.go`, `internal/codexmanager/recommendation/service_test.go`  
**Criteria**: امتیازدهی، Free/Plus priority، stale/risk/save/best، علت انتخاب و tie-break قطعی با Python parity داشته باشند.

### [x] TASK-020 - انتقال history، retention و chart series به Go [P]

**Dependencies**: TASK-009, TASK-014, TASK-018  
**Files**: `internal/codexmanager/history/store.go`, `internal/codexmanager/history/series.go`, `internal/codexmanager/history/*_test.go`  
**Criteria**: append JSONL، prune، rename، timezone و aggregation بدون بارگذاری نامحدود فایل و با خروجی مناسب chart کار کنند.

### [x] TASK-021 - انتقال maintenance و scheduler داخلی به Go

**Dependencies**: TASK-017, TASK-018, TASK-019, TASK-020  
**Files**: `internal/codexmanager/maintenance/service.go`, `internal/codexmanager/maintenance/scheduler.go`, `internal/codexmanager/maintenance/*_test.go`  
**Criteria**: refresh/check/history/session jobs با context cancellation، jitter، single-flight، backoff و اجرای دستی کار کنند؛ دیگر cron/systemd متعلق به Python لازم نباشد.

### [x] TASK-022 - انتقال Device Code login مبتنی بر app-server به Go

**Dependencies**: TASK-012, TASK-014, TASK-017  
**Files**: `internal/codexmanager/login/device.go`, `internal/codexmanager/login/registry.go`, `internal/codexmanager/login/*_test.go`  
**Criteria**: start/poll/cancel/timeout و import حساب با `CODEX_HOME` موقت انجام شود؛ flow پس از restart قابل تعیین‌تکلیف و auth موقت پاک‌سازی شود.

### [x] TASK-023 - انتقال Chrome profile discovery و cookie loading به Go

**Dependencies**: TASK-010, TASK-013  
**Files**: `internal/codexmanager/browser/profiles.go`, `internal/codexmanager/browser/cookies.go`, `internal/codexmanager/browser/*_test.go`  
**Criteria**: مسیرهای Linux/macOS/Windows، Local State، profile name و cookie DB با copy read-only پشتیبانی شوند؛ browser DB اصلی هرگز lock یا mutate نشود.

### [x] TASK-024 - انتقال account switcher discovery از storage مرورگر به Go

**Dependencies**: TASK-023  
**Files**: `internal/codexmanager/browser/accounts.go`, `internal/codexmanager/browser/indexeddb.go`, `internal/codexmanager/browser/accounts_test.go`  
**Criteria**: حساب‌های ذخیره‌شده Chrome از Local Storage/IndexedDB استخراج، deduplicate و به managed accountها وصل شوند؛ خرابی یک profile اسکن بقیه را متوقف نکند.

### [x] TASK-025 - انتقال ChatGPT device/session client و revoke به Go

**Dependencies**: TASK-016, TASK-023  
**Files**: `internal/codexmanager/browser/session_client.go`, `internal/codexmanager/browser/session_client_test.go`  
**Criteria**: list/revoke با timeout و proxy کار کند، current device محافظت شود، عملیات destructive نیازمند target صریح باشد و پاسخ‌های 401/403 قابل اقدام گزارش شوند.

### [x] TASK-026 - انتقال session monitor و سیاست cleanup به Go

**Dependencies**: TASK-017, TASK-019, TASK-024, TASK-025  
**Files**: `internal/codexmanager/browser/monitor.go`, `internal/codexmanager/browser/monitor_test.go`  
**Criteria**: dry-run، disable per account، plan-aware limit، status recording و cleanup کنترل‌شده پیاده شوند؛ بدون opt-in هیچ sessionی revoke نشود.

### [x] TASK-027 - ساخت facade واحد Codex Manager در Go

**Dependencies**: TASK-021, TASK-022, TASK-026  
**Files**: `internal/codexmanager/manager.go`, `internal/codexmanager/manager_test.go`  
**Criteria**: lifecycle، account actions، usage، history، login و browser operations پشت interface واحد قرار گیرند و shutdown برنامه همه workerها را متوقف کند.

### [x] TASK-028 - پایدارکردن account binding در Rust sidecar

**Dependencies**: TASK-002, TASK-003, TASK-019  
**Files**: `sidecars/codex-manager-gateway/src/bindings.rs`, `sidecars/codex-manager-gateway/src/router.rs`, `sidecars/codex-manager-gateway/src/main.rs`  
**Criteria**: binding شناسه chat/thread به account روی دیسک پایدار باشد، restart آن را حفظ کند، یک request فقط از یک snapshot کاندیدا استفاده کند و switch reason ثبت شود.

### [x] TASK-029 - تکمیل retry، cooldown و health در Rust sidecar

**Dependencies**: TASK-028  
**Files**: `sidecars/codex-manager-gateway/src/router.rs`, `sidecars/codex-manager-gateway/src/health.rs`, `sidecars/codex-manager-gateway/tests/**`  
**Criteria**: 401/403/429 و خطاهای network طبقه‌بندی، account cooldown اعمال، retry محدود و قابل مشاهده باشد؛ readiness نسخه config/status را نیز بررسی کند و stream نیمه‌کاره روی حساب دیگر replay نشود.

## Phase 3: اتصال app-server، API و UI

### [x] TASK-030 - پیاده‌سازی supervisor برای Rust sidecar

**Dependencies**: TASK-004, TASK-007, TASK-027, TASK-029  
**Files**: `internal/codexmanager/sidecar/supervisor.go`, `internal/codexmanager/sidecar/supervisor_test.go`  
**Criteria**: locate/start/readiness/restart/backoff/graceful-stop و log redaction کار کنند؛ فقط یک sidecar برای state directory اجرا شود و crash loop در UI قابل مشاهده باشد.

### [x] TASK-031 - ساخت API تنظیمات و عملیات Codex Manager

**Dependencies**: TASK-006, TASK-007, TASK-027, TASK-030  
**Files**: `internal/server/codex_manager_api.go`, `internal/server/routes.go`, `internal/server/workspace_ws.go`, `internal/server/codex_manager_api_test.go`  
**Criteria**: snapshot، toggle، health، account CRUD، check، recommendation، history، login و Chrome session endpoints با validation/CSRF/authorization موجود پروژه ارائه شوند و پاسخ‌ها redacted باشند.

### [x] TASK-032 - پیاده‌سازی فعال‌سازی یک‌تیکی Manager

**Dependencies**: TASK-030, TASK-031  
**Files**: `internal/server/codex_manager_activation.go`, `internal/server/codex_manager_activation_test.go`  
**Criteria**: یک command اتمیک secret/config را آماده، sidecar را اجرا و health check کند؛ در شکست تنظیم قبلی حفظ و خطای قابل اقدام برگردد؛ خاموش‌کردن، turn فعال را خراب نکند.

### [x] TASK-033 - تولید config overlay اختصاصی برای Codex app-server

**Dependencies**: TASK-006, TASK-007, TASK-012, TASK-030  
**Files**: `internal/providers/codex/configoverlay/overlay.go`, `internal/providers/codex/configoverlay/overlay_test.go`, `internal/server/workspace_turn_starter.go`  
**Criteria**: app-server با config/CODEX_HOME کنترل‌شده اجرا شود، provider `codex_manager` به loopback sidecar و Responses API وصل باشد، secret فقط از env برسد و تنظیمات user موجود مانند MCP/skills از بین نروند.

### [x] TASK-034 - اتصال provider به lifecycle هر chat/session

**Dependencies**: TASK-012, TASK-032, TASK-033  
**Files**: `internal/server/workspace_turn_starter.go`, `internal/server/workspace_turn_starter_test.go`, `internal/workspace/agent/coordinator.go`  
**Criteria**: modelProvider در start/resume/fork ارسال شود؛ provider fingerprint جزو reuse key باشد؛ تغییر mode app-server نامتناسب را پس از turn ببندد و هر chat provider خود را حفظ کند.

### [x] TASK-035 - حفظ catalog بومی Codex در حالت Manager

**Dependencies**: TASK-034  
**Files**: `internal/server/workspace_provider_models.go`, `internal/providers/catalog/catalog.go`, `internal/server/workspace_provider_models_test.go`  
**Criteria**: حالت Manager همان مدل‌ها، reasoning effort و modalityهای Codex را نشان دهد؛ `/v1/models` sidecar باعث ساخت mapping یا نمایش فرم alias نشود و شناسه مدل بدون تغییر به upstream برسد.

### [x] TASK-036 - پیاده‌سازی discovery و mapping برای Custom Provider

**Dependencies**: TASK-011, TASK-033  
**Files**: `internal/providers/custom/client.go`, `internal/providers/custom/catalog.go`, `internal/providers/custom/mapping.go`, `internal/providers/custom/*_test.go`  
**Criteria**: `/models` اختیاری، model دستی، passthrough بدون mapping و mapping اختیاری کار کنند؛ collision، مدل حذف‌شده و capability ناقص validation شوند و upstream ID درست در request قرار گیرد.

### [x] TASK-037 - افزودن API تست و preview برای Custom Provider

**Dependencies**: TASK-031, TASK-036  
**Files**: `internal/server/custom_provider_api.go`, `internal/server/custom_provider_api_test.go`, `internal/server/routes.go`  
**Criteria**: تست health/models/responses با timeout، preview نهایی model ID/header و خطای redacted ارائه شود؛ ذخیره provider نامعتبر بدون تأیید صریح ممکن نباشد.

### [x] TASK-038 - ساخت UI تنظیمات Provider با progressive disclosure

**Dependencies**: TASK-032, TASK-035, TASK-037  
**Files**: `web-react/src/client/app/SettingsPage.tsx`, `web-react/src/client/components/settings/CodexBackendSettings.tsx`, `web-react/src/client/components/settings/CustomProviderEditor.tsx`, `web-react/src/shared/types.ts`, `web-react/src/client/app/SettingsPage.test.tsx`  
**Criteria**: toggle Manager ساده و مستقل باشد؛ mapping فقط در Custom Provider باز شود؛ حالت‌های saving/testing/starting/error تا رسیدن backend به وضعیت نهایی disabled/loading باقی بمانند و UI فارسی/انگلیسی و RTL/LTR درست باشد.

### [x] TASK-039 - ساخت UI حساب‌ها، limit و recommendation [P]

**Dependencies**: TASK-031  
**Files**: `web-react/src/client/components/codex-manager/AccountsPanel.tsx`, `web-react/src/client/components/codex-manager/AccountCard.tsx`, `web-react/src/client/components/codex-manager/LimitWindows.tsx`, `web-react/src/client/components/codex-manager/*_test.tsx`  
**Criteria**: Free/Plus، وضعیت، limit window، reset، بهترین حساب، علت پیشنهاد، refresh state و account فعلی compact و مرتب نمایش داده شوند؛ secret یا raw token در DOM نباشد.

### [x] TASK-040 - ساخت نمودار تاریخچه مصرف [P]

**Dependencies**: TASK-020, TASK-031  
**Files**: `web-react/src/client/components/codex-manager/UsageHistoryChart.tsx`, `web-react/src/client/components/codex-manager/UsageHistoryChart.test.tsx`, `web-react/src/client/app/UsageSettingsSection.tsx`  
**Criteria**: account/window/time range قابل انتخاب، loading skeleton، empty/error state و محدودسازی نقاط برای history بزرگ پیاده شوند.

### [x] TASK-041 - ساخت UI ورود Device Code

**Dependencies**: TASK-022, TASK-031, TASK-039  
**Files**: `web-react/src/client/components/codex-manager/DeviceLoginDialog.tsx`, `web-react/src/client/components/codex-manager/DeviceLoginDialog.test.tsx`  
**Criteria**: code/URL، copy، countdown، poll، cancel، timeout و نتیجه import روشن باشند؛ بستن dialog flow را orphan نکند و retry کنترل‌شده باشد.

### [x] TASK-042 - ساخت UI Chrome profiles و session cleanup

**Dependencies**: TASK-026, TASK-031  
**Files**: `web-react/src/client/components/codex-manager/BrowserSessionsPanel.tsx`, `web-react/src/client/components/codex-manager/BrowserSessionsPanel.test.tsx`  
**Criteria**: profile/account/session، current-device protection، dry-run و confirmation هدف‌دار پیاده شوند؛ revoke optimistic نباشد و تا پاسخ نهایی loading/disabled باقی بماند.

### [x] TASK-043 - نمایش وضعیت routing و switch در chat

**Dependencies**: TASK-028, TASK-031, TASK-034  
**Files**: `internal/server/workspace_ws.go`, `web-react/src/client/components/chat-ui/SessionHealthPopover.tsx`, `web-react/src/client/components/chat-ui/ChatPreferenceControls.tsx`, `web-react/src/client/components/chat-ui/*test*`  
**Criteria**: account فعلی، pinned/automatic، switch reason، gateway health و آخرین refresh بدون افشای اطلاعات حساس نمایش داده شوند؛ اعلان فقط برای switch یا خطای معنادار باشد.

### [x] TASK-044 - import خودکار داده‌های نصب قبلی codex-manager

**Dependencies**: TASK-005, TASK-017, TASK-020, TASK-026, TASK-031  
**Files**: `internal/codexmanager/migrate/import.go`, `internal/codexmanager/migrate/import_test.go`, `internal/server/codex_manager_api.go`  
**Criteria**: dry-run/preview/import/backup انجام شود؛ account/status/history/config و Chrome cache شناخته‌شده منتقل شوند؛ اجرای دوم no-op باشد و در شکست partial state گزارش و rollback شود.

### [x] TASK-045 - یکپارچه‌سازی نصب، update، restart و uninstall sidecar

**Dependencies**: TASK-004, TASK-030, TASK-044  
**Files**: `scripts/install-release.sh`, `scripts/install-release.ps1`, `scripts/build-from-source.sh`, `scripts/build-from-source.ps1`, `internal/server/app_management.go`, `.goreleaser.yaml`  
**Criteria**: install/update sidecar درست را نصب کنند، `abolqasem restart` settings/accounts را حفظ کند، version mismatch قابل ترمیم باشد و uninstall فقط با انتخاب صریح داده‌های manager را حذف کند.

## Phase 4: امنیت، پایداری و انتشار

### [x] TASK-046 - امنیت و threat model حساب‌ها و مرورگر

**Dependencies**: TASK-031, TASK-033, TASK-042, TASK-045  
**Files**: `docs/security/codex-manager.md`, `internal/server/server_security_test.go`, `internal/secrets/store_test.go`  
**Criteria**: SSRF برای custom base URL، loopback binding، file permission، path traversal، token/cookie redaction، malicious model metadata، CSRF و revoke authorization تست شوند؛ sidecar به‌طور پیش‌فرض فقط روی loopback گوش دهد.

### [x] TASK-047 - تست برابری کامل Go با Python و حذف وابستگی Python

**Dependencies**: TASK-027, TASK-044  
**Files**: `internal/codexmanager/**/*_test.go`, `docs/codex-manager-parity.md`, `scripts/**`, `README.md`  
**Criteria**: تمام ردیف‌های parity matrix پاس شوند؛ هیچ subprocess/import/install از Python باقی نماند؛ تنها بخش runtime خارجی Rust sidecar باشد.

### [x] TASK-048 - تست end-to-end app-server از مسیر Manager

**Dependencies**: TASK-034, TASK-035, TASK-043  
**Files**: `internal/server/codex_manager_e2e_test.go`, `sidecars/codex-manager-gateway/tests/**`  
**Criteria**: initialize، model/list، thread start/resume/fork، text/image، tool call، steer، interrupt، compaction و reconnect از app-server تا sidecar تست شوند؛ model و reasoning effort بدون mapping حفظ شوند.

### [x] TASK-049 - تست end-to-end Custom Provider و mapping

**Dependencies**: TASK-036, TASK-037, TASK-038  
**Files**: `internal/server/custom_provider_e2e_test.go`, `web-react/src/client/components/settings/CustomProviderEditor.test.tsx`  
**Criteria**: passthrough، alias mapping، reasoning metadata، auth/header، مدل ناشناخته، endpoint قطع و retry تست شوند؛ manager mode در همه تست‌ها mapping UI/API نداشته باشد.

### [x] TASK-050 - تست crash/recovery و consistency

**Dependencies**: TASK-029, TASK-030, TASK-034, TASK-044  
**Files**: `internal/codexmanager/recovery_test.go`, `internal/server/workspace_runtime_test.go`, `sidecars/codex-manager-gateway/tests/recovery.rs`  
**Criteria**: kill sidecar، kill app-server، restart Abolqasem، فایل نیمه‌نوشته، lock قدیمی، token منقضی و history بزرگ بدون گیرکردن UI یا گم‌شدن binding بازیابی شوند.

### [x] TASK-051 - تست performance و محدودسازی کارهای پس‌زمینه

**Dependencies**: TASK-021, TASK-030, TASK-039, TASK-040  
**Files**: `internal/codexmanager/benchmark_test.go`, `web-react/src/client/components/codex-manager/*test*`, `docs/codex-manager-performance.md`  
**Criteria**: صدها account/history sample، چند chat هم‌زمان و Chrome profileهای متعدد benchmark شوند؛ APIها pagination/cache داشته باشند و بازشدن Settings منتظر check شبکه‌ای نماند.

### [x] TASK-052 - مستندات کاربر، مهاجرت و عیب‌یابی

**Dependencies**: TASK-038, TASK-041, TASK-042, TASK-045  
**Files**: `README.md`, `docs/codex-manager.md`, `docs/troubleshooting.md`  
**Criteria**: فعال‌سازی یک‌تیکی، افزودن حساب، auto-switch policy، custom provider/mapping، Chrome permission، backup/restore و خطاهای gateway/app-server با راه‌حل عملی مستند شوند.

### [x] TASK-053 - دروازه نهایی انتشار

**Dependencies**: TASK-046, TASK-047, TASK-048, TASK-049, TASK-050, TASK-051, TASK-052  
**Files**: `.github/workflows/test.yml`, `.github/workflows/release.yml`, `.github/release-notes/<version>.md`  
**Criteria**: Go/Rust/React test، lint، build matrix، install smoke test، upgrade از نسخه قبلی، artifact content و release notes پاس شوند؛ انتشار بدون sidecar متناظر یا بدون release note fail شود.

## ترتیب تحویل پیشنهادی

- **Milestone A — Backend parity**: TASK-001 تا TASK-027؛ تمام Python به Go منتقل شده ولی هنوز UI عمومی فعال نیست.
- **Milestone B — Gateway integration**: TASK-028 تا TASK-035؛ toggle ساده Manager و مسیر واقعی app-server آماده است.
- **Milestone C — Product UI**: TASK-036 تا TASK-045؛ custom provider، accounts، history، login و Chrome UI کامل‌اند.
- **Milestone D — Production release**: TASK-046 تا TASK-053؛ امنیت، recovery، performance و release gate کامل‌اند.

## شرط اتمام کل پروژه

- [x] فعال‌سازی Manager واقعاً با یک تیک و بدون model mapping انجام شود.
- [x] custom provider بدون mapping و با mapping اختیاری هر دو کار کنند.
- [x] مدل، reasoning effort، image/tool/steer/interrupt/compact در مسیر Manager سالم بمانند.
- [x] هیچ Python runtime یا Python subprocess در محصول نهایی باقی نماند.
- [x] Rust sidecar همراه هر artifact نصب و با نسخه Go هماهنگ شود.
- [x] حساب‌ها، limitها، recommendation، history، Device Login و Chrome sessionها از UI قابل مدیریت باشند.
- [x] restart/update تنظیمات، account store، history و bindingها را پاک نکند.
- [x] هیچ token، cookie یا gateway key در API پاسخ، frontend، transcript یا log ظاهر نشود.

## اصلاحات نهایی رابط و app-server (2026-08-29)

- [x] TASK-054 — ریست حساب Codex سریع و idempotent شود؛ بدون انتظار برای usage API و بدون resume کردن thread خراب.
- [x] TASK-055 — پین/برداشتن پین چت از منوی چت، event store و sidebar با مرتب‌سازی پایدار.
- [x] TASK-056 — لودینگ عملیات API نام و مرحلهٔ سرویس را نشان دهد و در حالت pending قابل کلیک مجدد نباشد.
- [x] TASK-057 — تغییر model و reasoning effort از native session و turn به رکورد قابل نمایش در transcript تبدیل شود.
- [x] TASK-058 — خطای legacy `subagent-completed` و خطای resume قابل بازیابی باشد؛ در صورت خرابی thread، thread تمیز ساخته شود و UI در حالت Thinking بی‌نهایت نماند.
- [x] TASK-059 — صفحهٔ مستقل Codex Manager برای افزودن/حذف/تغییرنام/فعال‌سازی حساب، نمایش Free/Plus، وضعیت ورود، سهمیه و مصرف، تاریخچه و نشست‌های Chrome؛ تنظیمات Provider از این صفحه جدا بماند و load-balancer به‌عنوان sidecar اختیاری نمایش داده شود.
- [x] TASK-060 — maintenance واقعیِ Codex Manager هنگام اجرای سرور فعال شود: هر ۶ ساعت با jitter، refresh حساب انتخاب‌شده و سایر حساب‌های ذخیره‌شده پیش از انقضا، history retention و shutdown امن بدون وابستگی به بازبودن UI.

## تکمیل parity عملیاتی Codex Manager (LiteLLM/OpenRouter خارج از scope)

- [x] TASK-061 — تنظیمات کامل Manager در UI/API: interval، jitter، proxy، retention و policy نگه‌داری تاریخچه؛ تغییر تنظیمات worker بعدی را بدون نیاز به systemd/cron اعمال کند.
- [x] TASK-062 — lifecycle کامل حساب: import امن `auth.json`، refresh اجباری یک حساب، انتخاب و فعال‌سازی بهترین حساب، rename همراه status/history و نمایش علت/زمان آخرین refresh. سوییچ دستی ابتدا نسخهٔ تازه‌ترِ auth زنده را sync می‌کند، `auth.json` منتخب را atomic جایگزین و فقط app-serverهای بیکار را reset می‌کند؛ هنگام turn فعال fail-safe است.
- [x] TASK-063 — نمایش کامل quota: تمام windowها، remaining، reset time/countdown، credits، state/error reason و recommendation قابل فهم در UI و API redacted.
- [x] TASK-064 — تاریخچهٔ کامل: rangeهای زمانی، timezone، window/account filter و pagination/sample limit بدون کندکردن Settings.
- [x] TASK-065 — Chrome parity: association چندحسابی profile، انتخاب account در profile، monitor/cleanup policy، preview/revoke امن و خطای قابل اقدام برای cookie/keyring.
- [x] TASK-066 — compact با حساب انتخابی و handoff کنترل‌شدهٔ native auth در مرز turn؛ عدم تغییر credential هنگام turn فعال و rollback قطعی در خطا.
- [x] TASK-067 — diagnostics عملیاتی: وضعیت binary/gateway/config/store/permission، worker و آخرین execution/failure در UI/API.
- [x] TASK-068 — آزمون end-to-end و parity نهایی هر قابلیت منتقل‌شده؛ baseline Python (۶۴ تست)، Go، UI/build و Rust sidecar همگی سبز شدند و docs به‌روزرسانی شد.
- [x] TASK-069 — migration عملیاتی: backupهای legacy و gateway key هرگز import نشوند؛ account/status اصلی و history قدیمی به schema Go به‌صورت idempotent merge شوند.
- [x] TASK-070 — parity monitor: پایش دوره‌ای Chrome sessionها با opt-in صریح، dry-run پیش‌فرض، current-device protection و گزارش امن در diagnostics/UI اجرا شود.
- [x] TASK-071 — اکشن‌های سریع popover وضعیت نشست: ورود به «اکانت‌های کدکس» و «انتخاب بهترین اکانت» با آیکن، tooltip و وضعیت در حال اجرا؛ مستقل از روشن‌بودن Manager.
- [x] TASK-072 — نمایش شمارش معکوس quota در جدول حساب‌ها به‌صورت تک‌خطی و شفاف (`2d 4:36` یا `6:07`) و بدون شکستن ردیف.
- [x] TASK-073 — API قدیمی `/api/agent/turn` برای Claude و OpenCode نیز با resume/fork/model/effort کار کند و وضعیت آن‌ها controllable گزارش شود.
- [x] TASK-074 — جابه‌جایی گروه‌های پروژه در سایدبار با event پایدار ذخیره و پس از refresh بازسازی شود؛ ورودی ناقص/تکراری رد شود.
- [x] TASK-075 — خطای دستور ناشناختهٔ WebSocket علت عدم‌هماهنگی نسخه و راهکار reload/restart را اعلام کند؛ همهٔ commandهای شناخته‌شده handler داشته باشند.
- [x] TASK-076 — buildهای Go برای شش target و pipeline نصب چندسکویی بررسی شوند؛ sidecar cross-build در CI با Zig/cargo-zigbuild gate شود.
- [x] TASK-077 — همگام‌سازی نشست native فعال: hook بلافاصله snapshot چت را broadcast کند؛ در تب visible هر ۱ ثانیه و در تب hidden هر ۱۵ ثانیه refresh تک‌درخواستی انجام شود؛ JSONL بدون تغییر با cache محدود مبتنی بر `mtime + size` دوباره parse نشود، تغییر متن با شناسهٔ ثابتِ streaming در UI دور ریخته نشود و refresh دستیِ کنار قفل نیز بدون انتظار برای ACK سنگین اجرا شود.
