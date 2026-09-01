# Codex Manager Python → Go parity matrix

این سند قرارداد رفتاری انتقال `/home/h-mousavi/Projects/Hamed/codex-manager/codex_manager` به Abolqasem است. هر مورد باید پیش از حذف Python با fixture بدون secret و تست Go پوشش داده شود.

## قواعد غیرقابل تغییر

1. `accounts/*.json` منبع حقیقت حساب‌هاست. `~/.codex/auth.json` فقط legacy live auth است و نباید در حالت Gateway برای routing جابه‌جا شود.
2. **Activate** با **automatic request routing** متفاوت است: Activate تنها عملیاتی صریح و خارج از turn فعال است؛ Gateway فقط request را به حسابی که quota مناسب دارد می‌فرستد.
3. هنگام refresh یا import، هویت email/account-id باید دوباره بررسی شود. token جدیدِ حساب دیگر هرگز نباید روی حساب فعلی نوشته شود.
4. token، refresh token، cookie و Set-Cookie فقط در memory یا secret store هستند و در response/log/transcript/UI قرار نمی‌گیرند.
5. فایل‌های manager با directory mode `0700` و data mode `0600` نوشته می‌شوند؛ writeها atomic و همراه پنج backup چرخشی‌اند.
6. هر maintenance/scheduler operation cancellable، single-flight و با timeout/backoff است.
7. browser monitor فقط sessionهای application=`Codex` را می‌بیند/عمل می‌کند، current device را نگه می‌دارد و بدون opt-in revoke نمی‌کند.

## ماتریس انتقال

| منبع Python | مقصد Go | رفتار قابل مشاهده | fixture / آزمون پذیرش |
|---|---|---|---|
| `paths.py` | `internal/codexmanager/storage/paths.go` | محل state، accounts، status، history؛ sanitize نام حساب؛ مرتب‌سازی deterministic | نام خالی، فاصله، unicode، traversal، duplicate، directory missing |
| `storage.py` | `internal/codexmanager/storage/{store,lock}.go` | JSON atomic، backup rotation، state/log، lock بین processها | crash حین write، JSON خراب، lock قدیمی، هم‌زمانی دو writer، permission |
| `config.py` و `constants.py` | `internal/state` و workerهای `internal/server/codex_manager_*` | interval، proxy، retention، مسیر دادهٔ Chrome و scheduler policy؛ config legacy و gateway key import نمی‌شوند | interval/path نامعتبر، default opt-in، redacted config |
| `auth.py` | `internal/codexmanager/auth/{jwt,identity,refresh}.go` | JWT claims، expiry، plan/account/email، refresh decision، refresh و identity safety | JWT خراب، expiry نزدیک، refresh موفق، identity mismatch، HTTP 401/timeout |
| `commands/accounts.py` | `internal/codexmanager/account` | add/import/list/rename/delete/activate/sync-live-auth | account تکراری، rename history، delete pinned، live auth قدیمی/متعلق به حساب دیگر |
| `codex/limits.py` | `internal/codexmanager/limits` | fetch و normalize windowها، reset، credits و rate-limit state | 5h/weekly/monthly، payload ناقص، 429، reset timestamp/timezone |
| `recommendation.py` و `commands/best.py` | `internal/codexmanager/recommendation` | score، Free/Plus ordering، BEST/RISK/SAVE/STALE و reason | tie-break، no quota، stale status، Free در برابر Plus |
| `history.py` و `commands/chart.py` | `internal/codexmanager/history` | append/prune/load/rename و series/window timezone-aware | JSONL بزرگ، retention، account rename، UTC/+03:30، empty series |
| `commands/maintenance.py` | `internal/codexmanager/maintenance` | sync، refresh inactive، fetch limits، append history و status | active refresh خاموش، force refresh، account error، cancellation، repeat call |
| `commands/scheduler.py` | `internal/codexmanager/maintenance/scheduler.go` | interval/jitter و اجرای درون process Abolqasem | single-flight، backoff، shutdown، manual refresh و no cron/systemd dependency |
| `codex/app_server.py` | RPC client موجود + `internal/codexmanager/login` | temporary app-server، initialize، account login و compaction events | app-server unavailable، JSON-RPC error، cleanup temp state، cancellation |
| `codex/device_login.py` | `internal/codexmanager/login/device.go` | start/poll/cancel/timeout، import auth موقت | code/URL، timeout، cancel، duplicate email، import failure |
| `commands/compact.py` | app-server thread compact integration | انتخاب حساب صریح و compact بدون corrupt کردن auth/session | turn active، session ID/path، failure و restore؛ routing خودکار از آن جداست |
| `chatgpt_sessions.py` | `internal/codexmanager/browser/{profiles,cookies,accounts,session_client}.go` | discover profile، read-only cookie copy، account switcher، session list/revoke | locked DB، profile خراب، cookie missing، partial sign-in، network error |
| `commands/sessions.py` | `internal/codexmanager/browser/monitor.go` و worker `internal/server/codex_manager_session_monitor.go` | dry-run، Codex-only filtering، current-device protection، cleanup policy و monitor دوره‌ای opt-in | zero/one/multiple Codex sessions، Windows-first policy، monitor disabled، preview بدون revoke |
| `commands/doctor.py` | health/status API + UI diagnostics | binary/config/store/version/permission/sidecar health | missing sidecar، stale status، permission mismatch، restart required |
| `commands/gateway.py` | `internal/codexmanager/sidecar/supervisor.go` | sidecar path/env/start/stop/readiness | bad binary، bad key، port collision، crash loop، graceful stop |
| `rust-gateway/src/main.rs` | `sidecars/codex-manager-gateway` | `/health`، `/v1/models`، `/v1/responses` و quota-based routing | auth failure، stream/non-stream response، 401/403/429 retry، no candidate |
| `textual_ui.py`, `views.py`, `terminal.py`, `cli.py` | React settings/pages + Go API | TUI/CLI rendering منتقل نمی‌شود؛ فقط رفتارهای عملیاتی منتقل می‌شوند | UI loading/error/empty/RTL/LTR و endpoint contract |
| `system.py`, `time_utils.py`, `errors.py` | package-local Go helpers/errors | subprocess/time/error formatting به API/structured errors تبدیل می‌شود | timeout، context cancellation، typed error to user-safe message |

