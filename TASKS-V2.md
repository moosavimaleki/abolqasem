در ادامه یک سند مهندسی آماده برای repo می‌نویسم. اسم فایل پیشنهادی:

```text
docs/integrations-and-cross-platform.md
```

# سند اتصال به Claude Code و Gemini CLI + توسعه Cross Platform

## 1. هدف سند

هدف پروژه این است که یک ابزار سبک و local داشته باشیم که بتواند sessionهای ابزارهای AI coding مثل Codex، Claude Code و Gemini CLI را بدون مصرف توکن، در مرورگر نشان دهد.

نام پیشنهادی عمومی پروژه:

```text
ai-session-viewer
```

چرا نه `codex-rtl-viewer`؟ چون از اول می‌خواهیم فقط مخصوص Codex نباشد و adapter برای Claude Code و Gemini CLI هم داشته باشد.

---

# 2. اصل طراحی Integration

هر agent باید فقط از طریق hook یا event محلی به viewer خبر بدهد.

یعنی این مسیر:

```text
Claude Code / Gemini CLI / Codex
        ↓
local hook command
        ↓
ai-session-viewer hook --agent claude|gemini|codex
        ↓
127.0.0.1:9090/api/hook
        ↓
Browser UI
```

نباید این مسیر را برویم:

```text
slash prompt
skill prompt
custom prompt
LLM instruction
```

چون آن‌ها ممکن است مدل را صدا بزنند و توکن مصرف کنند.

---

# 3. معماری Adapterها

ساختار داخلی پروژه باید agent-agnostic باشد:

```go
type AgentAdapter interface {
    Name() string
    InstallHook(scope InstallScope) error
    UninstallHook(scope InstallScope) error
    NormalizeHookInput(input []byte) (SessionEvent, error)
    ParseTranscript(path string, opts ParseOptions) ([]Message, error)
}
```

پیاده‌سازی‌ها:

```text
internal/adapters/codex/
internal/adapters/claude/
internal/adapters/gemini/
```

ساختار event مشترک:

```go
type SessionEvent struct {
    Agent          string `json:"agent"`           // codex, claude, gemini
    SessionID      string `json:"session_id"`
    TranscriptPath string `json:"transcript_path"`
    CWD            string `json:"cwd"`
    ProjectName    string `json:"project_name"`
    UpdatedAt      string `json:"updated_at"`
    Raw            any    `json:"raw,omitempty"`
}
```

نکته مهم: `transcript_path` باید در مدل داده optional فرض شود، چون ممکن است بعضی agentها همیشه مسیر transcript را دقیق ندهند. برای Claude Code این فیلد در ورودی hookهای مختلف از جمله `Stop` دیده می‌شود، اما برای Gemini باید adapter حالت fallback داشته باشد. Claude Code در hookهای command، JSON event را از stdin می‌دهد و برای HTTP hookها آن را POST body می‌فرستد؛ eventهایی مثل `SessionStart`، `UserPromptSubmit`، `Stop`، `PreToolUse` و `PostToolUse` هم در lifecycle آن تعریف شده‌اند. ([Claude][1])

---

# 4. اتصال به Claude Code

## 4.1. Hook مناسب

برای viewer، بهترین event در Claude Code:

```text
Stop
```

چون وقتی پاسخ Claude تمام می‌شود، ما می‌خواهیم session را در browser آپدیت کنیم.

Claude Code در تنظیمات hook از command hook پشتیبانی می‌کند؛ command hook یک shell command یا executable اجرا می‌کند، input را از stdin می‌گیرد و نتیجه را از stdout/exit code برمی‌گرداند. همچنین Claude Code برای command hook از `args` هم پشتیبانی می‌کند تا executable بدون shell quoting اجرا شود. ([Claude][1])

## 4.2. مسیرهای تنظیمات Claude

Claude Code hookها را می‌تواند از چند منبع بخواند، از جمله:

```text
~/.claude/settings.json
.claude/settings.json
.claude/settings.local.json
plugin hooks
```

پس installer ما باید حداقل دو حالت داشته باشد:

```bash
ai-session-viewer install --agent claude --scope user
ai-session-viewer install --agent claude --scope project
```

منبع user یعنی:

```text
~/.claude/settings.json
```

