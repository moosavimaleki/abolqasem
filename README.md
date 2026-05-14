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

اسکریپت‌های نصب به صورت پیش‌فرض این کارها را انجام می‌دهند:

- binary را نصب می‌کنند
- hookهای Codex، Claude Code و Gemini CLI را نصب می‌کنند
- در نصب از سورس، اگر `go` در PATH نباشد، تلاش می‌کنند آن را خودکار نصب کنند
- در انتها به تو یادآوری می‌کنند داخل agentها trust/approve را انجام دهی اگر prompt نشان داده شد


## Architecture

- Hook محلی event را از stdin می‌گیرد.
- اگر server بالا باشد، event به `POST /api/hook` فرستاده می‌شود.
- اگر server پایین باشد، event در `~/.cache/ai-agent-manager/pending-events.jsonl` ذخیره می‌شود.
- server روی `127.0.0.1` UI و API را serve می‌کند.
- transcriptها parse و cache می‌شوند و پیام‌ها به صورت pagination شده به UI می‌رسند.
