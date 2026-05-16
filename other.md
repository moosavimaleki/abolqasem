بله، لینک‌های اصلی مستندات/سورس هر سه:

## 1. OpenAI Codex

**Codex main repo** — مخزن اصلی Codex CLI:
([GitHub][1])

**Codex app-server README** — دقیقاً همان چیزی که دوستت گفته؛ توضیح app-server، JSON-RPC، transport و protocol:
([GitHub][2])

**Codex MCP interface docs** — توضیح APIهای `thread/start`، `turn/start`، `turn/steer`، `turn/interrupt` و history:
([GitHub][3])

**Codex app-server test client** — نمونه client برای تست کردن app-server:
([GitHub][4])

---

## 2. Claude Code

**Claude Code Agent SDK overview** — معادل اصلی برای programmatic agent control در Claude Code:
([Claude][5])

**Run Claude Code programmatically / headless** — اجرای Claude Code با `claude -p`، مناسب CI/CD و backend integration:
([Claude][6])

**Agent SDK quickstart** — شروع سریع با Python/TypeScript SDK:
([Claude][7])

---

## 3. Gemini CLI

**Gemini CLI official repo** — مخزن اصلی Gemini CLI:
([GitHub][8])

**Gemini CLI docs در Google Developers / Code Assist** — معرفی رسمی Gemini CLI:
([Google for Developers][9])

**Gemini CLI headless mode** — اجرای programmatic با `gemini -p` و خروجی text/json/stream-json:
([Gemini CLI][10])

**Gemini CLI ACP mode** — نزدیک‌ترین چیز به client/server control؛ JSON-RPC over stdio برای IDE و tool integration:
([GitHub][11])

---

مهم‌ترین‌ها برای کاری که تو می‌خواهی:

```txt
Codex:
codex-rs/app-server/README.md
codex-rs/docs/codex_mcp_interface.md

Claude:
Agent SDK overview
Run Claude Code programmatically

Gemini:
ACP Mode
Headless Mode
```

[1]: https://github.com/openai/codex?utm_source=chatgpt.com "openai/codex: Lightweight coding agent that runs in your ..."
[2]: https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md?utm_source=chatgpt.com "codex/codex-rs/app-server/README.md at main"
[3]: https://github.com/openai/codex/blob/main/codex-rs/docs/codex_mcp_interface.md?utm_source=chatgpt.com "codex/codex-rs/docs/codex_mcp_interface.md at main"
[4]: https://github.com/openai/codex/blob/main/codex-rs/app-server-test-client/README.md?utm_source=chatgpt.com "codex/codex-rs/app-server-test-client/README.md at main"
[5]: https://code.claude.com/docs/en/agent-sdk/overview?utm_source=chatgpt.com "Agent SDK overview - Claude Code Docs"
[6]: https://code.claude.com/docs/en/headless?utm_source=chatgpt.com "Run Claude Code programmatically"
[7]: https://code.claude.com/docs/en/agent-sdk/quickstart?utm_source=chatgpt.com "Quickstart - Claude Code Docs"
[8]: https://github.com/google-gemini/gemini-cli?utm_source=chatgpt.com "google-gemini/gemini-cli: An open-source AI ..."
[9]: https://developers.google.com/gemini-code-assist/docs/gemini-cli?utm_source=chatgpt.com "Gemini CLI | Gemini Code Assist"
[10]: https://geminicli.com/docs/cli/headless/?utm_source=chatgpt.com "Headless mode reference"
[11]: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/acp-mode.md?utm_source=chatgpt.com "acp-mode.md - gemini-cli"





**Codex app-server یعنی “کنترل‌پلین محلیِ Codex agent”.**

یعنی خودش مدل هوش مصنوعی نیست؛ یک **server/daemon محلی** است که بین UI / وب / IDE و موتور Codex قرار می‌گیرد.

معماری ذهنی‌اش این است:

```txt
Web UI / IDE / Viewer
        |
        | JSON-RPC / MCP / local transport
        v
Codex app-server
        |
        v
Codex agent runtime
        |
        +-- مدل OpenAI
        +-- فایل‌ها
        +-- shell commands
        +-- apply_patch
        +-- approvals
        +-- transcript/history
```

در Codex، primitive اصلی این‌هاست:

