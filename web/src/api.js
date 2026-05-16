export async function fetchSessions(options = {}) {
  const params = new URLSearchParams({
    limit: String(options.limit || 100),
    offset: String(options.offset || 0),
  });
  if (options.project) {
    params.set("project", options.project);
  }

  const response = await fetch(`/api/sessions?${params.toString()}`);
  if (!response.ok) {
    throw new Error("Failed to fetch sessions");
  }
  return response.json();
}

export async function fetchMessages(sessionKey, options = {}) {
  const params = new URLSearchParams({ limit: String(options.limit || 40) });
  if (options.before) {
    params.set("before", options.before);
  }

  const response = await fetch(`/api/session/${encodeURIComponent(sessionKey)}/messages?${params.toString()}`);
  if (!response.ok) {
    throw new Error("Failed to fetch messages");
  }
  return response.json();
}

export async function updateSession(sessionKey, payload = {}) {
  const response = await fetch(`/api/session/${encodeURIComponent(sessionKey)}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error("Failed to update session");
  }
  return response.json();
}

export async function fetchAppSettings() {
  const response = await fetch("/api/settings");
  if (!response.ok) {
    throw new Error("Failed to fetch app settings");
  }
  return response.json();
}

export async function updateAppSettings(payload = {}) {
  const response = await fetch("/api/settings", {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    throw new Error("Failed to update app settings");
  }
  return response.json();
}

export async function reloadSessions() {
  const response = await fetch("/api/actions/reload-sessions", { method: "POST" });
  if (!response.ok) {
    throw new Error("Failed to reload sessions");
  }
  return response.json();
}

export async function restartServer() {
  const response = await fetch("/api/actions/restart-server", { method: "POST" });
  if (!response.ok) {
    throw new Error("Failed to restart server");
  }
  return response.json();
}

export async function fetchHookStatus() {
  const response = await fetch("/api/hooks/status");
  if (!response.ok) {
    throw new Error("Failed to fetch hook status");
  }
  return response.json();
}

export async function fetchAgentStatus() {
  const response = await fetch("/api/agent/status");
  if (!response.ok) {
    throw new Error("Failed to fetch agent status");
  }
  return response.json();
}

export async function sendCodexTurn(payload = {}) {
  const response = await fetch("/api/agent/codex/turn", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(detail.trim() || "Failed to send Codex turn");
  }
  return response.json();
}

export async function sendAgentTurn(payload = {}) {
  const response = await fetch("/api/agent/turn", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(detail.trim() || "Failed to send agent turn");
  }
  return response.json();
}

export async function fetchSessionSearch(query, options = {}) {
  const params = new URLSearchParams({
    q: query || "",
    limit: String(options.limit || 100),
    offset: String(options.offset || 0),
  });

  const response = await fetch(`/api/search?${params.toString()}`);
  if (!response.ok) {
    throw new Error("Failed to search sessions");
  }
  return response.json();
}

export async function fetchFilePreview(options = {}) {
  const params = new URLSearchParams({
    path: options.path || "",
  });
  if (options.sessionKey) {
    params.set("session_key", options.sessionKey);
  }
  if (options.line) {
    params.set("line", String(options.line));
  }
  if (options.full) {
    params.set("full", "1");
  }

  const response = await fetch(`/api/file-preview?${params.toString()}`);
  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new Error(detail.trim() || "Failed to fetch file preview");
  }
  return response.json();
}

export function connectSessionEvents(onEvent) {
  const source = new EventSource("/api/events");
  source.onmessage = (event) => {
    onEvent(JSON.parse(event.data));
  };
  return source;
}