## قرارداد داده و ownership

| داده | مالک | دسترسی |
|---|---|---|
| account auth / refresh token | Go secret/account store | backend و sidecar فقط از فایل/env امن |
| gateway API key | Go secret store | فقط supervisor → sidecar env |
| account status و limit snapshot | Go manager store | Go API و Rust sidecar read-only snapshot |
| history | Go manager store | Go API فقط aggregate/paginated |
| durable chat→account binding | Rust sidecar store | sidecar؛ UI فقط redacted snapshot |
| Chrome cookies | memory موقتی Go | هرگز persisted یا exposed نمی‌شود |
| chat/thread/provider preference | Abolqasem state/transcript | app-server و UI |

## تفاوت‌های عمدی با Python

- Textual، argparse، رنگ‌های terminal، clipboard helper و نصب pip منتقل نمی‌شوند.
- systemd/crontab جای خود را به scheduler درون process Abolqasem می‌دهد.
- live `~/.codex/auth.json` برای درخواست‌های Gateway دست‌کاری نمی‌شود.
- Gateway binding از memory-only به persistent تبدیل می‌شود.
- monitor دوره‌ای Chrome فقط با opt-in فعال می‌شود، به‌طور پیش‌فرض preview است و هنگام خاموش‌بودن cookie هم نمی‌خواند. خروج واقعی همچنان نیازمند خاموش‌کردن «فقط پیش‌نمایش» است.
- compaction از lifecycle thread خود Abolqasem استفاده می‌کند؛ switch حساب برای compact باید explicit و جدا از auto-routing باشد.

## دروازه پذیرش حذف Python

- [x] همه ردیف‌های جدول fixture و test دارند.
- [x] خروجی Go برای fixtureهای limits/recommendation/history/auth با baseline Python برابر است.
- [x] import داده موجود idempotent و rollback-safe است.
- [x] هیچ `python`, `pip`, `textual` یا subprocess وابسته به package قدیمی در artifact نهایی نیست.
- [x] Rust sidecar بدون Python اجرا می‌شود و فقط snapshotهای Go را می‌خواند.

## تأیید نهایی (2026-08-30)

baseline Python و پیاده‌سازی Go هر دو با تست‌های خودشان سبز هستند؛ fixtureهای redactedِ auth/history/rate-limit نیز در Go parse و normalize می‌شوند. قرارداد UI، buildهای client/export-viewer و sidecar Rust هم بررسی شده‌اند:

- `PYTHONPATH=. pytest -q` در پروژهٔ قبلی: 64 passed.
- `go test ./... -count=1`: passed.
- `bun run test` و `bun run check` در `web-react`: passed.
- `cargo test` در `sidecars/codex-manager-gateway`: 6 passed.
