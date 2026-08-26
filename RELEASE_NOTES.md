# Abolqasem 1.3.0

این نسخه مسیر اجرای native app-server را تثبیت می‌کند و تجربهٔ چت RTL را کامل‌تر می‌سازد.

- حذف runtimeهای قدیمی tmux و Gemini از مسیر فعال
- نمایش activity واقعی Codex و کارت‌های command، file-change و plan
- پیوست تصویر و متن بلند با preview و بررسی متادیتای upload
- dedupe پیام‌های optimistic/session و رخدادهای `turn_aborted`
- صف، Steer و تخلیهٔ صف پس از خطای turn
- takeover نشست‌های قفل‌شده در وب و Telegram
- custom commandهای allowlist‌شدهٔ Telegram با ذخیرهٔ پایدار
- usage/context، sidebar پروژه‌محور و اندازه/پاک‌سازی cache در تنظیمات

## Verification

- `go test ./...` — 399 tests passed
- `bun test` — 456 tests passed
- `scripts/dev-install.sh` — installed and service healthy on `127.0.0.1:9092`