```txt
Thread = یک مکالمه / session
Turn   = یک نوبت کاربر → agent
Item   = پیام، reasoning، shell command، file edit، نتیجه tool و...
```

خود README رسمی Codex هم همین را می‌گوید: API سه primitive اصلی دارد: `Thread`، `Turn` و `Item`، و برای ادامه‌دادن مکالمه باید از APIهای thread و turn استفاده شود. ([GitHub][1])

پس حرف دوستت درست است:
فایل‌های transcript / rollout بیشتر **history/journal** هستند. نوشتن داخلشان مثل این نیست که به agent دستور داده باشی. مثل این است که لاگ nginx را ادیت کنی و انتظار داشته باشی request جدید اجرا شود.

---

### پس app-server دقیقا چه کاری می‌کند؟

کارهایی مثل این:

```txt
thread/start        ساخت session جدید
thread/resume       ادامه session قبلی
turn/start          فرستادن پیام جدید وقتی agent بیکار است
turn/steer          هدایت agent وسط اجرای یک turn
turn/interrupt      قطع کردن turn در حال اجرا
thread/read         خواندن history
thread/list         لیست threadها
approval requests   گرفتن تایید برای shell/edit
event streaming     استریم اتفاقات agent
```

در مستندات Codex MCP interface هم آمده که Codex یک JSON-RPC API برای مدیریت threadها، turnها، account، config و approvalها دارد و RPCهای اصلی شامل `thread/start`، `thread/resume`، `turn/start`، `turn/steer` و `turn/interrupt` هستند. ([GitHub][2])

بنابراین:

```txt
viewer فعلی تو = فقط می‌خواند و نمایش می‌دهد
app-server client = می‌تواند پیام بفرستد، turn بسازد، steer کند، approve کند
```

---

### فرق viewer با app-server client

**Viewer فعلی:**

```txt
read transcript file
parse rollout
render UI
```

این فقط replay گذشته است.

**Interactive client درست:**

```txt
connect to app-server
initialize
thread/start یا thread/resume
turn/start
receive events
handle approvals
render live state
persist mapping
```

یعنی برای وب تعاملی، باید وب‌ات یا backend وب‌ات clientِ app-server شود.

---

### آیا Claude Code هم چنین چیزی دارد؟

**هم دارد، هم دقیقاً نه.**

Claude Code چیزی به اسم `Codex app-server` ندارد، ولی معادل عملیاتی‌اش را دارد:

1. **Claude Agent SDK**
2. **headless mode با `claude -p`**
3. **Python / TypeScript SDK**
4. **structured output**
5. **tool approval callback**
6. **permission system**
7. **hooks**
8. **MCP integration**

مستندات Claude Code می‌گوید Agent SDK همان tools، agent loop و context managementی را می‌دهد که Claude Code استفاده می‌کند، و می‌شود آن را از CLI، Python یا TypeScript به‌صورت programmatic اجرا کرد. ([Claude][3])

مثلاً:

```bash
claude -p "Find and fix the bug in auth.py" --allowedTools "Read,Edit,Bash"
```

یا از SDK:

```txt
your web backend
    |
    v
Claude Agent SDK / claude -p
    |
    v
Claude Code agent runtime
```

ولی تفاوت مهم:

```txt
Codex app-server:
    شبیه یک local RPC server رسمی برای کنترل thread/turn

Claude Code:
    بیشتر SDK/headless/CLI محور است
```

Claude Code همچنین سیستم permission و approval دارد؛ مثلاً SDK می‌تواند با permission modes، hooks، allow/deny rules و `canUseTool` کنترل کند agent چه ابزاری را اجرا کند. ([Claude][4])

پس برای وب تعاملی با Claude Code، راه درست معمولاً این است:

```txt
Web UI
  -> backend خودت
  -> Claude Agent SDK / claude CLI process
  -> stream events / approvals back to web
```

نه اینکه فایل history Claude را دستکاری کنی.

---

### آیا Gemini CLI هم چنین چیزی دارد؟

**بله، از Claude حتی نزدیک‌تر به الگوی client/server است، ولی اسمش app-server نیست.**

