export async function fetchSessions(project = "") {
  const params = new URLSearchParams({ limit: "100" });
  if (project) {
    params.set("project", project);
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

export function connectSessionEvents(onEvent) {
  const source = new EventSource("/api/events");
  source.onmessage = (event) => {
    onEvent(JSON.parse(event.data));
  };
  return source;
}
