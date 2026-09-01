# Abolqasem

`abolqasem` یک viewer محلی و zero-token برای نمایش sessionهای Codex، Claude Code و Gemini CLI در مرورگر است. ابزار از hookهای محلی استفاده می‌کند، transcriptهای JSONL را می‌خواند، و آن‌ها را با UI سازگار با RTL/LTR، markdown امن، lazy loading و اعلان session فعال نمایش می‌دهد.

## What It Solves

ترمینال برای متن‌های فارسی، متن‌های mixed RTL/LTR، جدول‌های markdown، pathها و stack traceها خوانایی خوبی ندارد. این ابزار همان session را بدون فرستادن prompt به مدل، در مرورگر و روی `127.0.0.1` باز می‌کند.

## Install

نصب از releaseهای آماده GitHub، بدون نیاز به Go:

```bash
curl -fsSL https://raw.githubusercontent.com/moosavimaleki/abolqasem/refs/heads/main/scripts/install-release.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/moosavimaleki/abolqasem/refs/heads/main/scripts/install-release.ps1 | iex
```

برای build/install از سورس:

```bash
git clone <repo-url>
cd abolqasem
scripts/build-from-source.sh
```

برای توسعه فرانت، سورس داخل `web-react/` است. پوشه `web/` خروجی generated برای embed شدن داخل باینری Go است و با دستور زیر دوباره ساخته می‌شود. فایل‌های build داخل `web/` نباید commit شوند:

```bash
sh scripts/prepare-web-assets.sh
make build
```

اسکریپت‌های نصب به صورت پیش‌فرض این کارها را انجام می‌دهند:

- binary را نصب می‌کنند
- سرویس دائمی کاربر را نصب، فعال و health-check می‌کنند
- hookهای Codex، Claude Code و Gemini CLI را نصب می‌کنند
- در نصب از سورس، اگر `go` در PATH نباشد، تلاش می‌کنند آن را خودکار نصب کنند
- در انتها به تو یادآوری می‌کنند داخل agentها trust/approve را انجام دهی اگر prompt نشان داده شد

فرمان `abolqasem install` idempotent است؛ اجرای دوباره آن سرویس و هر سه hook را repair می‌کند. برای حذف کامل نیز از `abolqasem uninstall` استفاده کن.

## Codex Manager

در Settings → Providers می‌توانی Codex Manager را با یک تیک فعال کنی. این
حالت Rust sidecar محلی را اجرا می‌کند، حساب‌های Codex را مدیریت می‌کند و مدل‌های
بومی Codex را بدون model mapping عبور می‌دهد. Custom Provider مسیر جداگانه‌ای
برای endpointهای سازگار با Responses API دارد و mapping در آن اختیاری است.

راهنمای کامل فعال‌سازی، migration، Chrome sessions و عیب‌یابی در
[Codex Manager](docs/codex-manager.md)، [migration](docs/codex-manager-migration.md)
و [troubleshooting](docs/troubleshooting.md) قرار دارد.

## Architecture

- Hook محلی event را از stdin می‌گیرد.
- سرویس دائمی کاربر سرور داخلی را روی اولین پورت آزاد از `9092` به بعد بالا نگه می‌دارد و base URL واقعی را ذخیره می‌کند.
- hook در صورت متوقف بودن سرور، همان سرویس نصب‌شده را دوباره start می‌کند.
- اگر سرور پایین بماند، event در `~/.cache/abolqasem/pending-events.jsonl` ذخیره می‌شود.
- transcriptها parse و cache می‌شوند و پیام‌ها به صورت pagination شده به UI می‌رسند.
