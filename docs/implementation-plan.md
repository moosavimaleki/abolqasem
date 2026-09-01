# برنامهٔ اجرای پشتیبانی OpenCode

## تصمیم معماری

OpenCode یک provider مستقل است، نه یک alias برای Codex. برای اجرای turn از CLI نصب‌شده (`opencode`) استفاده می‌شود. فهرست و export سشن‌ها از SQLite محلی OpenCode در حالت read-only خوانده می‌شود، چون نسخهٔ نصب‌شدهٔ CLI پس از چاپ JSON گاهی helper طولانی‌عمر باقی می‌گذارد. این انتخاب نیازی به daemon دائمی، پورت ثابت یا نگهداری credential دوم در Abolqasem ندارد؛ OpenCode خود مالک credential و SQLite خود می‌ماند.

Fork بین providerها «تبدیل تاریخچه» است: transcript استاندارد Abolqasem به session بومی مقصد صادر می‌شود. برای OpenCode، exporter یک session با همان context قابل‌فهم ایجاد می‌کند و turn بعدی را روی همان session اجرا می‌کند. fork بومی OpenCode نیز از session token استفاده می‌کند.

## فایل‌ها

| بخش | فایل‌ها |
| --- | --- |
| adapter و parser | `internal/providers/opencode/*` |
| runtime | `internal/server/workspace_turn_starter.go` |
| import/export و fork | `internal/sessioninterop/import.go`, `internal/sessioninterop/export.go`, `internal/sessioninterop/export_opencode.go` |
| catalog/settings | `internal/providers/catalog/*`, `internal/providers/providerexec/*`, `internal/server/workspace_provider_models.go` |
| UI/provider picker | `web-react/src/shared/types.ts`, store و composer provider controls |

## کارها

- [x] OC-001: provider، executable discovery و catalog OpenCode را اضافه کن؛ اگر CLI نصب نیست UI آن را unavailable نشان دهد.
  - پذیرش: `opencode --version` قابل تشخیص باشد و مدل‌های پیکربندی‌شده با command استاندارد قابل کشف باشند.
- [x] OC-002: adapter turn با session resume/fork و parse خروجی JSON بساز.
  - پذیرش: prompt، model/variant، cancel و خطای CLI به قرارداد `agent.Turn` تبدیل شود.
- [x] OC-003: import/export sessionهای OpenCode و تبدیل transcript دوطرفه را بساز.
  - پذیرش: session خارجی OpenCode خوانده شود و Codex↔OpenCode fork session مقصدی قابل ادامه بسازد.
- [x] OC-004: runtime، تنظیمات و UI provider picker را وصل کن.
  - پذیرش: OpenCode کنار Codex و Claude قابل انتخاب باشد و fork conversion آن را به‌عنوان مقصد پیشنهاد کند.
- [x] OC-005: تست واحد، build و نصب سرویس محلی.
  - پذیرش: Go/TypeScript tests، build و health سرویس پاس شوند.

## ترتیب اجرا و ریسک

مسیر بحرانی OC-001 → OC-002 → OC-003 → OC-004 → OC-005 است. API OpenCode نسخه‌دار است؛ بنابراین wrapper CLI و exporter نسبت به API داخلی آن ترجیح داده می‌شوند و خطای نسخه در UI/turn به‌شکل قابل اقدام گزارش می‌شود.