Gemini CLI خودش یک agent متن‌باز ترمینالی است و طبق README رسمی، built-in tools مثل file operations، shell commands، web fetching، Google Search grounding و MCP support دارد. ([GitHub][5])

برای programmatic usage دو مسیر مهم دارد:

### 1. Headless mode

Gemini CLI یک headless mode دارد که با `-p` یا non-TTY فعال می‌شود و خروجی text، JSON یا JSONL streaming می‌دهد. مستنداتش صریحاً می‌گوید headless mode یک programmatic interface برای Gemini CLI است. ([Gemini CLI][6])

مثلاً:

```bash
gemini -p "Refactor this module" --output-format json
```

یا streaming:

```bash
gemini -p "Fix tests" --output-format stream-json
```

### 2. ACP mode

Gemini CLI همچنین **ACP mode** دارد. ACP یعنی Agent Client Protocol. این دقیقاً برای این است که IDE یا client خارجی بتواند با agent از طریق JSON-RPC روی stdio حرف بزند.

مستندات Gemini می‌گوید ACP mode برای کنترل programmatic، مخصوصاً IDE و developer tool integration است و از JSON-RPC 2.0 over stdio استفاده می‌کند. ([Gemini CLI][7])

مثلاً:

```bash
gemini --acp
```

پس برای Gemini:

```txt
Web UI
  -> backend adapter
  -> gemini --acp
  -> JSON-RPC over stdio
  -> Gemini agent
```

یا ساده‌تر:

```txt
Web UI
  -> backend
  -> gemini -p "...prompt..." --output-format stream-json
```

---

### مقایسه خیلی خلاصه

| ابزار           |                            معادل app-server دارد؟ | بهترین مسیر برای وب interactive                              |
| --------------- | ------------------------------------------------: | ------------------------------------------------------------ |
| **Codex**       |    بله، app-server / MCP server / thread-turn API | اتصال backend وب به `codex app-server` یا `codex mcp-server` |
| **Claude Code** | نه دقیقاً با این اسم؛ ولی Agent SDK/headless دارد | backend با Claude Agent SDK یا `claude -p`                   |
| **Gemini CLI**  |       نه با اسم app-server؛ ولی ACP/headless دارد | backend با `gemini --acp` یا headless JSON/JSONL             |

---

### جواب معماری برای پروژه تو

اگر viewer فعلی‌ات فقط transcript می‌خواند، بهترین مسیر این است:

```txt
Frontend
  |
  v
Your backend / local bridge
  |
  +-- sessionKey -> threadId mapping
  +-- auth
  +-- workspace allowlist
  +-- approval handling
  +-- event streaming to browser
  |
  v
Codex app-server / Gemini ACP / Claude SDK
```

یعنی مرورگر مستقیم به agent وصل نشود. مخصوصاً اگر agent می‌تواند shell command بزند یا فایل edit کند، مستقیم expose کردنش خطرناک است.

جمع‌بندی:

**Codex app-server همان API/control-plane زنده‌ی Codex است. برای پیام‌فرستادن باید با آن حرف بزنی، نه با فایل transcript. Claude Code و Gemini CLI هم قابلیت programmatic دارند، ولی شکل‌شان فرق دارد: Claude بیشتر SDK/headless است، Gemini بیشتر headless + ACP، و Codex thread/turn app-server API دارد.**

[1]: https://github.com/openai/codex/blob/main/codex-rs/app-server/README.md "codex/codex-rs/app-server/README.md at main · openai/codex · GitHub"
[2]: https://github.com/openai/codex/blob/main/codex-rs/docs/codex_mcp_interface.md "codex/codex-rs/docs/codex_mcp_interface.md at main · openai/codex · GitHub"
[3]: https://code.claude.com/docs/en/headless "Run Claude Code programmatically - Claude Code Docs"
[4]: https://code.claude.com/docs/en/agent-sdk/permissions "Configure permissions - Claude Code Docs"
[5]: https://github.com/google-gemini/gemini-cli "GitHub - google-gemini/gemini-cli: An open-source AI agent that brings the power of Gemini directly into your terminal. · GitHub"
[6]: https://geminicli.com/docs/cli/headless/ "Headless mode reference | Gemini CLI"
[7]: https://geminicli.com/docs/cli/acp-mode/?utm_source=chatgpt.com "ACP Mode"



