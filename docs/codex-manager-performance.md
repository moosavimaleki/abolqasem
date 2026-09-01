# Codex Manager performance guardrails

- The history API caps a response at 1,000 samples and the chart renders at
  most 120 evenly-spaced points.
- Account cards request only one latest sample per account. Network requests
  run concurrently and Settings does not wait for a maintenance check.
- Maintenance scheduler runs are single-flight with exponential backoff; a
  slow check cannot overlap the next one.
- Sidecar readiness and account operations use bounded request contexts.

Run `go test ./... -run '^$' -bench HistoryAppendAndRead -benchmem` when
comparing releases. A large history must remain readable without blocking the
chat UI; if a benchmark regresses, reduce the API limit or increase the
sampling interval rather than loading the full JSONL file in the browser.