منبع project یعنی:

```text
.claude/settings.json
```

Claude Code در مستندات خود این مسیرهای تنظیمات را برای user/project/local/plugin نشان می‌دهد. ([Claude][1])

## 4.3. Config پیشنهادی Claude

برای user-level install:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "ai-session-viewer",
            "args": ["hook", "--agent", "claude"],
            "timeout": 3
          }
        ]
      }
    ]
  }
}
```

برای Windows هم همین بهتر است، چون با `args` دیگر درگیر quote کردن path و PowerShell نمی‌شویم. اگر installer مجبور شد از shell form استفاده کند، آن‌وقت برای Windows باید `shell: "powershell"` بگذارد؛ Claude Code چنین حالتی را هم پشتیبانی می‌کند. ([Claude][1])

## 4.4. ورودی Claude Adapter

Claude Stop hook چیزی در این جنس می‌دهد:

```json
{
  "session_id": "abc123",
  "transcript_path": "/Users/.../.claude/projects/.../00893aaf.jsonl",
  "cwd": "/Users/...",
  "permission_mode": "default",
  "hook_event_name": "Stop",
  "stop_hook_active": true,
  "last_assistant_message": "I've completed..."
}
```

پس `claude.NormalizeHookInput` باید این‌ها را بردارد:

```go
SessionEvent{
    Agent:          "claude",
    SessionID:      input.SessionID,
    TranscriptPath: expandPath(input.TranscriptPath),
    CWD:            input.CWD,
    ProjectName:    base(input.CWD),
    UpdatedAt:      now,
    Raw:            input,
}
```

---

# 5. اتصال به Gemini CLI

## 5.1. Hook مناسب

برای Gemini CLI، برای viewer بهترین eventهای پیشنهادی:

```text
AfterAgent
SessionEnd
```

برای آپدیت زنده بعد از هر پاسخ، `AfterAgent` مناسب‌تر است. برای ثبت نهایی سشن هنگام خروج، `SessionEnd` هم می‌تواند به‌عنوان backup نصب شود.

Gemini CLI طبق مستنداتش hookها را در `.gemini/settings.json` یا سطح user/system/extensions تنظیم می‌کند و eventهایی مثل `BeforeTool`، `AfterTool`، `BeforeAgent`، `AfterAgent`، `Notification`، `SessionStart`، `SessionEnd`، `BeforeModel`، `AfterModel` و `BeforeToolSelection` دارد. ([GitHub][2])

## 5.2. قانون stdout/stderr در Gemini

Gemini hook باید این قانون را رعایت کند:

```text
logs → stderr
final JSON → stdout
```

پس command ما باید در پایان همیشه یک JSON کوچک برگرداند، مثلاً:

```json
{}
```

مستندات Gemini CLI صریحاً می‌گوید logها باید روی `stderr` نوشته شوند و فقط JSON نهایی روی `stdout` بیاید. ([GitHub][3])

## 5.3. مسیرهای تنظیمات Gemini

installer باید این scopeها را پشتیبانی کند:

```bash
ai-session-viewer install --agent gemini --scope user
ai-session-viewer install --agent gemini --scope project
```

مسیرها:

```text
User:
~/.gemini/settings.json

