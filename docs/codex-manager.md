# Codex Manager in Abolqasem

## فعال‌سازی

در Settings → Providers گزینهٔ Codex Manager را روشن کنید. فعال‌سازی یک‌تیکی
gateway را روی loopback اجرا می‌کند و مدل‌های بومی Codex را بدون mapping تغییر
نمی‌دهد. خاموش‌کردن برای turn فعال اختلال ایجاد نمی‌کند.

## حساب‌ها و تعویض خودکار

از پنل حساب‌ها Device Login را بزنید، کد و URL را در مرورگر کامل کنید و تا
نتیجهٔ import صبر کنید. plan (Free/Plus)، سهمیه و پیشنهاد بهترین حساب نمایش
داده می‌شود. سیاست auto-switch را فقط در صورت نیاز فعال کنید؛ token هرگز در UI
نمایش داده نمی‌شود.

در بازکردن این صفحه، `~/.codex/auth.json` زنده نیز به‌عنوان حساب فعال کشف و با store همگام می‌شود. در فعال‌سازی دستی، ابتدا نسخهٔ تازه‌ترِ auth زنده sync، سپس فایل منتخب به‌صورت atomic جایگزین می‌شود و فقط app-serverهای بیکار reset می‌شوند؛ در turn فعال عملیات با خطای امن متوقف می‌شود.

حساب‌های ذخیره‌شده در Manager در پس‌زمینه هر ۶ ساعت (با حداکثر ۱۰ دقیقه پراکندگی در تنظیم پیش‌فرض) بررسی می‌شوند. interval، jitter، proxy و retention از همان صفحه قابل تغییرند و worker بدون restart برنامه زمان‌بندی بعدی را اعمال می‌کند. access token نزدیک به انقضا با refresh token همان حساب تازه می‌شود؛ این کار مستقل از بازبودن Settings است و tokenها را به UI یا log نمی‌فرستد.

## Custom Provider

حالت Custom برای endpointهای سازگار با Responses API است. mapping اختیاری است:
اگر mapping نسازید شناسهٔ مدل همان‌طور که وارد کرده‌اید ارسال می‌شود. دکمهٔ Test
قبل از ذخیره health و فهرست مدل‌ها را بررسی می‌کند و خطاها redacted هستند.

## Chrome و پاک‌سازی

پروفایل و sessionها از پنل Chrome قابل مشاهده‌اند. دستگاه فعلی محافظت می‌شود؛
برای خروج از دستگاه دیگر تأیید صریح لازم است. گزینهٔ «فقط پیش‌نمایش» هیچ revokeی
انجام نمی‌دهد.

## backup و update

update/restart فقط باینری و sidecar را عوض می‌کند و state، account، history و
bindingها را نگه می‌دارد. برای انتقال نصب قبلی ابتدا `/api/codex-manager/migration`
را به‌صورت dry-run اجرا کنید. uninstall داده‌های Manager را حذف نمی‌کند مگر
اینکه `scripts/uninstall.sh --delete-manager-data` را اجرا کرده و عبارت DELETE
را تأیید کنید.
