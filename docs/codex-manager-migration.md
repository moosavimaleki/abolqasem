# مهاجرت Codex Manager به Abolqasem

## مالکیت داده

در حالت جدید، داده‌های manager در مسیر manager home باقی می‌مانند و Go آن‌ها را مدیریت می‌کند. Abolqasem نباید برای routing، فایل `~/.codex/auth.json` را جابه‌جا یا overwrite کند.

ورودی‌های شناخته‌شده:

- `~/.codex-manager/accounts/*.json`: حساب‌ها و tokenهای ذخیره‌شده
- `~/.codex-manager/status/*.json`: snapshot وضعیت و limit
- `~/.codex-manager/history/limits.jsonl`: تاریخچه limit
- `~/.codex-manager/config.json`: فقط برای شناسایی نصب قدیمی؛ هرگز import نمی‌شود چون می‌تواند `gateway_api_key` داشته باشد
- `~/.codex-manager/state.json`: active account و state
- cacheهای Chrome: فقط به‌صورت read-only و بدون کپی cookie

## روند import

1. Go مسیرها را resolve و permission را بررسی می‌کند.
2. dry-run فهرست فایل‌ها، حساب‌ها، history sample و موردهای قابل مهاجرت را بدون token نمایش می‌دهد.
3. پیش از write، همهٔ ورودی‌ها validate می‌شوند؛ `*.json.BAK*`، cache مرورگر و config حذف‌شده از plan هستند.
4. credentialها با نام و هویت ادغام می‌شوند؛ دو نام برای یک identity/refresh-token ساخته نمی‌شود.
5. تاریخچهٔ `history/limits.jsonl` و نام قدیمی `history/rate-limits.jsonl` به schema فعلی merge می‌شود؛ duplicate بر اساس account و timestamp حذف می‌شود.
6. config قدیمی import نمی‌شود؛ interval/proxy/monitor و مسیر دادهٔ Chrome از صفحهٔ «مدیریت حساب‌های Codex» تنظیم می‌شوند و gateway key فقط در secret store تازهٔ Abolqasem تولید می‌شود.
7. active account legacy فقط در store جدید mark می‌شود و `~/.codex/auth.json` را دست نمی‌زند.

## idempotency و rollback

- اجرای دوباره با همان source باید no-op باشد.
- source هرگز delete یا mutate نمی‌شود.
- write هر فایل ابتدا به temp و سپس atomic rename است.
- در failure، preimage همهٔ فایل‌های تغییرکرده (از جمله history) در همان transaction restore می‌شود و sidecar فعال نمی‌شود.
- rollback فقط داده‌های import‌شده را برمی‌گرداند و chat/transcriptهای Abolqasem را لمس نمی‌کند.

## هم‌زیستی با turn فعال

- import، activate و delete حساب در زمان turn فعال همان حساب مسدود یا deferred می‌شوند.
- Gateway از snapshot خواندنی استفاده می‌کند؛ refresh نباید فایل در حال مصرف را نیمه‌نویس کند.
- پس از تغییر config/provider، process جدید app-server برای turn بعدی ساخته می‌شود؛ turn جاری قطع نمی‌شود.

## سیاست حذف

Uninstall برنامه فقط باینری و sidecar را حذف می‌کند. حذف `~/.codex-manager`، history، account auth یا browser cache باید یک انتخاب جداگانه با confirmation صریح باشد.