Project:
.gemini/settings.json
```

مستندات Gemini CLI می‌گوید hookها در `settings.json` تعریف می‌شوند و نمونه config هم با `.gemini/settings.json` نشان داده شده است. ([GitHub][3])

## 5.4. Config پیشنهادی Gemini

```json
{
  "hooks": {
    "AfterAgent": [
      {
        "matcher": "*",
        "hooks": [
          {
            "name": "ai-session-viewer-after-agent",
            "type": "command",
            "command": "ai-session-viewer hook --agent gemini"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "matcher": "*",
        "hooks": [
          {
            "name": "ai-session-viewer-session-end",
            "type": "command",
            "command": "ai-session-viewer hook --agent gemini"
          }
        ]
      }
    ]
  }
}
```

نکته: اگر Gemini در نسخه‌ای از config خود `args` را پشتیبانی کرد، installer باید ترجیحاً از exec/args استفاده کند. اگر نه، command string کافی است.

## 5.5. Gemini Adapter باید defensive باشد

برای Gemini نباید فرض کنیم همیشه این را داریم:

```json
{
  "session_id": "...",
  "transcript_path": "...",
  "cwd": "..."
}
```

پس adapter باید چند مرحله fallback داشته باشد:

```text
۱. اگر input.session_id وجود داشت، استفاده کن.
۲. اگر input.transcript_path وجود داشت و فایل واقعی بود، استفاده کن.
۳. اگر cwd وجود داشت، project_name را از cwd بساز.
۴. اگر transcript_path نبود، آخرین session شناخته‌شده همان cwd را از state خودمان پیدا کن.
۵. اگر باز هم نبود، event را به‌عنوان metadata-only ذخیره کن.
```

برای viewer، نبودن transcript نباید crash ایجاد کند. UI باید بنویسد:

```text
این سشن ثبت شده، اما transcript قابل خواندن نیست.
```

---

# 6. طراحی CLI نهایی برای چند Agent

CLI باید این‌طوری باشد:

```bash
ai-session-viewer server --port 9090
ai-session-viewer hook --agent codex
ai-session-viewer hook --agent claude
ai-session-viewer hook --agent gemini

ai-session-viewer install --agent codex  --scope user
ai-session-viewer install --agent claude --scope user
ai-session-viewer install --agent gemini --scope user

ai-session-viewer uninstall --agent claude --scope user
ai-session-viewer status
ai-session-viewer open
```

برای نصب همه:

```bash
ai-session-viewer install --all --scope user
```

---

# 7. توسعه Cross Platform

## 7.1. تصمیم اصلی

Backend و CLI باید با Go نوشته شود.

دلیل:

```text
یک binary مستقل می‌سازیم.
نیازی به Node/Python runtime نیست.
روی Linux/macOS/Windows راحت distribute می‌شود.
HTTP server سبک می‌ماند.
```

برای ساده ماندن cross-platform build، پروژه باید تا حد ممکن pure Go باشد و از CGO دوری کند. Go از زمان 1.5 cross-compilation ساده برای برنامه‌های pure Go دارد و با `GOOS` و `GOARCH` می‌شود برای OS/Architectureهای دیگر build گرفت؛ اما هنگام cross-compiling، `cgo` به‌صورت پیش‌فرض غیرفعال می‌شود و اگر `import "C"` یا build modeهای خاص بخواهیم، به C cross-compiler نیاز داریم. ([Go][4])

## 7.2. مسیرهای سیستم‌عامل‌ها

هیچ مسیر hardcode نکن.

استفاده کن از:

```go
os.UserConfigDir()
os.UserCacheDir()
os.UserHomeDir()
```

ساختار منطقی:

```text
Config:
ai-session-viewer/config.json

Cache/State:
ai-session-viewer/state.json
ai-session-viewer/sessions.json
ai-session-viewer/pending-events.jsonl
```

در کد، مسیر نهایی را Go تعیین کند، نه خودمان.

## 7.3. باز کردن مرورگر در هر OS

فایل:

```text
internal/platform/browser.go
```

پیاده‌سازی پیشنهادی:

```go
func OpenBrowser(url string) error {
    switch runtime.GOOS {
    case "linux":
        return exec.Command("xdg-open", url).Start()
    case "darwin":
        return exec.Command("open", url).Start()
    case "windows":
        return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
    default:
        return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
    }
}
```

برای MVP کافی است. بعداً می‌شود fallback اضافه کرد.

---

# 8. ساختار پروژه Cross Platform

```text
ai-session-viewer/
├── cmd/
│   └── ai-session-viewer/
│       └── main.go
├── internal/
│   ├── adapters/
│   │   ├── codex/
│   │   ├── claude/
│   │   └── gemini/
│   ├── cli/
│   ├── server/
│   ├── parser/
│   ├── state/
│   ├── installer/
│   └── platform/
├── web/
│   ├── index.html
│   ├── app.js
│   └── styles.css
├── scripts/
│   ├── install.sh
│   └── install.ps1
├── .github/
│   └── workflows/
│       ├── test.yml
│       └── release.yml
├── .goreleaser.yaml
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

Frontend باید با `embed` داخل binary بیاید:

```go
//go:embed web/*
var webFS embed.FS
```

این باعث می‌شود برای نصب فقط یک binary لازم باشد.

---

# 9. Build دستی برای Linux/macOS/Windows

## 9.1. Linux amd64

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-linux-amd64 ./cmd/ai-session-viewer
```

## 9.2. Linux arm64

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-linux-arm64 ./cmd/ai-session-viewer
```

## 9.3. macOS Intel

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-darwin-amd64 ./cmd/ai-session-viewer
```

## 9.4. macOS Apple Silicon

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-darwin-arm64 ./cmd/ai-session-viewer
```

## 9.5. Windows amd64

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-windows-amd64.exe ./cmd/ai-session-viewer
```

## 9.6. Windows arm64

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
go build -trimpath -ldflags="-s -w" \
-o dist/ai-session-viewer-windows-arm64.exe ./cmd/ai-session-viewer
```

---

# 10. Makefile پیشنهادی

```makefile
APP=ai-session-viewer
PKG=./cmd/ai-session-viewer
DIST=dist

.PHONY: clean build test build-all

clean:
	rm -rf $(DIST)

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP) $(PKG)

build-all: clean
	mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-linux-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-linux-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-darwin-amd64 $(PKG)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-darwin-arm64 $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-windows-amd64.exe $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(DIST)/$(APP)-windows-arm64.exe $(PKG)
```

---

# 11. Release با GoReleaser

برای release عمومی بهتر است از GoReleaser استفاده شود، چون خروجی‌های چندسکویی، archive، checksum، GitHub Release و package managerها را استاندارد می‌کند. GoReleaser اکشن رسمی GitHub Actions دارد و نمونه workflow رسمی آن روی tag push اجرا می‌شود؛ خود مستندات هم می‌گوید checkout باید `fetch-depth: 0` داشته باشد تا history کامل برای release در دسترس باشد. ([GoReleaser][5])

## 11.1. `.goreleaser.yaml`

```yaml
version: 2

project_name: ai-session-viewer

before:
  hooks:
    - go mod tidy
    - go test ./...

builds:
  - id: ai-session-viewer
    main: ./cmd/ai-session-viewer
    binary: ai-session-viewer
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}

archives:
  - id: default
    format_overrides:
      - goos: windows
        format: zip
    files:
      - README.md
      - LICENSE

checksum:
  name_template: "checksums.txt"

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
```

## 11.2. GitHub Action

```yaml
name: release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v7
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

---

# 12. Installer Cross Platform

## 12.1. Linux/macOS install.sh

```bash
curl -fsSL https://raw.githubusercontent.com/OWNER/ai-session-viewer/main/scripts/install.sh | bash
```

رفتار installer:

```text
۱. OS/ARCH را تشخیص بده.
۲. آخرین release را از GitHub بگیر.
۳. binary درست را دانلود کن.
۴. در ~/.local/bin نصب کن.
۵. اگر PATH مشکل دارد، پیام واضح بده.
۶. پیشنهاد بده:
   ai-session-viewer install --agent claude --scope user
   ai-session-viewer install --agent gemini --scope user
   ai-session-viewer server
```

## 12.2. Windows install.ps1

```powershell
iwr -useb https://raw.githubusercontent.com/OWNER/ai-session-viewer/main/scripts/install.ps1 | iex
```

رفتار:

```text
۱. معماری را تشخیص بده.
۲. zip مناسب را دانلود کن.
۳. در مسیر زیر نصب کن:
   $env:LOCALAPPDATA\ai-session-viewer\bin
۴. PATH user را در صورت نیاز آپدیت کن.
۵. پیام بده که terminal جدید باز شود.
```

---

# 13. Installer برای hookها

## 13.1. Claude Installer

```bash
ai-session-viewer install --agent claude --scope user
```

باید:

```text
۱. فایل ~/.claude/settings.json را بخواند یا بسازد.
۲. JSON را parse کند.
۳. hooks.Stop را اضافه کند.
۴. اگر قبلاً وجود دارد duplicate نسازد.
۵. قبل از تغییر backup بگیرد.
```

Backup:

```text
~/.claude/settings.json.bak.ai-session-viewer-20260514-103000
```

## 13.2. Gemini Installer

```bash
ai-session-viewer install --agent gemini --scope user
```

باید:

```text
۱. فایل ~/.gemini/settings.json را بخواند یا بسازد.
۲. hooks.AfterAgent را اضافه کند.
۳. hooks.SessionEnd را optional اضافه کند.
۴. duplicate نسازد.
۵. backup بگیرد.
```

---

# 14. تست Cross Platform

## 14.1. Unit Test

```bash
go test ./...
```

بخش‌هایی که حتماً تست می‌خواهند:

```text
Claude hook input normalize
Gemini hook input normalize
Codex hook input normalize
JSONL parser
direction detector RTL/LTR
state atomic write
installer JSON merge
```

## 14.2. Matrix تست GitHub Actions

```yaml
name: test

on:
  pull_request:
  push:
    branches:
      - main

jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go: [stable]

    runs-on: ${{ matrix.os }}

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}

      - run: go test ./...
```

---

# 15. نکات مهم برای سبک ماندن سرور

چون server همیشه بالا می‌ماند:

```text
نباید polling سنگین داشته باشد.
نباید کل transcriptها را مدام parse کند.
نباید React/Node/Electron لازم داشته باشد.
نباید روی هر SSE event کل session را render کند.
```

رفتار درست:

```text
server idle:
    تقریباً صفر CPU

hook event:
    فقط state را update کن
    فقط SSE notification بده

browser:
    وقتی لازم شد، پیام‌ها را lazy-load کن

parser:
    فقط segment لازم را بخوان
```

در MVP می‌توان parser را ساده‌تر گرفت، ولی API باید از اول pagination داشته باشد.

---

# 16. ریسک‌های Cross Platform

## ریسک ۱: مسیرهای config متفاوت

راه‌حل:

```text
هیچ مسیر hardcode نکن.
برای config عمومی خودمان از os.UserConfigDir استفاده کن.
برای agentها path را در adapter تعریف کن.
```

## ریسک ۲: command quoting در Windows

راه‌حل:

```text
تا حد ممکن از executable + args استفاده کن.
برای Claude از args استفاده کن.
برای Gemini اگر فقط command string دارد، مسیر binary را بدون space نصب کن یا quote درست بساز.
```

## ریسک ۳: نبودن transcript_path در بعضی agentها

راه‌حل:

```text
transcript_path optional باشد.
adapter fallback داشته باشد.
UI حالت metadata-only را نشان دهد.
```

## ریسک ۴: schema تغییر کند

راه‌حل:

```text
parser tolerant باشد.
Raw event ذخیره شود.
unknown event crash نکند.
```

---

# 17. Definition of Done این سند

این بخش وقتی کامل است که:

```text
ai-session-viewer install --agent claude --scope user
```

باعث شود Claude Code بعد از هر پاسخ، session را به viewer معرفی کند.

و:

```text
ai-session-viewer install --agent gemini --scope user
```

باعث شود Gemini CLI بعد از هر agent turn یا session end، event را به viewer بفرستد.

و:

```text
ai-session-viewer server --port 9090
```

روی Linux/macOS/Windows اجرا شود و browser از این آدرس کار کند:

```text
http://127.0.0.1:9090
```

و releaseها برای این targetها ساخته شوند:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
windows/amd64
windows/arm64
```

جمع‌بندی فنی: پروژه باید از روز اول با سه لایه ساخته شود: `Agent Adapter` برای تفاوت Codex/Claude/Gemini، `Core Server` برای state و UI، و `Platform Layer` برای تفاوت‌های Windows/Linux/macOS. این جداسازی باعث می‌شود هم cross platform بماند، هم بعداً اضافه کردن agent جدید مثل Cursor CLI یا Qwen Code تبدیل به یک adapter تازه شود، نه بازنویسی کل پروژه.

[1]: https://code.claude.com/docs/en/hooks "Hooks reference - Claude Code Docs"
[2]: https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md?utm_source=chatgpt.com "gemini-cli/docs/reference/configuration.md at main"
[3]: https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/writing-hooks.md "gemini-cli/docs/hooks/writing-hooks.md at main · google-gemini/gemini-cli · GitHub"
[4]: https://go.dev/wiki/WindowsCrossCompiling "Go Wiki: Building Windows Go programs on Linux - The Go Programming Language"
[5]: https://goreleaser.com/customization/ci/actions/ "GitHub Actions – GoReleaser"
