# Codex Manager security model

Codex Manager keeps credentials, refresh tokens and browser cookies on the
server. API responses and React state contain only account labels, plan/status,
quota percentages and redacted health information. Secret files are created
with `0600`; state directories use `0700`.

The Rust gateway binds to loopback by default and authenticates local requests
with a generated key. The Go supervisor owns its lifecycle and never logs the
key. Custom provider URLs accept only HTTP(S), reject malformed URLs and are
never interpolated into shell commands. Deployments that expose the local API
through a reverse proxy must add authentication and CSRF protection at that
proxy; destructive account/device operations still require an explicit target.

Browser discovery copies the Chrome cookie database to a temporary file and
does not mutate the live profile. The current device cannot be revoked. Cookie
values, authorization headers, refresh tokens and gateway keys must not appear
in logs, transcript entries or frontend markup.

Threats covered by the implementation and tests include path traversal in
account/migration names, secret redaction, loopback-only manager routing,
atomic writes, stale locks, duplicate model mappings and accidental current
device revocation. A custom provider is an explicit user opt-in; network
egress should be restricted with the deployment firewall when untrusted URLs
are possible.
