export function debounce(fn, delay) {
  let handle = null;
  return (...args) => {
    clearTimeout(handle);
    handle = setTimeout(() => fn(...args), delay);
  };
}

export function formatTime(value) {
  if (!value) {
    return "زمان نامشخص";
  }
  return new Date(value).toLocaleString("fa-IR", {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

export function formatClock(value) {
  if (!value) {
    return "--:--";
  }
  return new Date(value).toLocaleTimeString("fa-IR", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function agentLabel(agent) {
  switch (agent) {
    case "codex":
      return "کدکس";
    case "claude":
      return "کلود";
    case "gemini":
      return "جمینای";
    default:
      return agent || "عامل نامشخص";
  }
}

export function roleLabel(role) {
  switch (role) {
    case "user":
      return "شما";
    case "tool":
      return "ابزار";
    case "system":
      return "رخداد";
    default:
      return "دستیار";
  }
}

export function normalizeMessage(message) {
  const role = message.role || "assistant";
  const text = message.text || "";
  return {
    ...message,
    id: message.id || `message-${message.index}`,
    domId: `message-${message.index}`,
    role,
    roleClass: ["assistant", "user", "tool", "system"].includes(role) ? role : "assistant",
    roleLabel: roleLabel(role),
    text,
    createdAtLabel: message.created_at ? formatTime(message.created_at) : "",
  };
}

export function sessionStatus(session) {
  if (session.metadata_only) {
    return "متن در دسترس نیست";
  }
  if ((session.message_count_estimate || 0) === 0) {
    return "در انتظار متن";
  }
  return "آماده";
}

export function escapeHTML(text) {
  return String(text)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

export function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function stripHTML(value) {
  const wrapper = document.createElement("div");
  wrapper.innerHTML = value || "";
  return wrapper.textContent || "";
}

export function copyText(text) {
  return navigator.clipboard.writeText(text || "").catch((error) => {
    console.error(error);
  });
}

export function scrollToBottom(element) {
  let frame = 0;
  const pin = () => {
    element.scrollTo({ top: element.scrollHeight, behavior: "auto" });
    frame += 1;
    if (frame < 6) {
      requestAnimationFrame(pin);
    }
  };

  requestAnimationFrame(pin);
  window.setTimeout(() => {
    element.scrollTo({ top: element.scrollHeight, behavior: "auto" });
  }, 120);
}

export function isNearBottom(element, threshold = 120) {
  return element.scrollHeight - element.scrollTop - element.clientHeight <= threshold;
}
