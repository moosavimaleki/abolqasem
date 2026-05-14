import { connectSessionEvents, fetchMessages, fetchSessions } from "./api.js";
import { buildReaderTOC, renderMessageContent } from "./content-renderer.js";
import { applySettings, clampFontSize, loadSettings, saveSettings, syncSettingsUI } from "./settings.js";
import {
  agentLabel,
  copyText,
  debounce,
  formatClock,
  formatTime,
  isNearBottom,
  normalizeMessage,
  scrollToBottom,
  sessionStatus,
} from "./utils.js";

export function createViewerApp() {
  const state = {
    sessions: [],
    messages: [],
    visibleMessages: [],
    currentSessionKey: "",
    currentSessionName: "",
    oldestCursor: "",
    hasMoreBefore: false,
    loadingOlder: false,
    loadRequestId: 0,
    search: "",
    searchScope: "session",
    reader: {
      open: false,
      messageId: "",
      search: "",
    },
    settings: loadSettings(),
    eventSource: null,
  };

  const els = {
    body: document.body,
    sessionsRail: document.getElementById("sessions-rail"),
    sessionsList: document.getElementById("sessions-list"),
    sessionCount: document.getElementById("session-count"),
    sessionTitle: document.getElementById("session-title"),
    chat: document.getElementById("chat-messages"),
    openSessions: document.getElementById("open-sessions"),
    closeSessions: document.getElementById("close-sessions"),
    messageSearchShell: document.getElementById("message-search-shell"),
    messageSearchToggle: document.getElementById("message-search-toggle"),
    searchScopeChip: document.getElementById("search-scope-chip"),
    messageSearch: document.getElementById("message-search"),
    refreshSession: document.getElementById("refresh-session"),
    activeSessionInfo: document.getElementById("active-session-info"),
    sessionInfoPopover: document.getElementById("session-info-popover"),
    sessionInfoBody: document.getElementById("session-info-body"),
    closeSessionInfo: document.getElementById("close-session-info"),
    openSettings: document.getElementById("open-settings"),
    settingsPopover: document.getElementById("settings-popover"),
    closeSettings: document.getElementById("close-settings"),
    readerPage: document.getElementById("reader-page"),
    readerScroll: document.getElementById("reader-scroll"),
    readerContent: document.getElementById("reader-content"),
    readerTocList: document.getElementById("reader-toc-list"),
    closeReader: document.getElementById("close-reader"),
    readerSettings: document.getElementById("reader-settings"),
    readerSearchShell: document.getElementById("reader-search-shell"),
    readerSearchToggle: document.getElementById("reader-search-toggle"),
    readerSearch: document.getElementById("reader-search"),
  };

  return {
    start() {
      applySettings(state.settings);
      syncSettingsUI(state.settings, els.settingsPopover);
      bindEvents();
      loadSessionList();
      state.eventSource = connectSessionEvents(handleSessionEvent);
    },
  };

  function bindEvents() {
    els.openSessions.addEventListener("click", () => {
      els.sessionsRail.classList.add("open");
    });
    els.closeSessions.addEventListener("click", () => {
      els.sessionsRail.classList.remove("open");
    });

    els.messageSearchToggle.addEventListener("click", () => {
      toggleSearch(els.messageSearchShell, els.messageSearch);
    });
    els.messageSearch.addEventListener("input", debounce(() => {
      state.search = els.messageSearch.value.trim().toLowerCase();
      runSearch();
    }, 120));
    els.searchScopeChip.addEventListener("click", () => {
      state.searchScope = "all";
      syncSearchScope();
      runSearch();
      els.messageSearch.focus();
    });
    els.refreshSession.addEventListener("click", refreshCurrentSession);

    els.activeSessionInfo.addEventListener("click", () => {
      toggleSessionInfoPopover();
    });
    els.closeSessionInfo.addEventListener("click", closeSessionInfoPopover);

    els.openSettings.addEventListener("click", () => {
      toggleSettingsPopover(els.openSettings);
    });
    els.readerSettings.addEventListener("click", () => {
      toggleSettingsPopover(els.readerSettings);
    });
    els.closeSettings.addEventListener("click", closeSettingsPopover);
    els.settingsPopover.addEventListener("click", handleSettingsClick);

    els.closeReader.addEventListener("click", closeReader);
    els.readerSearchToggle.addEventListener("click", () => {
      toggleSearch(els.readerSearchShell, els.readerSearch);
    });
    els.readerSearch.addEventListener("input", debounce(() => {
      state.reader.search = els.readerSearch.value.trim();
      renderReader();
    }, 120));

    els.chat.addEventListener("scroll", async () => {
      if (!state.currentSessionKey || !state.hasMoreBefore || state.loadingOlder || state.search) {
        return;
      }
      if (els.chat.scrollTop > 120) {
        return;
      }
      await loadOlderMessages();
    });
    els.sessionsList.addEventListener("scroll", hideSessionTooltip);
    window.addEventListener("resize", hideSessionTooltip);

    document.addEventListener("click", (event) => {
      const target = event.target;
      if (!els.settingsPopover.classList.contains("hidden")
        && !els.settingsPopover.contains(target)
        && !target.closest("#open-settings")
        && !target.closest("#reader-settings")) {
        closeSettingsPopover();
      }
      if (!els.sessionInfoPopover.classList.contains("hidden")
        && !els.sessionInfoPopover.contains(target)
        && !target.closest("#active-session-info")) {
        closeSessionInfoPopover();
      }
    });

    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") {
        return;
      }
      if (!els.settingsPopover.classList.contains("hidden")) {
        closeSettingsPopover();
        return;
      }
      if (!els.sessionInfoPopover.classList.contains("hidden")) {
        closeSessionInfoPopover();
        return;
      }
      if (state.reader.open) {
        closeReader();
        return;
      }
      collapseSearch(els.messageSearchShell, els.messageSearch);
    });
  }

  async function loadSessionList(options = {}) {
    try {
      const data = await fetchSessions();
      state.sessions = data.items || [];
      renderSessions(getVisibleSessions());
      els.sessionCount.textContent = `${state.sessions.length} نشست`;

      if (options.loadInitial !== false && !state.currentSessionKey && state.sessions.length > 0) {
        await loadSession(state.sessions[0].key);
      }
      if (state.sessions.length === 0) {
        renderEmptyState("هنوز نشستی ثبت نشده است.", "ai-session-viewer install --all --scope user");
      }
    } catch (error) {
      console.error(error);
      els.sessionCount.textContent = "خطا";
      renderEmptyState("نشست‌ها بارگذاری نشدند.", "ai-session-viewer server");
    }
  }

  async function refreshCurrentSession() {
    if (els.refreshSession.disabled) {
      return;
    }

    const sessionKey = state.currentSessionKey;
    els.refreshSession.disabled = true;
    els.refreshSession.classList.add("is-loading");
    try {
      await loadSessionList({ loadInitial: false });
      if (sessionKey && state.sessions.some((item) => item.key === sessionKey)) {
        await loadSession(sessionKey);
        return;
      }
      if (state.sessions.length > 0) {
        await loadSession(state.sessions[0].key);
      }
    } finally {
      els.refreshSession.disabled = false;
      els.refreshSession.classList.remove("is-loading");
    }
  }

  function renderSessions(sessions) {
    hideSessionTooltip();
    els.sessionsList.replaceChildren();

    if (sessions.length === 0) {
      const empty = document.createElement("div");
      empty.className = "session-list-empty";
      empty.textContent = state.search && state.searchScope === "all"
        ? "نشستی با این جست‌وجو پیدا نشد."
        : "نشستی وجود ندارد.";
      els.sessionsList.appendChild(empty);
      return;
    }

    sessions.forEach((session) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "session-item";
      item.classList.toggle("active", session.key === state.currentSessionKey);

      const title = document.createElement("span");
      title.className = "session-name";
      title.textContent = session.project_name || session.session_id || "نشست بدون نام";

      const info = document.createElement("span");
      info.className = "session-info";
      info.dataset.icon = "info";
      info.setAttribute("aria-label", "اطلاعات نشست");
      info.addEventListener("mouseenter", () => showSessionTooltip(info, session));
      info.addEventListener("mousemove", () => positionSessionTooltip(info, getSessionTooltip()));
      info.addEventListener("mouseleave", hideSessionTooltip);

      item.append(info, title);
      item.addEventListener("click", () => {
        els.sessionsRail.classList.remove("open");
        loadSession(session.key);
      });
      els.sessionsList.appendChild(item);
    });
  }

  function renderSessionTooltip(session) {
    const fragment = document.createDocumentFragment();
    [
      ["مدل", agentLabel(session.agent)],
      ["وضعیت", sessionStatus(session)],
      ["شروع", formatTime(session.updated_at)],
      ["مسیر", session.cwd || "نامشخص"],
      ["آخرین پیام", summarizeSessionPreview(session)],
    ].forEach(([label, value]) => {
      const row = document.createElement("span");
      const key = document.createElement("b");
      key.textContent = `${label}:`;
      row.append(key, document.createTextNode(` ${value}`));
      fragment.appendChild(row);
    });
    return fragment;
  }

  function getSessionTooltip() {
    let tooltip = document.getElementById("session-tooltip");
    if (!tooltip) {
      tooltip = document.createElement("span");
      tooltip.id = "session-tooltip";
      tooltip.className = "session-tooltip hidden";
      tooltip.setAttribute("role", "tooltip");
      tooltip.setAttribute("aria-hidden", "true");
      els.body.appendChild(tooltip);
    }
    return tooltip;
  }

  function showSessionTooltip(anchor, session) {
    const tooltip = getSessionTooltip();
    tooltip.replaceChildren(renderSessionTooltip(session));
    tooltip.classList.remove("hidden", "is-visible", "placed-left", "placed-right");
    tooltip.setAttribute("aria-hidden", "false");
    positionSessionTooltip(anchor, tooltip);
    window.requestAnimationFrame(() => tooltip.classList.add("is-visible"));
  }

  function hideSessionTooltip() {
    const tooltip = document.getElementById("session-tooltip");
    if (!tooltip) {
      return;
    }
    tooltip.classList.add("hidden");
    tooltip.classList.remove("is-visible", "placed-left", "placed-right");
    tooltip.setAttribute("aria-hidden", "true");
  }

  function positionSessionTooltip(anchor, tooltip) {
    if (!anchor || !tooltip || tooltip.classList.contains("hidden")) {
      return;
    }

    const rect = anchor.getBoundingClientRect();
    const margin = 12;
    const gap = 16;
    const width = tooltip.offsetWidth || 320;
    const height = tooltip.offsetHeight || 160;
    const placeLeft = rect.left >= width + gap + margin;
    const left = placeLeft
      ? rect.left - width - gap
      : Math.min(window.innerWidth - width - margin, rect.right + gap);
    const top = Math.max(margin, Math.min(window.innerHeight - height - margin, rect.top + (rect.height / 2) - (height / 2)));

    tooltip.style.left = `${Math.max(margin, left)}px`;
    tooltip.style.top = `${top}px`;
    tooltip.classList.toggle("placed-left", placeLeft);
    tooltip.classList.toggle("placed-right", !placeLeft);
  }

  async function loadSession(sessionKey) {
    const requestId = ++state.loadRequestId;
    const session = state.sessions.find((item) => item.key === sessionKey);
    state.currentSessionKey = sessionKey;
    state.currentSessionName = session?.project_name || session?.session_id || "نشست بدون نام";
    state.messages = [];
    state.visibleMessages = [];
    state.oldestCursor = "";
    state.hasMoreBefore = false;
    state.search = "";
    state.searchScope = "session";
    els.messageSearch.value = "";
    syncSearchScope();
    collapseSearch(els.messageSearchShell, els.messageSearch);
    els.sessionTitle.textContent = state.currentSessionName;
    renderSessions(getVisibleSessions());
    renderEmptyState("در حال بارگذاری نشست...", "");
    closeSessionInfoPopover();

    try {
      const data = await fetchMessages(sessionKey, { limit: 40 });
      if (requestId !== state.loadRequestId) {
        return;
      }
      if (data.status === "metadata_only") {
        renderEmptyState("متن این نشست در دسترس نیست.", "");
        return;
      }
      state.messages = (data.items || []).map(normalizeMessage);
      state.oldestCursor = data.oldest_cursor || "";
      state.hasMoreBefore = Boolean(data.has_more_before);
      renderMessages({ stickBottom: true });
    } catch (error) {
      if (requestId !== state.loadRequestId) {
        return;
      }
      console.error(error);
      renderEmptyState("پیام‌های نشست بارگذاری نشدند.", "");
    }
  }

  async function loadOlderMessages() {
    state.loadingOlder = true;
    const offsetFromBottom = els.chat.scrollHeight - els.chat.scrollTop;

    try {
      const data = await fetchMessages(state.currentSessionKey, {
        limit: 40,
        before: state.oldestCursor,
      });
      state.messages = [...(data.items || []).map(normalizeMessage), ...state.messages];
      state.oldestCursor = data.oldest_cursor || state.oldestCursor;
      state.hasMoreBefore = Boolean(data.has_more_before);
      renderMessages();
      els.chat.scrollTop = Math.max(0, els.chat.scrollHeight - offsetFromBottom);
    } catch (error) {
      console.error(error);
    } finally {
      state.loadingOlder = false;
    }
  }

  function renderMessages(options = {}) {
    els.chat.replaceChildren();

    if (state.messages.length === 0) {
      renderEmptyState("پیامی برای نمایش وجود ندارد.", "");
      return;
    }

    const filtered = state.search && state.searchScope === "session"
      ? state.messages.filter((message) => messageMatchesQuery(message, state.search))
      : state.messages;
    state.visibleMessages = filtered;

    if (state.hasMoreBefore && !state.search) {
      els.chat.appendChild(renderLoadMoreButton());
    }

    filtered.forEach((message) => {
      els.chat.appendChild(renderMessageCard(message));
    });

    if (filtered.length === 0) {
      renderEmptyState("نتیجه‌ای برای جست‌وجو پیدا نشد.", "");
      return;
    }

    if (options.stickBottom) {
      scrollToBottom(els.chat);
    }
  }

  function renderLoadMoreButton() {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "load-more";
    button.textContent = state.loadingOlder ? "در حال بارگذاری..." : "پیام‌های قدیمی‌تر";
    button.addEventListener("click", loadOlderMessages);
    return button;
  }

  function renderMessageCard(message) {
    if (message.role === "tool") {
      return renderToolMessage(message);
    }

    const card = document.createElement("article");
    card.className = `message-card ${message.roleClass}`;
    card.id = message.domId;

    if (message.role === "assistant") {
      card.appendChild(renderAssistantActions(message, "top"));
    }

    if (message.role === "system") {
      const label = document.createElement("p");
      label.className = "message-label";
      label.textContent = "رخداد";
      card.appendChild(label);
    }

    const content = renderMessageContent(message, "chat");
    card.appendChild(content);

    if (message.role === "assistant") {
      card.appendChild(renderAssistantActions(message, "bottom"));
    }

    if (message.role === "user" && message.createdAtLabel) {
      const meta = document.createElement("span");
      meta.className = "message-time";
      meta.textContent = formatClock(message.created_at);
      card.appendChild(meta);
    }

    return card;
  }

  function renderAssistantActions(message, position) {
    const actions = document.createElement("div");
    actions.className = `message-card-actions ${position}`;
    actions.append(
      iconAction("content_copy", "کپی پاسخ", () => copyText(message.text)),
      iconAction("menu_book", "حالت خواندن", () => openReader(message.id)),
    );
    return actions;
  }

  function renderToolMessage(message) {
    const details = document.createElement("details");
    details.className = "tool-message";

    const summary = document.createElement("summary");
    const title = document.createElement("span");
    title.textContent = `ابزار: ${firstLine(message.text)}`;
    const status = document.createElement("b");
    summary.append(title, status);
    details.appendChild(summary);

    const pre = document.createElement("pre");
    pre.textContent = message.text || "";
    details.appendChild(pre);
    return details;
  }

  function iconAction(icon, label, handler) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "inline-icon";
    button.dataset.icon = icon;
    button.setAttribute("aria-label", label);
    button.title = label;
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      handler();
    });
    return button;
  }

  function openReader(messageId) {
    state.reader.open = true;
    state.reader.messageId = messageId;
    state.reader.search = "";
    els.readerSearch.value = "";
    collapseSearch(els.readerSearchShell, els.readerSearch);
    els.body.classList.add("reader-active");
    els.readerPage.classList.remove("hidden");
    els.readerPage.setAttribute("aria-hidden", "false");
    renderReader();
  }

  function closeReader() {
    state.reader.open = false;
    state.reader.messageId = "";
    closeSettingsPopover();
    els.body.classList.remove("reader-active");
    els.readerPage.classList.add("hidden");
    els.readerPage.setAttribute("aria-hidden", "true");
  }

  function renderReader() {
    if (!state.reader.open) {
      return;
    }

    const message = state.messages.find((item) => item.id === state.reader.messageId);
    if (!message) {
      closeReader();
      return;
    }

    els.readerContent.replaceChildren();
    const article = document.createElement("article");
    article.className = "reader-article";

    const hasPrimaryHeading = /<h1\b/i.test(message.html || "");
    if (!hasPrimaryHeading) {
      const title = document.createElement("h1");
      title.textContent = state.currentSessionName || "حالت خواندن";
      article.appendChild(title);
    }

    article.appendChild(renderMessageContent(message, "reader", state.reader.search));
    els.readerContent.appendChild(article);
    buildReaderTOC(els.readerContent, els.readerTocList);
    els.readerScroll.scrollTop = 0;
  }

  function renderEmptyState(text, code) {
    els.chat.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.innerHTML = code ? `<p>${text}</p><code>${code}</code>` : `<p>${text}</p>`;
    els.chat.appendChild(empty);
  }

  function toggleSearch(shell, input) {
    const collapsed = shell.classList.toggle("collapsed");
    if (!collapsed) {
      syncSearchScope();
      input.focus();
      return;
    }
    input.value = "";
    if (input === els.messageSearch) {
      state.search = "";
      state.searchScope = "session";
      syncSearchScope();
      renderSessions(getVisibleSessions());
      renderMessages();
    }
    if (input === els.readerSearch) {
      state.reader.search = "";
      renderReader();
    }
  }

  function collapseSearch(shell, input) {
    shell.classList.add("collapsed");
    input.value = "";
  }

  function syncSearchScope() {
    const scopedToSession = state.searchScope === "session";
    els.searchScopeChip.classList.toggle("hidden", !scopedToSession);
    els.messageSearch.placeholder = scopedToSession ? "جست‌وجو در همین نشست" : "جست‌وجو در همه نشست‌ها";
    els.messageSearch.setAttribute("aria-label", els.messageSearch.placeholder);
  }

  function runSearch() {
    if (state.searchScope === "all") {
      renderSessions(getVisibleSessions());
      if (state.search) {
        renderGlobalSearchState();
        return;
      }
    } else {
      renderSessions(getVisibleSessions());
    }
    renderMessages();
  }

  function getVisibleSessions() {
    if (!state.search || state.searchScope !== "all") {
      return state.sessions;
    }
    return state.sessions.filter((session) => sessionMatchesQuery(session, state.search));
  }

  function sessionMatchesQuery(session, query) {
    return [
      session.project_name,
      session.session_id,
      session.cwd,
      session.agent,
      session.last_preview,
    ].some((value) => String(value || "").toLowerCase().includes(query));
  }

  function messageMatchesQuery(message, query) {
    return `${message.roleLabel} ${message.text}`.toLowerCase().includes(query);
  }

  function renderGlobalSearchState() {
    els.chat.replaceChildren();
    const resultCount = getVisibleSessions().length;
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = `${resultCount} نشست در جست‌وجوی همه نشست‌ها پیدا شد. نتیجه‌ها در فهرست سمت راست فیلتر شده‌اند.`;
    els.chat.appendChild(empty);
  }

  function toggleSettingsPopover(anchor) {
    if (!els.settingsPopover.classList.contains("hidden")
      && els.settingsPopover.dataset.anchor === anchor.id) {
      closeSettingsPopover();
      return;
    }

    els.settingsPopover.dataset.anchor = anchor.id;
    els.settingsPopover.classList.remove("hidden");
    els.settingsPopover.setAttribute("aria-hidden", "false");
    positionSettingsPopover(anchor);
  }

  function closeSettingsPopover() {
    els.settingsPopover.classList.add("hidden");
    els.settingsPopover.setAttribute("aria-hidden", "true");
    els.settingsPopover.dataset.anchor = "";
  }

  function toggleSessionInfoPopover() {
    if (!els.sessionInfoPopover.classList.contains("hidden")) {
      closeSessionInfoPopover();
      return;
    }

    renderActiveSessionInfo();
    const rect = els.activeSessionInfo.getBoundingClientRect();
    els.sessionInfoPopover.classList.remove("hidden");
    els.sessionInfoPopover.setAttribute("aria-hidden", "false");
    const width = els.sessionInfoPopover.offsetWidth || 360;
    const top = rect.bottom + 12;
    const left = Math.max(16, Math.min(window.innerWidth - width - 16, rect.left - width + rect.width));
    els.sessionInfoPopover.style.top = `${top}px`;
    els.sessionInfoPopover.style.left = `${left}px`;
  }

  function closeSessionInfoPopover() {
    els.sessionInfoPopover.classList.add("hidden");
    els.sessionInfoPopover.setAttribute("aria-hidden", "true");
  }

  function renderActiveSessionInfo() {
    els.sessionInfoBody.replaceChildren();
    const session = currentSession();
    if (!session) {
      els.sessionInfoBody.textContent = "نشستی انتخاب نشده است.";
      return;
    }

    [
      ["نام", session.project_name || "نشست بدون نام"],
      ["شناسه", session.session_id || "نامشخص"],
      ["کلید", session.key || "نامشخص"],
      ["عامل", agentLabel(session.agent)],
      ["مسیر اجرا", session.cwd || "نامشخص"],
      ["وضعیت", sessionStatus(session)],
      ["آخرین فعالیت", formatTime(session.updated_at)],
      ["تعداد پیام", `${session.message_count_estimate || state.messages.length || 0}`],
      ["مسیر transcript", session.transcript_path || "نامشخص"],
    ].forEach(([label, value]) => {
      els.sessionInfoBody.appendChild(infoRow(label, value));
    });
  }

  function infoRow(label, value) {
    const row = document.createElement("div");
    row.className = "session-info-row";
    const key = document.createElement("span");
    key.textContent = label;
    const val = document.createElement("code");
    val.textContent = value;
    row.append(key, val);
    return row;
  }

  function positionSettingsPopover(anchor) {
    const rect = anchor.getBoundingClientRect();
    const popover = els.settingsPopover;
    const top = rect.bottom + 12;
    const width = popover.offsetWidth || 320;
    const left = Math.max(16, Math.min(window.innerWidth - width - 16, rect.left - width + rect.width));
    popover.style.top = `${top}px`;
    popover.style.left = `${left}px`;
  }

  function handleSettingsClick(event) {
    const target = event.target.closest("button");
    if (!target) {
      return;
    }

    if (target.id === "font-size-decrease") {
      updateSettings({ fontSize: clampFontSize(state.settings.fontSize - 1) });
      return;
    }
    if (target.id === "font-size-increase") {
      updateSettings({ fontSize: clampFontSize(state.settings.fontSize + 1) });
      return;
    }

    const group = target.closest("[data-setting]");
    if (!group || !target.dataset.value) {
      return;
    }

    const key = group.dataset.setting;
    const value = key === "lineHeight" ? Number(target.dataset.value) : target.dataset.value;
    updateSettings({ [key]: value });
  }

  function updateSettings(patch) {
    state.settings = { ...state.settings, ...patch };
    saveSettings(state.settings);
    applySettings(state.settings);
    syncSettingsUI(state.settings, els.settingsPopover);
    if (state.reader.open) {
      renderReader();
    }
  }

  function handleSessionEvent(event) {
    const shouldStick = isNearBottom(els.chat);
    loadSessionList().then(() => {
      if (event.session_key === state.currentSessionKey) {
        loadSession(state.currentSessionKey).then(() => {
          if (shouldStick) {
            scrollToBottom(els.chat);
          }
        });
      }
    });
  }

  function currentSession() {
    return state.sessions.find((item) => item.key === state.currentSessionKey) || null;
  }

  function summarizeSessionPreview(session) {
    const preview = String(session.last_preview || "").replace(/\s+/g, " ").trim();
    if (!preview) {
      return session.metadata_only ? "متن نشست در دسترس نیست." : "پیش‌نمایشی ثبت نشده است.";
    }
    return preview.length > 180 ? `${preview.slice(0, 180)}...` : preview;
  }

  function firstLine(text) {
    const value = String(text || "").trim().split("\n")[0] || "خروجی ابزار";
    return value.length > 80 ? `${value.slice(0, 80)}...` : value;
  }
}
