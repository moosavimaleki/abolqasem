import { connectSessionEvents, fetchFilePreview, fetchMessages, fetchSessionSearch, fetchSessions } from "./api.js";
import { renderMessageContent } from "./content-renderer.js";
import { renderReaderDocument } from "./reader-renderer.js";
import { applySettings, clampFontSize, loadSettings, saveSettings, syncSettingsUI } from "./settings.js";
import {
  agentLabel,
  copyText,
  debounce,
  escapeHTML,
  formatClock,
  formatTime,
  isNearBottom,
  normalizeMessage,
  scrollToBottom,
  sessionStatus,
} from "./utils.js";

const SESSION_PAGE_SIZE = 100;

export function createViewerApp() {
  const state = {
    sessions: [],
    sessionsNextOffset: 0,
    sessionsHasMore: true,
    sessionsLoading: false,
    sessionsTotal: 0,
    searchResults: [],
    searchResultsNextOffset: 0,
    searchResultsHasMore: false,
    searchResultsLoading: false,
    searchResultsTotal: 0,
    searchRequestId: 0,
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
    sessionNoticeTimer: null,
    sessionNoticeInterval: null,
    filePreviewRequestId: 0,
    filePreviewDirectURL: "",
    filePreviewFromRoute: false,
    stickyReaderFrame: 0,
    stickyReaderMessageId: "",
  };

  const els = {
    body: document.body,
    sessionsRail: document.getElementById("sessions-rail"),
    sessionsList: document.getElementById("sessions-list"),
    sessionCount: document.getElementById("session-count"),
    sessionTitle: document.getElementById("session-title"),
    chat: document.getElementById("chat-messages"),
    stickyReaderAction: document.getElementById("sticky-reader-action"),
    openSessions: document.getElementById("open-sessions"),
    closeSessions: document.getElementById("close-sessions"),
    jumpToEnd: document.getElementById("jump-to-end"),
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
    filePreview: document.getElementById("file-preview"),
    filePreviewTitle: document.getElementById("file-preview-title"),
    filePreviewMeta: document.getElementById("file-preview-meta"),
    filePreviewBody: document.getElementById("file-preview-body"),
    openFilePreviewLink: document.getElementById("open-file-preview-link"),
    closeFilePreview: document.getElementById("close-file-preview"),
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
      openInitialFileRoute();
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
    els.messageSearch.addEventListener("keydown", handleMessageSearchKeydown);
    els.searchScopeChip.addEventListener("click", () => {
      removeSessionSearchScope();
    });
    els.refreshSession.addEventListener("click", refreshCurrentSession);
    els.jumpToEnd.addEventListener("click", () => {
      scrollToBottom(els.chat);
      syncJumpToEnd();
      syncStickyReaderActions();
    });
    els.stickyReaderAction.addEventListener("click", () => {
      if (state.stickyReaderMessageId) {
        openReader(state.stickyReaderMessageId);
      }
    });

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
    els.openFilePreviewLink.addEventListener("click", () => {
      if (state.filePreviewDirectURL) {
        window.open(state.filePreviewDirectURL, "_blank", "noopener,noreferrer");
      }
    });
    els.closeFilePreview.addEventListener("click", closeFilePreview);
    els.filePreview.addEventListener("click", (event) => {
      if (!state.filePreviewFromRoute && event.target === els.filePreview) {
        closeFilePreview();
      }
    });

    els.chat.addEventListener("scroll", async () => {
      syncJumpToEnd();
      scheduleStickyReaderSync();
      if (!state.currentSessionKey || !state.hasMoreBefore || state.loadingOlder || state.search) {
        return;
      }
      if (els.chat.scrollTop > 120) {
        return;
      }
      await loadOlderMessages();
    });
    els.chat.addEventListener("click", handleContentLinkClick);
    els.readerContent.addEventListener("click", handleContentLinkClick);
    els.sessionsList.addEventListener("scroll", () => {
      hideSessionTooltip();
      maybeLoadMoreSessions();
    });
    window.addEventListener("resize", () => {
      hideSessionTooltip();
      scheduleStickyReaderSync();
    });

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
      if (!els.filePreview.classList.contains("hidden") && !state.filePreviewFromRoute) {
        closeFilePreview();
        return;
      }
      if (!els.filePreview.classList.contains("hidden")) {
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
    if (state.sessionsLoading) {
      return;
    }
    const reset = options.reset !== false;
    const offset = reset ? 0 : state.sessionsNextOffset;
    if (!reset && !state.sessionsHasMore) {
      return;
    }

    state.sessionsLoading = true;
    if (reset) {
      state.sessionsNextOffset = 0;
      state.sessionsHasMore = true;
    } else {
      renderSessions(getVisibleSessions());
    }

    let failed = false;
    try {
      const data = await fetchSessions({
        limit: SESSION_PAGE_SIZE,
        offset,
      });
      const page = data.items || [];
      state.sessions = reset ? page : mergeSessionPages(state.sessions, page);
      state.sessionsNextOffset = Number(data.next_offset || 0);
      state.sessionsHasMore = state.sessionsNextOffset > 0;
      state.sessionsTotal = Number(data.total || state.sessions.length);
      renderSessions(getVisibleSessions());
      syncSessionCount();

      if (options.loadInitial !== false && !state.currentSessionKey && state.sessions.length > 0) {
        await loadSession(state.sessions[0].key);
      }
      if (state.sessions.length === 0) {
        renderEmptyState("هنوز نشستی ثبت نشده است.", "ai-agent-manager install --all --scope user");
      }
    } catch (error) {
      failed = true;
      console.error(error);
      els.sessionCount.textContent = "خطا";
      renderEmptyState("نشست‌ها بارگذاری نشدند.", "ai-agent-manager install");
    } finally {
      state.sessionsLoading = false;
      if (!failed) {
        renderSessions(getVisibleSessions());
        syncSessionCount();
      }
    }
  }

  async function loadMoreSessions() {
    if (isGlobalSearchActive()) {
      await loadGlobalSearchResults({ reset: false });
      return;
    }
    await loadSessionList({ reset: false, loadInitial: false });
  }

  async function loadGlobalSearchResults(options = {}) {
    const query = state.search;
    if (!query) {
      resetGlobalSearchResults();
      renderSessions(getVisibleSessions());
      syncSessionCount();
      return;
    }
    if (state.searchResultsLoading) {
      return;
    }
    const reset = options.reset !== false;
    const offset = reset ? 0 : state.searchResultsNextOffset;
    if (!reset && !state.searchResultsHasMore) {
      return;
    }

    const requestId = reset ? state.searchRequestId + 1 : state.searchRequestId;
    state.searchRequestId = requestId;
    state.searchResultsLoading = true;
    if (reset) {
      state.searchResults = [];
      state.searchResultsNextOffset = 0;
      state.searchResultsHasMore = false;
      state.searchResultsTotal = 0;
      renderSessions([]);
      renderGlobalSearchState();
      syncSessionCount();
    } else {
      renderSessions(getVisibleSessions());
    }

    try {
      const data = await fetchSessionSearch(query, {
        limit: SESSION_PAGE_SIZE,
        offset,
      });
      if (requestId !== state.searchRequestId || query !== state.search) {
        return;
      }
      const page = data.items || [];
      state.searchResults = reset ? page : mergeSessionPages(state.searchResults, page);
      state.sessions = mergeSessionPages(state.sessions, page);
      state.searchResultsNextOffset = Number(data.next_offset || 0);
      state.searchResultsHasMore = state.searchResultsNextOffset > 0;
      state.searchResultsTotal = Number(data.total || state.searchResults.length);
      state.searchResultsLoading = false;
      renderSessions(getVisibleSessions());
      renderGlobalSearchState();
      syncSessionCount();
    } catch (error) {
      if (requestId === state.searchRequestId) {
        console.error(error);
        renderGlobalSearchError();
      }
    } finally {
      if (requestId === state.searchRequestId) {
        state.searchResultsLoading = false;
        renderSessions(getVisibleSessions());
        renderGlobalSearchState();
        syncSessionCount();
      }
    }
  }

  function maybeLoadMoreSessions() {
    if (isGlobalSearchActive()) {
      if (state.searchResultsLoading || !state.searchResultsHasMore) {
        return;
      }
    } else if (state.sessionsLoading || !state.sessionsHasMore) {
      return;
    }
    const distanceFromBottom = els.sessionsList.scrollHeight - els.sessionsList.scrollTop - els.sessionsList.clientHeight;
    if (distanceFromBottom < 280) {
      loadMoreSessions();
    }
  }

  function mergeSessionPages(existing, incoming) {
    const merged = new Map();
    existing.forEach((session) => merged.set(session.key, session));
    incoming.forEach((session) => merged.set(session.key, session));
    return [...merged.values()].sort((a, b) => new Date(b.updated_at || 0) - new Date(a.updated_at || 0));
  }

  function resetGlobalSearchResults() {
    state.searchRequestId += 1;
    state.searchResults = [];
    state.searchResultsNextOffset = 0;
    state.searchResultsHasMore = false;
    state.searchResultsLoading = false;
    state.searchResultsTotal = 0;
  }

  function syncSessionCount() {
    if (isGlobalSearchActive()) {
      if (state.searchResultsLoading && state.searchResults.length === 0) {
        els.sessionCount.textContent = "در حال جست‌وجو";
        return;
      }
      const loaded = state.searchResults.length;
      els.sessionCount.textContent = state.searchResultsHasMore
        ? `${loaded}+ نتیجه`
        : `${loaded} نتیجه`;
      return;
    }
    if (state.sessionsLoading && state.sessions.length === 0) {
      els.sessionCount.textContent = "در حال بارگذاری";
      return;
    }
    const loaded = state.sessions.length;
    const total = Math.max(state.sessionsTotal || loaded, loaded);
    els.sessionCount.textContent = state.sessionsHasMore
      ? `${loaded} از ${total} نشست`
      : `${total} نشست`;
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
      empty.textContent = state.searchResultsLoading
        ? "در حال جست‌وجو در متن نشست‌ها..."
        : state.search && state.searchScope === "all"
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
      const sessionLabel = formatSessionListLabel(session);
      title.textContent = sessionLabel;
      title.title = sessionLabel;
      item.title = sessionLabel;
      const searchSnippet = renderSessionSearchSnippet(session);

      const info = document.createElement("span");
      info.className = "session-info";
      info.dataset.icon = "info";
      info.setAttribute("aria-label", "اطلاعات نشست");
      info.addEventListener("mouseenter", () => showSessionTooltip(info, session));
      info.addEventListener("mousemove", () => positionSessionTooltip(info, getSessionTooltip()));
      info.addEventListener("mouseleave", hideSessionTooltip);

      item.append(info, title);
      if (searchSnippet) {
        item.appendChild(searchSnippet);
      }
      item.addEventListener("click", () => {
        els.sessionsRail.classList.remove("open");
        loadSession(session.key);
      });
      els.sessionsList.appendChild(item);
    });
    renderSessionListFooter();
  }

  function renderSessionListFooter() {
    if (isGlobalSearchActive()) {
      if (state.searchResults.length === 0) {
        return;
      }
      const footer = document.createElement("div");
      footer.className = "session-list-footer";
      if (state.searchResultsLoading) {
        footer.textContent = "در حال جست‌وجوی بیشتر...";
      } else if (state.searchResultsHasMore) {
        footer.textContent = "برای نتایج بیشتر اسکرول کنید";
      } else {
        footer.textContent = "همه نتایج بارگذاری شد";
      }
      els.sessionsList.appendChild(footer);
      return;
    }
    if (state.sessions.length === 0) {
      return;
    }

    const footer = document.createElement("div");
    footer.className = "session-list-footer";
    if (state.sessionsLoading) {
      footer.textContent = "در حال بارگذاری نشست‌های بیشتر...";
    } else if (state.sessionsHasMore) {
      footer.textContent = "برای نشست‌های بیشتر اسکرول کنید";
    } else {
      footer.textContent = "همه نشست‌ها بارگذاری شد";
    }
    els.sessionsList.appendChild(footer);
  }

  function renderSessionTooltip(session) {
    const fragment = document.createDocumentFragment();
    [
      ["مدل", agentLabel(session.agent)],
      ["وضعیت", sessionStatus(session)],
      ["شروع", formatTime(session.updated_at)],
      ["مسیر", session.cwd || "نامشخص"],
      ["اولین پیام", summarizeSessionFirstPreview(session)],
      ["آخرین پیام", summarizeSessionPreview(session)],
    ].forEach(([label, value]) => {
      const row = document.createElement("span");
      const key = document.createElement("b");
      key.textContent = `${label}:`;
      row.append(key, document.createTextNode(` ${value}`));
      fragment.appendChild(row);
    });
    (session.search_matches || []).slice(0, 2).forEach((match) => {
      const row = document.createElement("span");
      const key = document.createElement("b");
      key.textContent = "نمونه:";
      row.append(key, document.createTextNode(` ${match.snippet || ""}`));
      fragment.appendChild(row);
    });
    return fragment;
  }

  function renderSessionSearchSnippet(session) {
    const match = (session.search_matches || [])[0];
    if (!match?.snippet) {
      return null;
    }
    const snippet = document.createElement("span");
    snippet.className = "session-search-snippet";
    snippet.textContent = match.snippet;
    snippet.title = match.snippet;
    return snippet;
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
    resetGlobalSearchResults();
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
      syncJumpToEnd();
    }
    scheduleStickyReaderSync();
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
    card.dataset.messageId = message.id;

    if (message.role === "system") {
      const label = document.createElement("p");
      label.className = "message-label";
      label.textContent = "رخداد";
      card.appendChild(label);
    }

    const content = renderMessageContent(message, "chat");
    card.appendChild(content);

    if (message.role === "assistant") {
      card.appendChild(renderAssistantActions(message));
    }

    if (message.role === "user" && message.createdAtLabel) {
      const meta = document.createElement("span");
      meta.className = "message-time";
      meta.textContent = formatClock(message.created_at);
      card.appendChild(meta);
    }

    return card;
  }

  function renderAssistantActions(message) {
    const actions = document.createElement("div");
    actions.className = "message-card-actions";
    const copyAction = iconAction("content_copy", "کپی پاسخ", () => copyText(message.text));
    const readerAction = iconAction("menu_book", "حالت خواندن", () => openReader(message.id));
    readerAction.classList.add("message-reader-action");
    actions.append(
      copyAction,
      readerAction,
    );
    return actions;
  }

  function scheduleStickyReaderSync() {
    if (state.stickyReaderFrame) {
      return;
    }
    state.stickyReaderFrame = window.requestAnimationFrame(() => {
      state.stickyReaderFrame = 0;
      syncStickyReaderActions();
    });
  }

  function syncStickyReaderActions() {
    if (!els.chat) {
      return;
    }
    const chatRect = els.chat.getBoundingClientRect();
    const shellRect = els.chat.parentElement.getBoundingClientRect();
    const floatingBottom = readPixelVariable(els.chat, "--floating-action-bottom", 44);
    const actionSize = els.stickyReaderAction.offsetHeight || 42;
    const actionTop = shellRect.bottom - floatingBottom - actionSize;
    const actionCenterY = actionTop + (actionSize / 2);
    let activeMessageId = "";
    let activeLeft = 0;

    els.chat.querySelectorAll(".message-card.assistant").forEach((card) => {
      if (activeMessageId) {
        return;
      }
      const readerAction = card.querySelector(".message-reader-action");
      if (!readerAction) {
        return;
      }

      const cardRect = card.getBoundingClientRect();
      const readerRect = readerAction.getBoundingClientRect();
      const cardContainsAction = cardRect.top < actionCenterY && cardRect.bottom > actionCenterY;
      const readerVisible = rectIntersects(readerRect, chatRect, 4);
      if (!cardContainsAction || readerVisible) {
        return;
      }

      activeMessageId = messageIdFromCard(card);
      activeLeft = Math.max(12, cardRect.left - shellRect.left - actionSize - 16);
    });

    state.stickyReaderMessageId = activeMessageId;
    els.stickyReaderAction.style.left = `${activeLeft}px`;
    els.stickyReaderAction.classList.toggle("hidden", !activeMessageId);
    els.stickyReaderAction.classList.toggle("is-visible", Boolean(activeMessageId));
    els.stickyReaderAction.tabIndex = activeMessageId ? 0 : -1;
    els.stickyReaderAction.setAttribute("aria-hidden", activeMessageId ? "false" : "true");
  }

  function rectIntersects(rect, containerRect, inset = 0) {
    return rect.bottom > containerRect.top + inset
      && rect.top < containerRect.bottom - inset
      && rect.right > containerRect.left + inset
      && rect.left < containerRect.right - inset;
  }

  function readPixelVariable(element, name, fallback) {
    const raw = getComputedStyle(element).getPropertyValue(name).trim();
    const value = Number.parseFloat(raw);
    return Number.isFinite(value) ? value : fallback;
  }

  function messageIdFromCard(card) {
    return card.dataset.messageId || "";
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

    renderReaderDocument({
      contentRoot: els.readerContent,
      tocRoot: els.readerTocList,
      scrollRoot: els.readerScroll,
      message,
      title: state.currentSessionName || "حالت خواندن",
      search: state.reader.search,
    });
  }

  function handleContentLinkClick(event) {
    const link = event.target.closest("a");
    if (!link) {
      return;
    }

    const reference = parseLocalFileReference(link.href);
    if (!reference) {
      return;
    }

    event.preventDefault();
    openFilePreview(reference, { sessionKey: state.currentSessionKey });
  }

  function parseLocalFileReference(href) {
    let url;
    try {
      url = new URL(href, window.location.href);
    } catch {
      return null;
    }

    const host = url.hostname.toLowerCase();
    const isLoopback = host === "127.0.0.1" || host === "localhost" || host === "::1" || host === "[::1]";
    if (!isLoopback || normalizePort(url) !== normalizePort(window.location)) {
      return null;
    }

    let path = decodeURIComponent(url.pathname);
    let line = 0;
    const match = path.match(/^(.+):(\d+)(?::\d+)?$/);
    if (match) {
      path = match[1];
      line = Number(match[2]);
    }

    if (/^\/[a-zA-Z]:\//.test(path)) {
      path = path.slice(1);
    }
    if (!looksLikeLocalFilePath(path)) {
      return null;
    }

    return {
      path,
      line,
      directURL: buildLocalFileURL(path, line),
    };
  }

  function normalizePort(locationLike) {
    if (locationLike.port) {
      return locationLike.port;
    }
    return locationLike.protocol === "https:" ? "443" : "80";
  }

  function looksLikeLocalFilePath(path) {
    return /^\/(home|Users|tmp|var)\//.test(path) || /^[a-zA-Z]:\//.test(path);
  }

  function buildLocalFileURL(path, line = 0) {
    const normalizedPath = path.startsWith("/") ? path : `/${path}`;
    const suffix = line ? `:${line}` : "";
    return `${window.location.origin}${encodeURI(normalizedPath)}${suffix}`;
  }

  function openInitialFileRoute() {
    const reference = parseLocalFileReference(window.location.href);
    if (!reference) {
      return;
    }
    openFilePreview(reference, { fromRoute: true });
  }

  async function openFilePreview(reference, options = {}) {
    const requestId = ++state.filePreviewRequestId;
    state.filePreviewFromRoute = Boolean(options.fromRoute);
    state.filePreviewDirectURL = reference.directURL || buildLocalFileURL(reference.path, reference.line);
    showFilePreviewLoading(reference);

    try {
      const preview = await fetchFilePreview({
        sessionKey: options.sessionKey || "",
        path: reference.path,
        line: reference.line,
        full: true,
      });
      if (requestId !== state.filePreviewRequestId) {
        return;
      }
      renderFilePreview(preview);
    } catch (error) {
      if (requestId !== state.filePreviewRequestId) {
        return;
      }
      console.error(error);
      showFilePreviewError(filePreviewErrorMessage(error));
    }
  }

  function showFilePreviewLoading(reference) {
    syncFilePreviewMode();
    els.filePreview.classList.remove("is-markdown");
    els.filePreviewTitle.textContent = filePreviewTitle(reference.path, reference.line);
    els.filePreviewMeta.textContent = reference.path;
    els.filePreviewBody.replaceChildren();
    const loading = document.createElement("p");
    loading.className = "file-preview-loading";
    loading.textContent = "در حال خواندن فایل...";
    els.filePreviewBody.appendChild(loading);
    els.filePreview.classList.remove("hidden");
    els.filePreview.setAttribute("aria-hidden", "false");
  }

  function renderFilePreview(preview) {
    syncFilePreviewMode();
    els.filePreviewTitle.textContent = filePreviewTitle(preview.path, preview.line);
    els.filePreviewMeta.textContent = preview.line ? `${preview.path}:${preview.line}` : preview.path;
    els.filePreviewBody.replaceChildren();

    if (shouldRenderMarkdownPreview(preview)) {
      renderMarkdownFilePreview(preview);
      return;
    }
    renderCodeFilePreview(preview);
  }

  function shouldRenderMarkdownPreview(preview) {
    return state.filePreviewFromRoute && preview.language === "markdown" && preview.html;
  }

  function renderMarkdownFilePreview(preview) {
    els.filePreview.classList.add("is-markdown");

    const layout = document.createElement("div");
    layout.className = "reader-layout file-preview-reader-layout";

    const toc = document.createElement("aside");
    toc.className = "reader-toc file-preview-reader-toc";
    toc.setAttribute("aria-label", "فهرست بخش‌ها");
    const tocTitle = document.createElement("p");
    tocTitle.textContent = "فهرست";
    const tocList = document.createElement("nav");
    toc.append(tocTitle, tocList);

    const scroll = document.createElement("main");
    scroll.className = "reader-scroll file-preview-reader-scroll";

    const content = document.createElement("div");
    content.className = "reader-content file-preview-reader-content";

    scroll.appendChild(content);
    layout.append(scroll, toc);
    els.filePreviewBody.appendChild(layout);

    renderReaderDocument({
      contentRoot: content,
      tocRoot: tocList,
      scrollRoot: scroll,
      title: baseName(preview.path),
      message: {
      role: "assistant",
      text: "",
      html: preview.html,
      },
    });
  }

  function renderCodeFilePreview(preview) {
    els.filePreview.classList.remove("is-markdown");

    const code = document.createElement("div");
    code.className = "file-preview-code";
    let highlightedRow = null;
    (preview.lines || []).forEach((line) => {
      const row = document.createElement("div");
      row.className = "file-preview-line";
      row.classList.toggle("is-highlighted", Boolean(line.highlight));
      if (line.highlight) {
        highlightedRow = row;
      }

      const number = document.createElement("span");
      number.className = "file-preview-line-number";
      number.textContent = String(line.number);

      const content = document.createElement("span");
      content.className = "file-preview-line-code";
      content.innerHTML = highlightPreviewLine(line.text || "", preview.language);

      row.append(number, content);
      code.appendChild(row);
    });

    els.filePreviewBody.appendChild(code);
    if (highlightedRow) {
      window.requestAnimationFrame(() => {
        highlightedRow.scrollIntoView({ block: "center" });
      });
    }
  }

  function highlightPreviewLine(text, language) {
    if (!window.hljs) {
      return escapeHTML(text);
    }
    try {
      if (language && language !== "plaintext" && window.hljs.getLanguage?.(language)) {
        return window.hljs.highlight(text, { language, ignoreIllegals: true }).value;
      }
      return escapeHTML(text);
    } catch (error) {
      console.warn("File preview highlighting failed", error);
      return escapeHTML(text);
    }
  }

  function closeFilePreview() {
    if (state.filePreviewFromRoute) {
      return;
    }
    state.filePreviewRequestId += 1;
    state.filePreviewFromRoute = false;
    state.filePreviewDirectURL = "";
    syncFilePreviewMode();
    els.filePreview.classList.add("hidden");
    els.filePreview.setAttribute("aria-hidden", "true");
    els.filePreviewBody.replaceChildren();
  }

  function showFilePreviewError(message) {
    syncFilePreviewMode();
    els.filePreview.classList.remove("is-markdown");
    els.filePreviewBody.replaceChildren();
    const error = document.createElement("p");
    error.className = "file-preview-error";
    error.textContent = message;
    els.filePreviewBody.appendChild(error);
    els.filePreview.classList.remove("hidden");
    els.filePreview.setAttribute("aria-hidden", "false");
  }

  function syncFilePreviewMode() {
    const routeMode = Boolean(state.filePreviewFromRoute);
    els.filePreview.classList.toggle("is-route", routeMode);
    els.filePreview.classList.toggle("is-modal", !routeMode);
    els.openFilePreviewLink.classList.toggle("hidden", routeMode);
    els.closeFilePreview.classList.toggle("hidden", routeMode);
  }

  function filePreviewErrorMessage(error) {
    const message = String(error?.message || "");
    if (message.includes("not allowed")) {
      return "این فایل خارج از مسیر پروژه‌های شناخته‌شده است و برای امنیت نمایش داده نمی‌شود.";
    }
    if (message.includes("too large")) {
      return "این فایل برای preview خیلی بزرگ است.";
    }
    if (message.includes("supported text/code")) {
      return "این مسیر جزو فایل‌های متنی/کدی مجاز برای preview نیست.";
    }
    if (message.includes("not found")) {
      return "فایل یا خط مورد نظر پیدا نشد.";
    }
    return "نمایش فایل ناموفق بود.";
  }

  function baseName(path) {
    return String(path || "").split(/[\\/]/).filter(Boolean).pop() || "file";
  }

  function filePreviewTitle(path, line) {
    return line ? `${baseName(path)}:${line}` : baseName(path);
  }

  function renderEmptyState(text, code) {
    els.chat.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.innerHTML = code ? `<p>${text}</p><code>${code}</code>` : `<p>${text}</p>`;
    els.chat.appendChild(empty);
    syncJumpToEnd();
  }

  function syncJumpToEnd() {
    const shouldShow = state.currentSessionKey && !isNearBottom(els.chat, 260) && els.chat.scrollHeight > els.chat.clientHeight + 260;
    els.jumpToEnd.classList.toggle("hidden", !shouldShow);
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
      resetGlobalSearchResults();
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

  function handleMessageSearchKeydown(event) {
    if (event.key !== "Backspace" || state.searchScope !== "session" || els.messageSearch.value) {
      return;
    }
    event.preventDefault();
    removeSessionSearchScope();
  }

  function removeSessionSearchScope() {
    state.searchScope = "all";
    syncSearchScope();
    runSearch();
    els.messageSearch.focus();
  }

  function runSearch() {
    if (state.searchScope === "all") {
      if (state.search) {
        loadGlobalSearchResults({ reset: true });
        return;
      }
      resetGlobalSearchResults();
      renderSessions(getVisibleSessions());
      renderMessages();
      return;
    }
    resetGlobalSearchResults();
    renderSessions(getVisibleSessions());
    renderMessages();
  }

  function getVisibleSessions() {
    if (isGlobalSearchActive()) {
      return state.searchResults;
    }
    return state.sessions;
  }

  function isGlobalSearchActive() {
    return Boolean(state.search && state.searchScope === "all");
  }

  function messageMatchesQuery(message, query) {
    return `${message.roleLabel} ${message.text}`.toLowerCase().includes(query);
  }

  function renderGlobalSearchState() {
    els.chat.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-state";
    if (state.searchResultsLoading) {
      empty.textContent = "در حال جست‌وجو در متن همه نشست‌ها...";
    } else {
      const resultCount = state.searchResults.length;
      empty.textContent = resultCount === 0
        ? "نتیجه‌ای در متن نشست‌ها پیدا نشد."
        : `${resultCount}${state.searchResultsHasMore ? "+" : ""} نشست در متن گفت‌وگوها پیدا شد. نتیجه‌ها در فهرست سمت راست هستند.`;
    }
    els.chat.appendChild(empty);
  }

  function renderGlobalSearchError() {
    els.chat.replaceChildren();
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "جست‌وجو در متن نشست‌ها انجام نشد.";
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
      if (event.session_key && event.session_key === state.currentSessionKey) {
        loadSession(state.currentSessionKey).then(() => {
          if (shouldStick) {
            scrollToBottom(els.chat);
          }
        });
        return;
      }
      showSessionNotice(event);
    });
  }

  function getSessionNotice() {
    let notice = document.getElementById("session-notice");
    if (notice) {
      return notice;
    }

    notice = document.createElement("div");
    notice.id = "session-notice";
    notice.className = "session-notice hidden";
    notice.setAttribute("role", "status");

    const text = document.createElement("span");
    text.className = "session-notice-text";

    const action = document.createElement("button");
    action.type = "button";
    action.className = "session-notice-action";

    const close = document.createElement("button");
    close.type = "button";
    close.className = "inline-icon session-notice-close";
    close.dataset.icon = "close";
    close.setAttribute("aria-label", "بستن اعلان نشست");
    close.addEventListener("click", hideSessionNotice);

    notice.append(text, action, close);
    els.body.appendChild(notice);
    return notice;
  }

  function showSessionNotice(event) {
    if (!event?.session_key) {
      return;
    }

    const notice = getSessionNotice();
    const title = event.project_name || event.session_id || "نشست جدید";
    const durationSeconds = 8;
    let remainingSeconds = durationSeconds;
    const action = notice.querySelector(".session-notice-action");

    const goToSession = async () => {
      hideSessionNotice();
      await loadSessionList({ loadInitial: false });
      if (state.sessions.some((item) => item.key === event.session_key)) {
        await loadSession(event.session_key);
      }
    };
    const updateCountdown = () => {
      action.textContent = `رفتن به این چت (${remainingSeconds})`;
      action.style.setProperty("--notice-progress", String(Math.max(0, remainingSeconds / durationSeconds)));
    };

    notice.querySelector(".session-notice-text").textContent = `نشست به‌روزرسانی شد: ${title}`;
    action.onclick = goToSession;
    updateCountdown();

    notice.classList.remove("hidden");
    window.requestAnimationFrame(() => notice.classList.add("is-visible"));

    window.clearTimeout(state.sessionNoticeTimer);
    window.clearInterval(state.sessionNoticeInterval);
    state.sessionNoticeInterval = window.setInterval(() => {
      remainingSeconds -= 1;
      updateCountdown();
      if (remainingSeconds <= 0) {
        goToSession();
      }
    }, 1000);
    state.sessionNoticeTimer = window.setTimeout(goToSession, durationSeconds * 1000);
  }

  function hideSessionNotice() {
    const notice = document.getElementById("session-notice");
    if (!notice) {
      return;
    }
    notice.classList.remove("is-visible");
    window.clearTimeout(state.sessionNoticeTimer);
    window.clearInterval(state.sessionNoticeInterval);
    state.sessionNoticeTimer = window.setTimeout(() => {
      notice.classList.add("hidden");
    }, 180);
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

  function summarizeSessionFirstPreview(session) {
    const preview = normalizeInlineText(session.first_preview);
    if (!preview) {
      return session.metadata_only ? "متن نشست در دسترس نیست." : "پیام اولی ثبت نشده است.";
    }
    return preview;
  }

  function formatSessionListLabel(session) {
    const projectName = normalizeInlineText(session.project_name || session.session_id || "نشست بدون نام");
    const firstPreview = normalizeInlineText(session.first_preview);
    return firstPreview ? `${projectName} / ${firstPreview}` : projectName;
  }

  function normalizeInlineText(value) {
    return String(value || "").replace(/\s+/g, " ").trim();
  }

  function firstLine(text) {
    const value = String(text || "").trim().split("\n")[0] || "خروجی ابزار";
    return value.length > 80 ? `${value.slice(0, 80)}...` : value;
  }
}
