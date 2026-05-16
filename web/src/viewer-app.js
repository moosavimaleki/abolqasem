import {
  connectSessionEvents,
  fetchAppSettings,
  fetchAgentStatus,
  fetchFilePreview,
  fetchHookStatus,
  fetchMessages,
  fetchSessionSearch,
  fetchSessions,
  reloadSessions,
  restartServer,
  sendAgentTurn,
  updateAppSettings,
  updateSession,
} from "./api.js";
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
const DEFAULT_APP_SETTINGS = {
  hook_updates: true,
  hook_follow_mode: "auto",
  ignore_hook_navigation_while_typing: true,
  filesystem_discovery: true,
  default_agent: "codex",
  agent_models: {
    codex: "",
    claude: "",
    gemini: "",
  },
};

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
    currentProjectName: "",
    oldestCursor: "",
    hasMoreBefore: false,
    loadingOlder: false,
    loadRequestId: 0,
    search: "",
    searchScope: "session",
    editingSessionTitle: false,
    projectSessionCounts: {},
    reader: {
      open: false,
      messageId: "",
      search: "",
    },
    settings: loadSettings(),
    appSettings: { ...DEFAULT_APP_SETTINGS },
    eventSource: null,
    sessionNoticeTimer: null,
    sessionNoticeInterval: null,
    appSettingsStatusTimer: null,
    agentStatus: { agents: [], codex: { available: false } },
    composerAgent: DEFAULT_APP_SETTINGS.default_agent,
    agentSending: false,
    composerNewSession: false,
    filePreviewRequestId: 0,
    filePreviewDirectURL: "",
    filePreviewFromRoute: false,
    stickyReaderFrame: 0,
    stickyReaderMessageId: "",
    projectSessions: {
      open: false,
      project: "",
      items: [],
      nextOffset: 0,
      hasMore: false,
      total: 0,
      loading: false,
      requestId: 0,
    },
  };

  const els = {
    body: document.body,
    sessionsRail: document.getElementById("sessions-rail"),
    sessionsList: document.getElementById("sessions-list"),
    sessionCount: document.getElementById("session-count"),
    sessionHeader: document.getElementById("session-header"),
    sessionProject: document.getElementById("session-project"),
    sessionProjectLabel: document.getElementById("session-project-label"),
    sessionProjectBadge: document.getElementById("session-project-badge"),
    sessionTitleSeparator: document.getElementById("session-title-separator"),
    sessionTitleButton: document.getElementById("session-title-button"),
    sessionTitleInput: document.getElementById("session-title-input"),
    sessionTitle: document.getElementById("session-title"),
    chat: document.getElementById("chat-messages"),
    agentComposer: document.getElementById("agent-composer"),
    agentComposerLabel: document.getElementById("agent-composer-label"),
    agentComposerAgent: document.getElementById("agent-composer-agent"),
    agentComposerModel: document.getElementById("agent-composer-model"),
    agentModelOptions: document.getElementById("agent-model-options"),
    agentNewSession: document.getElementById("agent-new-session"),
    agentComposerInput: document.getElementById("agent-composer-input"),
    agentComposerSubmit: document.getElementById("agent-composer-submit"),
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
    appSettingsModal: document.getElementById("app-settings-modal"),
    closeAppSettings: document.getElementById("close-app-settings"),
    appSettingsReloadSessions: document.getElementById("app-settings-reload-sessions"),
    appSettingsCheckHooks: document.getElementById("app-settings-check-hooks"),
    appSettingsCopyURL: document.getElementById("app-settings-copy-url"),
    appSettingsRestartServer: document.getElementById("app-settings-restart-server"),
    appSettingsStatus: document.getElementById("app-settings-status"),
    appHooksStatus: document.getElementById("app-hooks-status"),
    filePreview: document.getElementById("file-preview"),
    filePreviewTitle: document.getElementById("file-preview-title"),
    filePreviewMeta: document.getElementById("file-preview-meta"),
    filePreviewBody: document.getElementById("file-preview-body"),
    openFilePreviewLink: document.getElementById("open-file-preview-link"),
    closeFilePreview: document.getElementById("close-file-preview"),
    projectSessionsModal: document.getElementById("project-sessions-modal"),
    projectSessionsMeta: document.getElementById("project-sessions-meta"),
    projectSessionsList: document.getElementById("project-sessions-list"),
    closeProjectSessions: document.getElementById("close-project-sessions"),
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
      loadAppSettings();
      loadAgentStatus();
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
    els.agentComposer.addEventListener("submit", handleAgentComposerSubmit);
    els.agentComposerAgent.addEventListener("change", handleComposerAgentChange);
    els.agentComposerModel.addEventListener("change", handleComposerModelChange);
    els.agentComposerInput.addEventListener("input", () => {
      autosizeComposer();
      syncAgentComposer();
    });
    els.agentComposerInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        els.agentComposer.requestSubmit();
      }
    });
    els.agentNewSession.addEventListener("click", () => {
      state.composerNewSession = !state.composerNewSession;
      syncAgentComposer();
      els.agentComposerInput.focus();
    });
    els.refreshSession.addEventListener("click", refreshCurrentSession);
    els.sessionProject.addEventListener("click", openProjectSessionsModal);
    els.sessionTitleButton.addEventListener("click", beginSessionTitleEdit);
    els.sessionTitleInput.addEventListener("keydown", handleSessionTitleInputKeydown);
    els.sessionTitleInput.addEventListener("blur", () => {
      if (state.editingSessionTitle) {
        commitSessionTitleEdit();
      }
    });
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
      openAppSettingsModal();
    });
    els.readerSettings.addEventListener("click", () => {
      toggleSettingsPopover(els.readerSettings);
    });
    els.closeSettings.addEventListener("click", closeSettingsPopover);
    els.settingsPopover.addEventListener("click", handleSettingsClick);
    els.closeAppSettings.addEventListener("click", closeAppSettingsModal);
    els.appSettingsModal.addEventListener("click", handleAppSettingsClick);
    els.appSettingsModal.addEventListener("change", handleAppSettingsChange);

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
    els.closeProjectSessions.addEventListener("click", closeProjectSessionsModal);
    els.projectSessionsModal.addEventListener("click", (event) => {
      if (event.target === els.projectSessionsModal) {
        closeProjectSessionsModal();
      }
    });
    els.projectSessionsList.addEventListener("scroll", maybeLoadMoreProjectSessions);
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
      if (state.editingSessionTitle
        && !els.sessionHeader.contains(target)
        && !target.closest("#session-title-button")) {
        commitSessionTitleEdit();
      }
    });

    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") {
        return;
      }
      if (!els.projectSessionsModal.classList.contains("hidden")) {
        closeProjectSessionsModal();
        return;
      }
      if (!els.appSettingsModal.classList.contains("hidden")) {
        closeAppSettingsModal();
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
      if (state.editingSessionTitle) {
        cancelSessionTitleEdit();
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
      state.sessions = dedupeSessions(state.sessions);
      state.sessionsNextOffset = Number(data.next_offset || 0);
      state.sessionsHasMore = state.sessionsNextOffset > 0;
      state.sessionsTotal = Number(data.total || state.sessions.length);
      if (state.currentSessionKey) {
        const active = currentSession();
        state.currentSessionName = sessionDisplayName(active);
        state.currentProjectName = normalizeInlineText(active?.project_name || "");
        syncCurrentSessionHeader();
        syncAgentComposer();
      }
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
    existing.forEach((session) => merged.set(sessionIdentity(session), session));
    incoming.forEach((session) => merged.set(sessionIdentity(session), session));
    return [...merged.values()].sort((a, b) => new Date(b.updated_at || 0) - new Date(a.updated_at || 0));
  }

  function dedupeSessions(items) {
    const merged = new Map();
    items.forEach((session) => {
      merged.set(sessionIdentity(session), session);
    });
    return [...merged.values()].sort((a, b) => new Date(b.updated_at || 0) - new Date(a.updated_at || 0));
  }

  function sessionIdentity(session) {
    if (!session) {
      return "";
    }
    const transcriptPath = normalizeInlineText(session.transcript_path || "");
    if (transcriptPath) {
      return transcriptPath;
    }
    const key = normalizeInlineText(session.key || "");
    if (key) {
      return key;
    }
    return [
      normalizeInlineText(session.agent || "unknown"),
      normalizeInlineText(session.session_id || ""),
      normalizeInlineText(session.project_name || ""),
    ].join(":");
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
        await refreshCurrentSessionContent({ stickBottom: true });
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

  async function refreshCurrentSessionContent(options = {}) {
    if (!state.currentSessionKey) {
      return;
    }

    const requestId = ++state.loadRequestId;
    const data = await fetchMessages(state.currentSessionKey, { limit: 40 });
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
    renderMessages({ stickBottom: options.stickBottom !== false });
    if (state.reader.open) {
      renderReader();
    }
  }

  function renderSessions(sessions) {
    hideSessionTooltip();
    els.sessionsList.replaceChildren();
    const uniqueSessions = dedupeSessions(sessions);

    if (uniqueSessions.length === 0) {
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

    uniqueSessions.forEach((session) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "session-item";
      item.classList.toggle("active", session.key === state.currentSessionKey);

      const textWrap = document.createElement("span");
      textWrap.className = "session-primary";

      const project = document.createElement("span");
      project.className = "session-project-name";
      project.textContent = normalizeInlineText(session.project_name || "پروژه نامشخص");

      const separator = document.createElement("span");
      separator.className = "session-name-separator";
      separator.setAttribute("aria-hidden", "true");

      const title = document.createElement("span");
      title.className = "session-name";
      const sessionLabel = sessionDisplayName(session);
      title.textContent = sessionLabel;

      textWrap.append(project, separator, title);
      item.title = `${project.textContent} / ${sessionLabel}`;
      const searchSnippet = renderSessionSearchSnippet(session);

      const info = document.createElement("span");
      info.className = "session-info";
      info.dataset.icon = "info";
      info.setAttribute("aria-label", "اطلاعات نشست");
      info.addEventListener("mouseenter", () => showSessionTooltip(info, session));
      info.addEventListener("mousemove", () => positionSessionTooltip(info, getSessionTooltip()));
      info.addEventListener("mouseleave", hideSessionTooltip);

      item.append(info, textWrap);
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
      ["نام نشست", sessionDisplayName(session)],
      ["پروژه", normalizeInlineText(session.project_name || "پروژه نامشخص")],
      ["عامل", agentLabel(session.agent)],
      ["مدل", normalizeInlineText(session.model || "نامشخص")],
      ["وضعیت", sessionStatus(session)],
      ["آخرین فعالیت", formatTime(session.updated_at)],
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

  async function loadSession(sessionKey, options = {}) {
    const requestId = ++state.loadRequestId;
    const hintedSession = options.sessionHint ? buildSessionStub(options.sessionHint) : null;
    const existingSession = state.sessions.find((item) => item.key === sessionKey);
    const session = existingSession || hintedSession;
    if (hintedSession && !existingSession) {
      state.sessions = mergeSessionPages(state.sessions, [hintedSession]);
    }
    state.currentSessionKey = sessionKey;
    state.currentSessionName = sessionDisplayName(session);
    state.currentProjectName = normalizeInlineText(session?.project_name || "");
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
    syncCurrentSessionHeader();
    syncAgentComposer();
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
      return dedupeSessions(state.searchResults);
    }
    return dedupeSessions(state.sessions);
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

  async function loadAppSettings() {
    try {
      state.appSettings = normalizeAppSettings(await fetchAppSettings());
      renderAppSettingsUI();
    } catch (error) {
      console.error(error);
    }
  }

  async function loadAgentStatus() {
    try {
      state.agentStatus = normalizeAgentStatus(await fetchAgentStatus());
    } catch (error) {
      console.error(error);
      state.agentStatus = normalizeAgentStatus();
    } finally {
      syncAgentComposer();
    }
  }

  function normalizeAgentStatus(status = {}) {
    const fallbackCodex = status.codex || { available: false, capabilities: {} };
    const agents = Array.isArray(status.agents) && status.agents.length > 0
      ? status.agents
      : [
        {
          agent: "codex",
          label: "کدکس",
          available: Boolean(fallbackCodex.available),
          controllable: true,
          default_model: fallbackCodex.default_model || "",
          models: Array.isArray(fallbackCodex.models) ? fallbackCodex.models : [],
          capabilities: fallbackCodex.capabilities || {},
        },
      ];
    const normalizedAgents = agents.map((item) => ({
      agent: normalizeAgentName(item.agent),
      label: item.label || agentLabel(item.agent),
      available: Boolean(item.available),
      controllable: Boolean(item.controllable),
      default_model: normalizeInlineText(item.default_model || ""),
      models: normalizeAgentModels(item.models || []),
      error: item.error || "",
      capabilities: item.capabilities || {},
    })).filter((item) => item.agent);
    return {
      ...status,
      agents: normalizedAgents,
      codex: {
        ...fallbackCodex,
        available: Boolean(fallbackCodex.available),
        models: normalizeAgentModels(fallbackCodex.models || []),
      },
    };
  }

  function normalizeAgentModels(models = []) {
    const seen = new Set();
    return models.map((item) => {
      const model = normalizeInlineText(item.model || item.id || "");
      if (!model || seen.has(model)) {
        return null;
      }
      seen.add(model);
      return {
        id: normalizeInlineText(item.id || model),
        model,
        display_name: normalizeInlineText(item.display_name || item.displayName || model),
        description: normalizeInlineText(item.description || ""),
        is_default: Boolean(item.is_default || item.isDefault),
      };
    }).filter(Boolean);
  }

  function normalizeAgentName(agent) {
    const value = normalizeInlineText(agent || "").toLowerCase();
    return ["codex", "claude", "gemini"].includes(value) ? value : "";
  }

  function getAgentStatus(agentName) {
    const normalized = normalizeAgentName(agentName);
    return state.agentStatus.agents.find((item) => item.agent === normalized) || null;
  }

  function getDefaultAgent() {
    const configured = normalizeAgentName(state.appSettings.default_agent);
    if (configured && getAgentStatus(configured)) {
      return configured;
    }
    return state.agentStatus.agents[0]?.agent || "codex";
  }

  function getSelectedAgent(session, newSession) {
    if (!newSession && session?.agent) {
      return normalizeAgentName(session.agent);
    }
    return normalizeAgentName(state.composerAgent) || getDefaultAgent();
  }

  function getConfiguredModel(agentName) {
    const agentKey = normalizeAgentName(agentName);
    return normalizeInlineText(state.appSettings.agent_models?.[agentKey] || "");
  }

  function getSuggestedDefaultModel(agentName) {
    return normalizeInlineText(getAgentStatus(agentName)?.default_model || "");
  }

  function getComposerModel(agentName) {
    return normalizeInlineText(els.agentComposerModel.value || "");
  }

  function formatComposerModelLabel(agentName, model) {
    if (model) {
      return model;
    }
    const suggested = getSuggestedDefaultModel(agentName);
    return suggested ? `پیش‌فرض agent (${suggested})` : "مدل پیش‌فرض agent";
  }

  function renderComposerAgentOptions(selectedAgent, locked) {
    els.agentComposerAgent.replaceChildren();
    const knownAgents = state.agentStatus.agents.length > 0
      ? state.agentStatus.agents
      : [{ agent: "codex", label: "کدکس", available: false, controllable: true, capabilities: {} }];
    knownAgents.forEach((item) => {
      const option = document.createElement("option");
      option.value = item.agent;
      option.textContent = item.label || agentLabel(item.agent);
      option.disabled = locked
        ? item.agent !== selectedAgent
        : !item.available || !item.controllable || !item.capabilities?.can_send;
      option.selected = item.agent === selectedAgent;
      els.agentComposerAgent.appendChild(option);
    });
    els.agentComposerAgent.value = selectedAgent;
    els.agentComposerAgent.disabled = locked || state.agentSending;
  }

  function renderComposerModelOptions(agentName) {
    const models = getAgentStatus(agentName)?.models || [];
    els.agentModelOptions.replaceChildren();
    models.forEach((item) => {
      const option = document.createElement("option");
      option.value = item.model;
      option.label = item.display_name || item.model;
      els.agentModelOptions.appendChild(option);
    });
    const configuredModel = getConfiguredModel(agentName);
    if (document.activeElement !== els.agentComposerModel) {
      els.agentComposerModel.value = configuredModel;
    }
    const suggestedDefault = getSuggestedDefaultModel(agentName);
    els.agentComposerModel.placeholder = suggestedDefault ? `default: ${suggestedDefault}` : "model";
    els.agentComposerModel.disabled = state.agentSending;
  }

  function syncAgentComposer() {
    const session = currentSession();
    const messageReady = normalizeInlineText(els.agentComposerInput.value).length > 0;
    const forcedNewSession = state.composerNewSession || !session || !state.currentSessionKey;
    const selectedAgent = getSelectedAgent(session, forcedNewSession);
    const selectedStatus = getAgentStatus(selectedAgent);
    const canSend = Boolean(selectedStatus?.available && selectedStatus?.controllable && selectedStatus?.capabilities?.can_send);
    const isSameAgentSession = Boolean(session?.agent && normalizeAgentName(session.agent) === selectedAgent);
    const hasCwd = Boolean(normalizeInlineText(session?.cwd || state.currentProjectName));
    const canContinue = canSend && isSameAgentSession && Boolean(state.currentSessionKey);
    const canStart = canSend && hasCwd;
    const newSession = forcedNewSession || !canContinue;
    const enabled = !state.agentSending && messageReady && (newSession ? canStart : canContinue);
    const model = getComposerModel(selectedAgent);
    const modelLabel = formatComposerModelLabel(selectedAgent, model);

    els.agentComposer.classList.toggle("is-sending", state.agentSending);
    els.agentComposer.classList.toggle("is-unavailable", !canSend);
    els.agentComposer.classList.toggle("is-new-session", newSession);
    els.agentComposerSubmit.disabled = !enabled;
    els.agentNewSession.disabled = state.agentSending || !canStart;
    els.agentNewSession.classList.toggle("active", newSession);
    renderComposerAgentOptions(selectedAgent, !newSession);
    renderComposerModelOptions(selectedAgent);

    if (state.agentSending) {
      els.agentComposerLabel.textContent = `${agentLabel(selectedAgent)} با ${modelLabel} در حال کار است...`;
      return;
    }
    if (!selectedStatus?.available) {
      els.agentComposerLabel.textContent = `${agentLabel(selectedAgent)} در PATH در دسترس نیست.`;
      return;
    }
    if (!selectedStatus?.controllable || !selectedStatus?.capabilities?.can_send) {
      els.agentComposerLabel.textContent = `ارسال پیام برای ${agentLabel(selectedAgent)} هنوز پیاده‌سازی نشده است.`;
      return;
    }
    if (!session) {
      els.agentComposerLabel.textContent = "برای شروع، یک نشست پروژه‌دار را انتخاب کنید.";
      return;
    }
    if (newSession) {
      els.agentComposerLabel.textContent = `نشست جدید ${agentLabel(selectedAgent)} با ${modelLabel} در ${normalizeInlineText(session.project_name || "این پروژه")}`;
      return;
    }
    if (canContinue) {
      els.agentComposerLabel.textContent = `ادامه همین نشست با ${agentLabel(selectedAgent)} / ${modelLabel}`;
      return;
    }
    els.agentComposerLabel.textContent = "این نشست فعلاً فقط قابل مشاهده است.";
  }

  async function handleComposerAgentChange() {
    const nextAgent = normalizeAgentName(els.agentComposerAgent.value) || "codex";
    state.composerAgent = nextAgent;
    await updateAppSettingsState({ default_agent: nextAgent }, { silent: true });
    syncAgentComposer();
  }

  async function handleComposerModelChange() {
    const session = currentSession();
    const forcedNewSession = state.composerNewSession || !session || !state.currentSessionKey;
    const agentName = getSelectedAgent(session, forcedNewSession);
    const model = normalizeInlineText(els.agentComposerModel.value || "");
    await updateAgentModelSetting(agentName, model, { silent: true });
    syncAgentComposer();
  }

  async function updateAgentModelSetting(agentName, model, options = {}) {
    const agentKey = normalizeAgentName(agentName);
    if (!agentKey) {
      return;
    }
    const agentModels = { ...(state.appSettings.agent_models || {}), [agentKey]: model };
    await updateAppSettingsState({ agent_models: agentModels }, options);
  }

  function autosizeComposer() {
    const input = els.agentComposerInput;
    input.style.height = "auto";
    input.style.height = `${Math.min(160, Math.max(42, input.scrollHeight))}px`;
  }

  async function handleAgentComposerSubmit(event) {
    event.preventDefault();
    if (state.agentSending) {
      return;
    }
    const message = els.agentComposerInput.value.trim();
    if (!message) {
      return;
    }
    const session = currentSession();
    const forcedNewSession = state.composerNewSession || !session || !state.currentSessionKey;
    const selectedAgent = getSelectedAgent(session, forcedNewSession);
    const selectedStatus = getAgentStatus(selectedAgent);
    const newSession = forcedNewSession || normalizeAgentName(session?.agent) !== selectedAgent;
    if (!selectedStatus?.available || !selectedStatus?.controllable || !selectedStatus?.capabilities?.can_send || !session?.cwd) {
      syncAgentComposer();
      return;
    }
    const model = getComposerModel(selectedAgent);

    state.agentSending = true;
    syncAgentComposer();
    appendPendingUserMessage(message);
    els.agentComposerInput.value = "";
    autosizeComposer();

    try {
      const data = await sendAgentTurn({
        agent: selectedAgent,
        session_key: newSession ? "" : state.currentSessionKey,
        cwd: session.cwd,
        message,
        new: newSession,
        model,
      });
      if (data.session) {
        applySessionUpdate(data.session);
      }
      const nextKey = data.session_key || data.session?.key || state.currentSessionKey;
      state.composerNewSession = false;
      if (nextKey) {
        await loadSession(nextKey, { sessionHint: data.session });
      }
      await loadSessionList({ loadInitial: false });
    } catch (error) {
      console.error(error);
      appendComposerError(error.message || "ارسال پیام انجام نشد.");
    } finally {
      state.agentSending = false;
      syncAgentComposer();
    }
  }

  function appendPendingUserMessage(text) {
    if (!state.currentSessionKey) {
      return;
    }
    const message = normalizeMessage({
      id: `pending-${Date.now()}`,
      index: state.messages.length + 1,
      role: "user",
      text,
      created_at: new Date().toISOString(),
    });
    state.messages = [...state.messages, message];
    renderMessages({ stickBottom: true });
  }

  function appendComposerError(text) {
    const wrapper = document.createElement("div");
    wrapper.className = "composer-error";
    wrapper.textContent = text;
    els.chat.appendChild(wrapper);
    scrollToBottom(els.chat);
  }

  function normalizeAppSettings(settings = {}) {
    const next = { ...DEFAULT_APP_SETTINGS, ...settings };
    next.agent_models = {
      ...DEFAULT_APP_SETTINGS.agent_models,
      ...(settings.agent_models || {}),
    };
    if (!["auto", "notice", "off"].includes(next.hook_follow_mode)) {
      next.hook_follow_mode = DEFAULT_APP_SETTINGS.hook_follow_mode;
    }
    next.default_agent = normalizeAgentName(next.default_agent) || DEFAULT_APP_SETTINGS.default_agent;
    Object.keys(next.agent_models).forEach((agentName) => {
      const normalized = normalizeAgentName(agentName);
      if (!normalized) {
        delete next.agent_models[agentName];
        return;
      }
      next.agent_models[normalized] = normalizeInlineText(next.agent_models[agentName] || "");
      if (normalized !== agentName) {
        delete next.agent_models[agentName];
      }
    });
    next.hook_updates = Boolean(next.hook_updates);
    next.ignore_hook_navigation_while_typing = Boolean(next.ignore_hook_navigation_while_typing);
    next.filesystem_discovery = Boolean(next.filesystem_discovery);
    state.composerAgent = normalizeAgentName(state.composerAgent || next.default_agent) || next.default_agent;
    return next;
  }

  function openAppSettingsModal() {
    renderAppSettingsUI();
    els.appSettingsModal.classList.remove("hidden");
    els.appSettingsModal.setAttribute("aria-hidden", "false");
    setAppSettingsStatus("");
  }

  function closeAppSettingsModal() {
    els.appSettingsModal.classList.add("hidden");
    els.appSettingsModal.setAttribute("aria-hidden", "true");
  }

  function renderAppSettingsUI() {
    const settings = state.appSettings;
    els.appSettingsModal.querySelectorAll("[data-app-toggle]").forEach((button) => {
      const key = button.dataset.appToggle;
      const enabled = Boolean(settings[key]);
      button.classList.toggle("active", enabled);
      button.setAttribute("aria-pressed", String(enabled));
    });
    els.appSettingsModal.querySelectorAll("[data-app-setting]").forEach((group) => {
      const key = group.dataset.appSetting;
      group.querySelectorAll("[data-value]").forEach((button) => {
        button.classList.toggle("active", String(button.dataset.value) === String(settings[key]));
      });
    });
    els.appSettingsModal.querySelectorAll("[data-app-agent-model]").forEach((input) => {
      const agentName = normalizeAgentName(input.dataset.appAgentModel);
      input.value = normalizeInlineText(settings.agent_models?.[agentName] || "");
    });
  }

  async function handleAppSettingsChange(event) {
    const input = event.target.closest("[data-app-agent-model]");
    if (!input || !els.appSettingsModal.contains(input)) {
      return;
    }
    const agentName = normalizeAgentName(input.dataset.appAgentModel);
    const model = normalizeInlineText(input.value || "");
    await updateAgentModelSetting(agentName, model);
    syncAgentComposer();
  }

  async function handleAppSettingsClick(event) {
    if (event.target === els.appSettingsModal) {
      closeAppSettingsModal();
      return;
    }

    const button = event.target.closest("button");
    if (!button || !els.appSettingsModal.contains(button)) {
      return;
    }

    const toggleKey = button.dataset.appToggle;
    if (toggleKey) {
      await updateAppSettingsState({ [toggleKey]: !Boolean(state.appSettings[toggleKey]) });
      return;
    }

    const group = button.closest("[data-app-setting]");
    if (group && button.dataset.value) {
      const key = group.dataset.appSetting;
      if (key === "default_agent") {
        state.composerAgent = normalizeAgentName(button.dataset.value) || state.composerAgent;
      }
      await updateAppSettingsState({ [key]: button.dataset.value });
      syncAgentComposer();
      return;
    }

    if (button.id === "app-settings-reload-sessions") {
      await runAppSettingsAction(button, "در حال بارگذاری نشست‌ها...", async () => {
        await reloadSessions();
        await loadSessionList({ loadInitial: false });
        setAppSettingsStatus("نشست‌ها دوباره بارگذاری شدند.");
      });
      return;
    }

    if (button.id === "app-settings-check-hooks") {
      await runAppSettingsAction(button, "در حال بررسی hookها...", async () => {
        const data = await fetchHookStatus();
        renderHookStatus(data.items || []);
        setAppSettingsStatus("وضعیت hookها به‌روزرسانی شد.");
      });
      return;
    }

    if (button.id === "app-settings-copy-url") {
      await copyText(window.location.origin);
      setAppSettingsStatus("آدرس محلی کپی شد.");
      return;
    }

    if (button.id === "app-settings-restart-server") {
      await runAppSettingsAction(button, "در حال ری‌استارت سرور...", async () => {
        await restartServer();
        setAppSettingsStatus("درخواست ری‌استارت ثبت شد. چند ثانیه بعد صفحه را تازه کنید.");
      });
    }
  }

  async function updateAppSettingsState(patch, options = {}) {
    const previous = state.appSettings;
    state.appSettings = normalizeAppSettings({ ...state.appSettings, ...patch });
    renderAppSettingsUI();
    try {
      state.appSettings = normalizeAppSettings(await updateAppSettings(patch));
      renderAppSettingsUI();
      if (!options.silent) {
        setAppSettingsStatus("تنظیمات ذخیره شد.");
      }
    } catch (error) {
      console.error(error);
      state.appSettings = previous;
      renderAppSettingsUI();
      if (!options.silent) {
        setAppSettingsStatus("ذخیره تنظیمات انجام نشد.");
      }
    }
  }

  async function runAppSettingsAction(button, pendingText, action) {
    if (button.disabled) {
      return;
    }
    button.disabled = true;
    setAppSettingsStatus(pendingText);
    try {
      await action();
    } catch (error) {
      console.error(error);
      setAppSettingsStatus("عملیات انجام نشد.");
    } finally {
      button.disabled = false;
    }
  }

  function renderHookStatus(items) {
    els.appHooksStatus.replaceChildren();
    if (items.length === 0) {
      els.appHooksStatus.classList.add("hidden");
      return;
    }
    items.forEach((item) => {
      const row = document.createElement("div");
      row.className = "app-hook-row";

      const agent = document.createElement("strong");
      agent.textContent = agentLabel(item.agent);

      const badges = document.createElement("div");
      badges.className = "app-hook-badges";
      badges.append(
        hookBadge("user", Boolean(item.user_installed)),
        hookBadge("project", Boolean(item.project_installed)),
      );
      if (item.error) {
        const error = document.createElement("span");
        error.className = "app-hook-badge";
        error.textContent = "error";
        error.title = item.error;
        badges.appendChild(error);
      }
      row.append(agent, badges);
      els.appHooksStatus.appendChild(row);
    });
    els.appHooksStatus.classList.remove("hidden");
  }

  function hookBadge(label, active) {
    const badge = document.createElement("span");
    badge.className = "app-hook-badge";
    badge.classList.toggle("active", active);
    badge.textContent = `${label}: ${active ? "installed" : "off"}`;
    return badge;
  }

  function setAppSettingsStatus(text) {
    els.appSettingsStatus.textContent = text;
    window.clearTimeout(state.appSettingsStatusTimer);
    if (!text) {
      return;
    }
    state.appSettingsStatusTimer = window.setTimeout(() => {
      els.appSettingsStatus.textContent = "";
    }, 4200);
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
      ["نام نشست", sessionDisplayName(session)],
      ["پروژه", normalizeInlineText(session.project_name || "پروژه نامشخص")],
      ["شناسه", session.session_id || "نامشخص"],
      ["کلید", session.key || "نامشخص"],
      ["عامل", agentLabel(session.agent)],
      ["مدل", normalizeInlineText(session.model || "نامشخص")],
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
    if (event?.source === "hook" && !state.appSettings.hook_updates) {
      return;
    }

    const shouldStick = isNearBottom(els.chat);
    loadSessionList({ loadInitial: false }).then(() => {
      if (event.session_key && event.session_key === state.currentSessionKey) {
        hideSessionNotice();
        refreshCurrentSessionContent({ stickBottom: shouldStick }).catch((error) => {
          console.error(error);
        });
        return;
      }
      if (event?.source === "hook") {
        handleHookSessionNotice(event);
        return;
      }
      showSessionNotice(event, { autoOpen: false });
    });
  }

  function handleHookSessionNotice(event) {
    const mode = state.appSettings.hook_follow_mode;
    if (mode === "off") {
      return;
    }
    const holdNavigation = state.appSettings.ignore_hook_navigation_while_typing && isUserEditing();
    showSessionNotice(event, { autoOpen: mode === "auto" && !holdNavigation });
  }

  function isUserEditing() {
    const active = document.activeElement;
    if (state.editingSessionTitle) {
      return true;
    }
    if (!active) {
      return false;
    }
    const tagName = active.tagName?.toLowerCase();
    return tagName === "input"
      || tagName === "textarea"
      || active.isContentEditable
      || Boolean(active.closest(".search-box"));
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

  function showSessionNotice(event, options = {}) {
    if (!event?.session_key) {
      return;
    }

    const notice = getSessionNotice();
    const title = normalizeInlineText(event.session_name || event.project_name || event.session_id || "نشست جدید");
    const durationSeconds = 8;
    let remainingSeconds = durationSeconds;
    const action = notice.querySelector(".session-notice-action");
    const autoOpen = options.autoOpen !== false;

    const goToSession = async () => {
      hideSessionNotice();
      await loadSession(event.session_key, { sessionHint: event });
      await loadSessionList({ loadInitial: false });
    };
    const updateCountdown = () => {
      action.textContent = autoOpen ? `رفتن به این چت (${remainingSeconds})` : "رفتن به این چت";
      action.style.setProperty("--notice-progress", autoOpen ? String(Math.max(0, remainingSeconds / durationSeconds)) : "0");
    };

    notice.querySelector(".session-notice-text").textContent = `نشست به‌روزرسانی شد: ${title}`;
    action.onclick = goToSession;
    updateCountdown();

    notice.classList.remove("hidden");
    window.requestAnimationFrame(() => notice.classList.add("is-visible"));

    window.clearTimeout(state.sessionNoticeTimer);
    window.clearInterval(state.sessionNoticeInterval);
    if (!autoOpen) {
      return;
    }
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

  function sessionDisplayName(session) {
    return normalizeInlineText(
      session?.session_name
      || session?.first_preview
      || session?.last_preview
      || session?.session_id
      || "نشست بدون نام",
    );
  }

  function buildSessionStub(source) {
    const key = normalizeInlineText(source?.key || source?.session_key || "");
    if (!key) {
      return null;
    }
    const sessionId = normalizeInlineText(source.session_id || key.split(":").slice(1).join(":"));
    const updatedAt = normalizeInlineText(source.updated_at || new Date().toISOString());
    return {
      key,
      session_id: sessionId,
      session_name: normalizeInlineText(source.session_name || ""),
      project_name: normalizeInlineText(source.project_name || ""),
      updated_at: updatedAt,
      first_preview: "",
      last_preview: "",
      message_count_estimate: 0,
      metadata_only: false,
      invalid_reason: "",
    };
  }

  function syncCurrentSessionHeader() {
    const hasSession = Boolean(state.currentSessionKey);
    const hasProject = hasSession && Boolean(state.currentProjectName);
    const hasTitle = hasSession && !state.editingSessionTitle;
    const projectCount = hasProject ? currentProjectSessionCount(state.currentProjectName) : 0;
    const hasProjectBadge = projectCount > 1;

    els.sessionTitle.classList.toggle("hidden", hasSession);
    els.sessionProject.classList.toggle("hidden", !hasProject);
    els.sessionProjectBadge.classList.toggle("hidden", !hasProjectBadge);
    els.sessionTitleButton.classList.toggle("hidden", !hasTitle);
    els.sessionTitleInput.classList.toggle("hidden", !state.editingSessionTitle);
    els.sessionTitleSeparator.classList.toggle("hidden", !hasProject || !hasSession);

    if (!hasSession) {
      els.sessionTitle.textContent = "نمایشگر نشست‌ها";
      els.sessionProjectLabel.textContent = "";
      els.sessionProjectBadge.textContent = "";
      els.sessionProject.title = "";
      els.sessionTitleButton.textContent = "";
      els.sessionTitleInput.value = "";
      return;
    }

    els.sessionProjectLabel.textContent = state.currentProjectName || "پروژه نامشخص";
    els.sessionProjectBadge.textContent = hasProjectBadge ? String(projectCount) : "";
    els.sessionProject.title = hasProject
      ? `${state.currentProjectName} • ${projectCount > 0 ? `${projectCount} نشست` : "نمایش نشست‌های پروژه"}`
      : "";
    els.sessionProject.setAttribute(
      "aria-label",
      hasProject
        ? `باز کردن نشست‌های پروژه ${state.currentProjectName}${projectCount > 0 ? `، ${projectCount} نشست` : ""}`
        : "نمایش نشست‌های پروژه",
    );
    els.sessionTitleButton.textContent = state.currentSessionName || "نشست بدون نام";
    els.sessionTitleButton.title = "برای تغییر نام کلیک کنید";
    if (!state.editingSessionTitle) {
      els.sessionTitleInput.value = state.currentSessionName || "";
    }
    if (hasProject) {
      ensureProjectSessionCount(state.currentProjectName);
    }
  }

  function beginSessionTitleEdit() {
    if (!state.currentSessionKey) {
      return;
    }
    state.editingSessionTitle = true;
    syncCurrentSessionHeader();
    els.sessionTitleInput.value = state.currentSessionName || "";
    els.sessionTitleInput.focus();
    els.sessionTitleInput.select();
  }

  function cancelSessionTitleEdit() {
    state.editingSessionTitle = false;
    syncCurrentSessionHeader();
  }

  function handleSessionTitleInputKeydown(event) {
    if (event.key === "Enter") {
      event.preventDefault();
      commitSessionTitleEdit();
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      cancelSessionTitleEdit();
    }
  }

  async function commitSessionTitleEdit() {
    if (!state.editingSessionTitle || !state.currentSessionKey) {
      return;
    }

    const nextTitle = normalizeInlineText(els.sessionTitleInput.value);
    const currentTitle = normalizeInlineText(state.currentSessionName);
    state.editingSessionTitle = false;
    syncCurrentSessionHeader();
    if (!nextTitle || nextTitle === currentTitle) {
      return;
    }

    try {
      const updated = await updateSession(state.currentSessionKey, { session_name: nextTitle });
      applySessionUpdate(updated);
    } catch (error) {
      console.error(error);
    }
  }

  function applySessionUpdate(updated) {
    if (!updated?.key) {
      return;
    }
    state.sessions = mergeSessionPages(state.sessions, [updated]);
    state.searchResults = mergeSessionPages(state.searchResults, [updated]);
    if (state.currentSessionKey === updated.key) {
      state.currentSessionName = sessionDisplayName(updated);
      state.currentProjectName = normalizeInlineText(updated.project_name || "");
      syncCurrentSessionHeader();
      syncAgentComposer();
      if (state.reader.open) {
        renderReader();
      }
    }
    if (state.projectSessions.open) {
      state.projectSessions.items = mergeSessionPages(state.projectSessions.items, [updated]);
      renderProjectSessions();
    }
    renderSessions(getVisibleSessions());
  }

  function openProjectSessionsModal() {
    const project = normalizeInlineText(state.currentProjectName);
    if (!project) {
      return;
    }
    state.projectSessions.open = true;
    state.projectSessions.project = project;
    state.projectSessions.items = [];
    state.projectSessions.nextOffset = 0;
    state.projectSessions.hasMore = false;
    state.projectSessions.total = 0;
    state.projectSessions.requestId += 1;
    els.projectSessionsModal.classList.remove("hidden");
    els.projectSessionsModal.setAttribute("aria-hidden", "false");
    renderProjectSessions();
    loadProjectSessionsPage({ reset: true });
  }

  function closeProjectSessionsModal() {
    state.projectSessions.open = false;
    state.projectSessions.loading = false;
    state.projectSessions.requestId += 1;
    els.projectSessionsModal.classList.add("hidden");
    els.projectSessionsModal.setAttribute("aria-hidden", "true");
  }

  async function loadProjectSessionsPage(options = {}) {
    const project = normalizeInlineText(state.projectSessions.project);
    if (!project || state.projectSessions.loading) {
      return;
    }
    const reset = options.reset !== false;
    if (!reset && !state.projectSessions.hasMore) {
      return;
    }

    const requestId = ++state.projectSessions.requestId;
    state.projectSessions.loading = true;
    renderProjectSessions();
    try {
      const data = await fetchSessions({
        project,
        limit: SESSION_PAGE_SIZE,
        offset: reset ? 0 : state.projectSessions.nextOffset,
      });
      if (requestId !== state.projectSessions.requestId || !state.projectSessions.open) {
        return;
      }
      const page = data.items || [];
      state.projectSessions.items = reset ? page : mergeSessionPages(state.projectSessions.items, page);
      state.projectSessions.nextOffset = Number(data.next_offset || 0);
      state.projectSessions.hasMore = state.projectSessions.nextOffset > 0;
      state.projectSessions.total = Number(data.total || state.projectSessions.items.length);
    } catch (error) {
      if (requestId === state.projectSessions.requestId) {
        console.error(error);
      }
    } finally {
      if (requestId === state.projectSessions.requestId) {
        state.projectSessions.loading = false;
        renderProjectSessions();
      }
    }
  }

  function maybeLoadMoreProjectSessions() {
    if (!state.projectSessions.open || state.projectSessions.loading || !state.projectSessions.hasMore) {
      return;
    }
    const distanceFromBottom = els.projectSessionsList.scrollHeight - els.projectSessionsList.scrollTop - els.projectSessionsList.clientHeight;
    if (distanceFromBottom < 220) {
      loadProjectSessionsPage({ reset: false });
    }
  }

  function renderProjectSessions() {
    const items = dedupeSessions(state.projectSessions.items);
    const project = state.projectSessions.project || "پروژه";
    els.projectSessionsMeta.textContent = state.projectSessions.loading && items.length === 0
      ? "در حال بارگذاری نشست‌ها..."
      : `${items.length}${state.projectSessions.hasMore ? "+" : ""} از ${Math.max(items.length, state.projectSessions.total || 0)} نشست`;
    els.projectSessionsList.replaceChildren();

    if (items.length === 0) {
      const empty = document.createElement("div");
      empty.className = "session-list-empty";
      empty.textContent = state.projectSessions.loading
        ? "در حال بارگذاری نشست‌های پروژه..."
        : `نشستی برای پروژه ${project} پیدا نشد.`;
      els.projectSessionsList.appendChild(empty);
      return;
    }

    items.forEach((session) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "project-session-item";
      item.classList.toggle("active", session.key === state.currentSessionKey);

      const title = document.createElement("span");
      title.className = "project-session-name";
      title.textContent = sessionDisplayName(session);

      const meta = document.createElement("span");
      meta.className = "project-session-meta";
      meta.textContent = `${formatTime(session.updated_at)} • ${agentLabel(session.agent)}`;

      item.append(title, meta);
      item.addEventListener("click", async () => {
        state.sessions = mergeSessionPages(state.sessions, [session]);
        closeProjectSessionsModal();
        els.sessionsRail.classList.remove("open");
        await loadSession(session.key);
      });
      els.projectSessionsList.appendChild(item);
    });

    if (state.projectSessions.loading || state.projectSessions.hasMore) {
      const footer = document.createElement("div");
      footer.className = "session-list-footer";
      footer.textContent = state.projectSessions.loading
        ? "در حال بارگذاری نشست‌های بیشتر..."
        : "برای نشست‌های بیشتر اسکرول کنید";
      els.projectSessionsList.appendChild(footer);
    }
  }

  function currentProjectSessionCount(projectName) {
    const project = normalizeInlineText(projectName);
    if (!project) {
      return 0;
    }
    const cached = state.projectSessionCounts[project];
    if (cached?.total > 0) {
      return cached.total;
    }
    return state.sessions.filter((session) => normalizeInlineText(session.project_name || "") === project).length;
  }

  async function ensureProjectSessionCount(projectName) {
    const project = normalizeInlineText(projectName);
    if (!project) {
      return;
    }
    const cached = state.projectSessionCounts[project];
    if (cached?.loading || Number.isFinite(cached?.total)) {
      return;
    }

    state.projectSessionCounts[project] = { total: cached?.total, loading: true };
    try {
      const data = await fetchSessions({ project, limit: 1, offset: 0 });
      state.projectSessionCounts[project] = {
        total: Number(data.total || 0),
        loading: false,
      };
    } catch (error) {
      console.error(error);
      state.projectSessionCounts[project] = { total: cached?.total, loading: false };
    } finally {
      if (project === state.currentProjectName) {
        syncCurrentSessionHeader();
      }
    }
  }

  function normalizeInlineText(value) {
    return String(value || "").replace(/\s+/g, " ").trim();
  }

  function firstLine(text) {
    const value = String(text || "").trim().split("\n")[0] || "خروجی ابزار";
    return value.length > 80 ? `${value.slice(0, 80)}...` : value;
  }
}
