# AI Session Viewer

`ai-session-viewer` یک viewer محلی و zero-token برای نمایش sessionهای Codex، Claude Code و Gemini CLI در مرورگر است. ابزار از hookهای محلی استفاده می‌کند، transcriptهای JSONL را می‌خواند، و آن‌ها را با UI سازگار با RTL/LTR، markdown امن، lazy loading و اعلان session فعال نمایش می‌دهد.

## What It Solves

ترمینال برای متن‌های فارسی، متن‌های mixed RTL/LTR، جدول‌های markdown، pathها و stack traceها خوانایی خوبی ندارد. این ابزار همان session را بدون فرستادن prompt به مدل، در مرورگر و روی `127.0.0.1` باز می‌کند.

## Install

پیش‌نیاز فقط Go 1.22+ است.

```bash
git clone <repo-url>
cd codex-rtl-plugin
make build
mkdir -p ~/.local/bin
install -m 0755 dist/ai-session-viewer ~/.local/bin/ai-session-viewer
```

برای نصب hookها:

```bash
ai-session-viewer install --agent codex --scope user
ai-session-viewer install --agent claude --scope user
ai-session-viewer install --agent gemini --scope user
```

برای همه agentها:

```bash
ai-session-viewer install --all --scope user
```

## Run

server را بالا بیاور:

```bash
ai-session-viewer server
```

viewer را در مرورگر باز کن:

```bash
ai-session-viewer open
```

اگر server بالا نیست:

```bash
ai-session-viewer open --start-server
```

وضعیت server و hookها:

```bash
ai-session-viewer status
```

## CLI

```text
ai-session-viewer server
ai-session-viewer hook --agent codex|claude|gemini
ai-session-viewer install --agent codex|claude|gemini --scope user|project
ai-session-viewer uninstall --agent codex|claude|gemini --scope user|project
ai-session-viewer open [--start-server]
ai-session-viewer status
```

## Architecture

- Hook محلی event را از stdin می‌گیرد.
- اگر server بالا باشد، event به `POST /api/hook` فرستاده می‌شود.
- اگر server پایین باشد، event در `~/.cache/ai-session-viewer/pending-events.jsonl` ذخیره می‌شود.
- server روی `127.0.0.1` UI و API را serve می‌کند.
- transcriptها parse و cache می‌شوند و پیام‌ها به صورت pagination شده به UI می‌رسند.

## Notes About Legacy Files

این repo قبلاً نسخه‌ای skill/python-based داشت. آن مسیر دیگر implementation اصلی نیست. فایل‌های legacy فقط برای compatibility سبک نگه داشته شده‌اند و نباید مسیر اصلی نصب یا استفاده باشند.

## Development

فرمان‌های اصلی:

```bash
gofmt -w .
go vet ./...
go test ./...
make build
make build-all
```

برای تست build تمیز:

```bash
tmp=$(mktemp -d)
git archive HEAD | tar -x -C "$tmp"
cd "$tmp"
make build
```
