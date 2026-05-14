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

export async function fetchFilePreview(options = {}) {
  const params = new URLSearchParams({
    session_key: options.sessionKey || "",
    path: options.path || "",
    line: String(options.line || 1),
  });

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
