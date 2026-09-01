# Codex Manager sidecar protocol

The sidecar is a local Rust HTTP service. It is never exposed outside loopback.

## Startup contract

- `CODEX_MANAGER_GATEWAY_LISTEN`: loopback address and port, selected by the Go supervisor.
- `CODEX_MANAGER_GATEWAY_API_KEY`: generated per installation and passed only through the child environment.
- `CODEX_MANAGER_HOME`: manager-owned account/status directory.
- `CODEX_MANAGER_MODELS_CACHE`: read-only model cache path.
- `CODEX_MANAGER_GATEWAY_UPSTREAM`: upstream Responses endpoint.
- `CODEX_MANAGER_PROXY`: optional outbound proxy.

The supervisor must reject non-loopback listen addresses unless an explicit future security policy permits them.

## Readiness and shutdown

- `GET /health` returns `200` and `ok` once the listener is accepting requests.
- The supervisor owns process lifetime and sends a graceful termination signal.
- A non-zero exit, bind failure, malformed configuration, or crash-loop is surfaced as a typed backend error.

## HTTP API

- `GET /health`: unauthenticated liveness probe on loopback only.
- `GET /v1/models`: requires `Authorization: Bearer <gateway-key>`.
- `POST /v1/responses`: requires the same bearer key; request/stream semantics are Responses API compatible.
- `X-Conversation-ID` or `X-Thread-ID` identifies a durable chat binding. These identifiers contain no secret.

## Secret and data rules

- Never put the gateway key in a URL, command-line argument, transcript, or log.
- Go owns account credentials and provides the sidecar only the manager home path; Rust reads account files and status snapshots.
- API responses to Abolqasem contain model/status metadata only and must redact access/refresh tokens.
- The sidecar must not modify `~/.codex/auth.json`.

## Compatibility

Go checks a sidecar version/health response before enabling Manager mode. If the sidecar is absent or incompatible, the previous provider configuration remains active and no running turn is interrupted.
