# AI Agent Manager

`ai-agent-manager` یک viewer محلی و zero-token برای نمایش sessionهای Codex، Claude Code و Gemini CLI در مرورگر است. ابزار از hookهای محلی استفاده می‌کند، transcriptهای JSONL را می‌خواند، و آن‌ها را با UI سازگار با RTL/LTR، markdown امن، lazy loading و اعلان session فعال نمایش می‌دهد.

## What It Solves

ترمینال برای متن‌های فارسی، متن‌های mixed RTL/LTR، جدول‌های markdown، pathها و stack traceها خوانایی خوبی ندارد. این ابزار همان session را بدون فرستادن prompt به مدل، در مرورگر و روی `127.0.0.1` باز می‌کند.

## Install

نصب از releaseهای آماده GitHub، بدون نیاز به Go:

```bash
curl -fsSL https://raw.githubusercontent.com/moosavimaleki/ai-agent-manager/main/scripts/install-release.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/moosavimaleki/ai-agent-manager/main/scripts/install-release.ps1 | iex
```

برای نصب از سورس:

```bash
git clone <repo-url>
cd ai-agent-manager
scripts/install.sh
```

برای توسعه فرانت، سورس داخل `web-react/` است. پوشه `web/` خروجی generated برای embed شدن داخل باینری Go است و با دستور زیر دوباره ساخته می‌شود:

```bash
sh scripts/prepare-web-assets.sh
make build
```

اسکریپت‌های نصب به صورت پیش‌فرض این کارها را انجام می‌دهند:

- binary را نصب می‌کنند
- hookهای Codex، Claude Code و Gemini CLI را نصب می‌کنند
- در نصب از سورس، اگر `go` در PATH نباشد، تلاش می‌کنند آن را خودکار نصب کنند
- در انتها به تو یادآوری می‌کنند داخل agentها trust/approve را انجام دهی اگر prompt نشان داده شد


## Architecture

- Hook محلی event را از stdin می‌گیرد.
- در حالت hook، رابط داخلی برنامه سرور را idempotent روی اولین پورت آزاد از `9090` به بعد بالا می‌آورد و base URL واقعی را ذخیره می‌کند.
- اگر سرور تازه توسط hook بالا آمده باشد، مرورگر پیش‌فرض روی همان base URL باز می‌شود.
- اگر سرور پایین بماند، event در `~/.cache/ai-agent-manager/pending-events.jsonl` ذخیره می‌شود.
- در حالت service، سرویس دائمی سیستم‌عامل همین سرور داخلی را مدیریت می‌کند.
- transcriptها parse و cache می‌شوند و پیام‌ها به صورت pagination شده به UI می‌رسند.
