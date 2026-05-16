# Claude Adapter Decision

Kanna uses `@anthropic-ai/claude-agent-sdk` directly from its TypeScript backend.

For the Go port, the default implementation should use the local `claude` CLI in non-interactive stream mode:

```text
claude --print --output-format stream-json --include-partial-messages
```

Reasons:

- The target backend is Go, so a permanent JavaScript SDK bridge would keep the old backend architecture alive.
- The local CLI is the stable integration point available on this machine.
- The CLI exposes model, effort, permission mode, resume, fork-session, and stream-json output.
- CI can test command construction and stream parsing without requiring auth or spending tokens.

Current scope:

- Build command arguments matching Kanna session options.
- Parse stream-json assistant/result events into transcript entries.
- Keep session management and tool approval parity for Task 8.2.

Not chosen:

- Permanent JS bridge: rejected as default because it violates the Go-backend port goal.
- Remote Claude API adapter: deferred because Kanna behavior is Claude Code session/tool behavior, not plain Messages API behavior.
