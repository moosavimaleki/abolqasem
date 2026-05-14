import { copyText, escapeHTML, escapeRegExp } from "./utils.js";

let mermaidReady = false;
let mermaidSequence = 0;
const localFileURLPattern = /\bhttps?:\/\/(?:127\.0\.0\.1|localhost|\[::1\])(?::\d+)?\/[^\s<>"`]+(?::\d+(?::\d+)?)?/gi;

export function renderMessageContent(message, mode = "chat", search = "") {
  const body = document.createElement("div");
  body.className = `content-body ${contentDirection(message)}`;
  body.dataset.mode = mode;
  body.innerHTML = message.html || escapeHTML(message.text || "");

  linkifyLocalFileReferences(body);
  enhanceContent(body);
  wrapPersianRuns(body);
  if (search) {
    highlightText(body, search);
  }
  return body;
}

function contentDirection(message) {
  if (message.role === "tool") {
    return "ltr";
  }
  return "rtl";
}

export function buildReaderTOC(contentRoot, tocRoot) {
  tocRoot.replaceChildren();
  const headings = [...contentRoot.querySelectorAll("h1, h2, h3")];

  if (headings.length === 0) {
    const empty = document.createElement("span");
    empty.className = "toc-empty";
    empty.textContent = "بدون سرفصل";
    tocRoot.appendChild(empty);
    return;
  }

  headings.forEach((heading, index) => {
    if (!heading.id) {
      heading.id = `reader-heading-${index}`;
    }

    const button = document.createElement("button");
    button.type = "button";
    button.className = `toc-link toc-${heading.tagName.toLowerCase()}`;
    button.textContent = heading.textContent || `بخش ${index + 1}`;
    button.addEventListener("click", () => {
      heading.scrollIntoView({ behavior: "smooth", block: "start" });
    });
    tocRoot.appendChild(button);
  });
}

export function highlightText(root, query) {
  const value = query.trim();
  if (!value) {
    return;
  }

  const regex = new RegExp(escapeRegExp(value), "gi");
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.nodeValue || !node.nodeValue.trim()) {
        return NodeFilter.FILTER_REJECT;
      }
      if (node.parentElement && ["SCRIPT", "STYLE", "MARK"].includes(node.parentElement.tagName)) {
        return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  const nodes = [];

  while (walker.nextNode()) {
    nodes.push(walker.currentNode);
  }

  nodes.forEach((node) => {
    const text = node.nodeValue || "";
    regex.lastIndex = 0;
    if (!regex.test(text)) {
      return;
    }

    const fragment = document.createDocumentFragment();
    let lastIndex = 0;
    regex.lastIndex = 0;
    let match;

    while ((match = regex.exec(text))) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex, match.index)));
      const mark = document.createElement("mark");
      mark.textContent = match[0];
      fragment.appendChild(mark);
      lastIndex = match.index + match[0].length;
    }

    fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    node.parentNode.replaceChild(fragment, node);
  });
}

function linkifyLocalFileReferences(root) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.nodeValue || !localFileURLPattern.test(node.nodeValue)) {
        localFileURLPattern.lastIndex = 0;
        return NodeFilter.FILTER_REJECT;
      }
      localFileURLPattern.lastIndex = 0;
      if (node.parentElement?.closest("a, pre, code, kbd, samp, script, style")) {
        return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  const nodes = [];

  while (walker.nextNode()) {
    nodes.push(walker.currentNode);
  }

  nodes.forEach((node) => {
    const text = node.nodeValue || "";
    const fragment = document.createDocumentFragment();
    let lastIndex = 0;
    localFileURLPattern.lastIndex = 0;
    let match;

    while ((match = localFileURLPattern.exec(text))) {
      const rawURL = match[0];
      const trimmedURL = rawURL.replace(/[)\].,;!?]+$/g, "");
      const trailing = rawURL.slice(trimmedURL.length);
      fragment.appendChild(document.createTextNode(text.slice(lastIndex, match.index)));

      const link = document.createElement("a");
      link.href = trimmedURL;
      link.textContent = trimmedURL;
      fragment.appendChild(link);
      if (trailing) {
        fragment.appendChild(document.createTextNode(trailing));
      }
      lastIndex = match.index + rawURL.length;
    }

    fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    node.parentNode.replaceChild(fragment, node);
  });
}

function enhanceContent(root) {
  root.querySelectorAll("a").forEach((link) => {
    link.target = "_blank";
    link.rel = "noreferrer noopener";
  });

  root.querySelectorAll("table").forEach((table) => {
    if (table.parentElement?.classList.contains("table-wrap")) {
      return;
    }
    const wrap = document.createElement("div");
    wrap.className = "table-wrap";
    table.parentNode.insertBefore(wrap, table);
    wrap.appendChild(table);
  });

  root.querySelectorAll("pre").forEach((pre) => {
    if (pre.parentElement?.classList.contains("code-frame")) {
      return;
    }

    const code = pre.querySelector("code");
    if (isMermaidBlock(code)) {
      renderMermaidBlock(pre, code);
      return;
    }

    highlightCode(pre);

    const frame = document.createElement("div");
    frame.className = "code-frame";

    const header = document.createElement("div");
    header.className = "code-toolbar";

    const label = document.createElement("span");
    label.className = "code-label";
    label.textContent = detectCodeLanguage(code);

    const action = document.createElement("button");
    action.type = "button";
    action.className = "inline-icon";
    action.dataset.icon = "content_copy";
    action.setAttribute("aria-label", "کپی کد");
    action.addEventListener("click", (event) => {
      event.stopPropagation();
      copyText(pre.innerText);
    });

    const body = document.createElement("div");
    body.className = "code-canvas";

    header.append(label, action);
    pre.parentNode.insertBefore(frame, pre);
    body.appendChild(pre);
    frame.append(header, body);
  });
}

function isMermaidBlock(code) {
  if (!code) {
    return false;
  }
  return /(^|\s)language-mermaid(\s|$)/.test(code.className);
}

function detectCodeLanguage(code) {
  const className = code?.className || "";
  const match = className.match(/(?:^|\s)language-([a-z0-9_+-]+)(?:\s|$)/i);
  const lang = (match?.[1] || "").toLowerCase();

  switch (lang) {
    case "js":
      return "JavaScript";
    case "ts":
      return "TypeScript";
    case "jsx":
      return "JSX";
    case "tsx":
      return "TSX";
    case "sh":
    case "shell":
    case "bash":
    case "zsh":
      return "Shell";
    case "ps1":
    case "powershell":
      return "PowerShell";
    case "yml":
      return "YAML";
    case "md":
      return "Markdown";
    case "":
      return "Code";
    default:
      return lang.toUpperCase();
  }
}

function renderMermaidBlock(pre, code) {
  const source = code.textContent?.trim() || "";
  if (!source) {
    return;
  }

  const frame = document.createElement("div");
  frame.className = "mermaid-frame";

  const header = document.createElement("div");
  header.className = "mermaid-toolbar";

  const label = document.createElement("span");
  label.className = "mermaid-label";
  label.textContent = "Mermaid";

  let viewer = null;
  const actions = document.createElement("div");
  actions.className = "mermaid-actions";
  const copyAction = mermaidAction("content_copy", "کپی نمودار Mermaid", (event) => {
    event.stopPropagation();
    copyText(source);
  });
  const zoomOutAction = mermaidAction("zoom_out", "کوچک‌تر کردن نمودار", () => viewer?.zoomBy(0.82));
  const resetAction = mermaidAction("center_focus_strong", "بازنشانی نمای نمودار", () => viewer?.reset());
  const zoomInAction = mermaidAction("zoom_in", "بزرگ‌تر کردن نمودار", () => viewer?.zoomBy(1.18));
  const fullscreenAction = mermaidAction("fullscreen", "تمام‌صفحه کردن نمودار", () => viewer?.toggleFullscreen());

  actions.append(copyAction, zoomOutAction, resetAction, zoomInAction, fullscreenAction);
  header.append(label, actions);

  const canvas = document.createElement("div");
  canvas.className = "mermaid-canvas";
  canvas.textContent = "در حال رندر نمودار...";

  frame.append(header, canvas);
  pre.parentNode.insertBefore(frame, pre);
  pre.remove();

  ensureMermaidConfigured();
  if (!window.mermaid?.render) {
    showMermaidFallback(canvas, pre, "کتابخانه Mermaid بارگذاری نشد.");
    return;
  }

  const renderId = `mermaid-diagram-${mermaidSequence++}`;
  window.mermaid
    .render(renderId, source)
    .then(({ svg, bindFunctions }) => {
      canvas.replaceChildren();
      const viewport = document.createElement("div");
      viewport.className = "mermaid-viewport";
      viewport.innerHTML = svg;
      canvas.appendChild(viewport);
      canvas.classList.add("is-ready", "is-interactive");
      bindFunctions?.(canvas);
      const renderedSVG = viewport.querySelector("svg");
      if (renderedSVG) {
        viewer = setupMermaidViewer(frame, canvas, renderedSVG, fullscreenAction);
      }
    })
    .catch((error) => {
      console.warn("Mermaid rendering failed", error);
      showMermaidFallback(canvas, pre, "رندر Mermaid ناموفق بود.");
    });
}

function mermaidAction(icon, label, handler) {
  const action = document.createElement("button");
  action.type = "button";
  action.className = "inline-icon";
  action.dataset.icon = icon;
  action.setAttribute("aria-label", label);
  action.title = label;
  action.addEventListener("click", (event) => {
    event.stopPropagation();
    handler(event);
  });
  return action;
}

function setupMermaidViewer(frame, canvas, svg, fullscreenAction) {
  const state = {
    scale: 1,
    x: 0,
    y: 0,
    dragging: false,
    dragX: 0,
    dragY: 0,
    originX: 0,
    originY: 0,
  };
  svg.classList.add("mermaid-svg");

  const apply = () => {
    svg.style.setProperty("--mermaid-zoom", String(state.scale));
    svg.style.setProperty("--mermaid-pan-x", `${state.x}px`);
    svg.style.setProperty("--mermaid-pan-y", `${state.y}px`);
  };
  const zoomAt = (factor, x, y) => {
    const previous = state.scale;
    const next = clamp(previous * factor, 0.25, 5);
    if (next === previous) {
      return;
    }
    const ratio = next / previous;
    state.x = x - (x - state.x) * ratio;
    state.y = y - (y - state.y) * ratio;
    state.scale = next;
    apply();
  };
  const syncFullscreenIcon = () => {
    const active = document.fullscreenElement === frame || frame.classList.contains("is-expanded");
    fullscreenAction.dataset.icon = active ? "fullscreen_exit" : "fullscreen";
    fullscreenAction.setAttribute("aria-label", active ? "خروج از تمام‌صفحه" : "تمام‌صفحه کردن نمودار");
  };

  canvas.addEventListener("wheel", (event) => {
    event.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const factor = Math.exp(-event.deltaY * 0.001);
    zoomAt(factor, event.clientX - rect.left, event.clientY - rect.top);
  }, { passive: false });

  canvas.addEventListener("pointerdown", (event) => {
    if (event.button !== 0) {
      return;
    }
    state.dragging = true;
    state.dragX = event.clientX;
    state.dragY = event.clientY;
    state.originX = state.x;
    state.originY = state.y;
    canvas.classList.add("is-panning");
    canvas.setPointerCapture?.(event.pointerId);
  });

  canvas.addEventListener("pointermove", (event) => {
    if (!state.dragging) {
      return;
    }
    state.x = state.originX + event.clientX - state.dragX;
    state.y = state.originY + event.clientY - state.dragY;
    apply();
  });

  const stopPan = (event) => {
    if (!state.dragging) {
      return;
    }
    state.dragging = false;
    canvas.classList.remove("is-panning");
    canvas.releasePointerCapture?.(event.pointerId);
  };
  canvas.addEventListener("pointerup", stopPan);
  canvas.addEventListener("pointercancel", stopPan);
  document.addEventListener("fullscreenchange", syncFullscreenIcon);

  apply();
  syncFullscreenIcon();
  return {
    zoomBy(factor) {
      const rect = canvas.getBoundingClientRect();
      zoomAt(factor, rect.width / 2, rect.height / 2);
    },
    reset() {
      state.scale = 1;
      state.x = 0;
      state.y = 0;
      apply();
    },
    toggleFullscreen() {
      if (document.fullscreenElement === frame) {
        document.exitFullscreen?.();
        return;
      }
      if (frame.classList.contains("is-expanded")) {
        frame.classList.remove("is-expanded");
        syncFullscreenIcon();
        return;
      }
      if (frame.requestFullscreen) {
        frame.requestFullscreen().catch(() => {
          frame.classList.add("is-expanded");
          syncFullscreenIcon();
        });
        return;
      }
      frame.classList.add("is-expanded");
      syncFullscreenIcon();
    },
  };
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function ensureMermaidConfigured() {
  if (mermaidReady || !window.mermaid?.initialize) {
    return;
  }
  mermaidReady = true;

  const styles = getComputedStyle(document.body);
  window.mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: "base",
    fontFamily: styles.getPropertyValue("--font-family").trim() || "sans-serif",
    themeVariables: {
      primaryColor: colorOr(styles, "--surface-soft", "#f3f4f6"),
      primaryTextColor: colorOr(styles, "--text", "#111827"),
      primaryBorderColor: colorOr(styles, "--line-strong", "#cbd5e1"),
      lineColor: colorOr(styles, "--accent", "#2563eb"),
      secondaryColor: colorOr(styles, "--code-bg", "#e5e7eb"),
      tertiaryColor: colorOr(styles, "--surface", "#ffffff"),
      background: colorOr(styles, "--surface", "#ffffff"),
    },
  });
}

function colorOr(styles, variableName, fallback) {
  return styles.getPropertyValue(variableName).trim() || fallback;
}

function showMermaidFallback(canvas, pre, message) {
  canvas.replaceChildren();
  canvas.classList.add("has-error");

  const note = document.createElement("p");
  note.className = "mermaid-error";
  note.textContent = message;

  pre.classList.add("mermaid-source");
  canvas.append(note, pre);
}

function wrapPersianRuns(root) {
  const persianPattern = /([\u0600-\u06FF\u0750-\u077F\u08A0-\u08FF\uFB50-\uFDFF\uFE70-\uFEFF\u200C-\u200F]+)/g;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.nodeValue || !persianPattern.test(node.nodeValue)) {
        persianPattern.lastIndex = 0;
        return NodeFilter.FILTER_REJECT;
      }
      persianPattern.lastIndex = 0;
      if (node.parentElement?.closest("pre, code, kbd, samp, script, style, .persian-run")) {
        return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  const nodes = [];

  while (walker.nextNode()) {
    nodes.push(walker.currentNode);
  }

  nodes.forEach((node) => {
    const text = node.nodeValue || "";
    const fragment = document.createDocumentFragment();
    let lastIndex = 0;
    persianPattern.lastIndex = 0;
    let match;

    while ((match = persianPattern.exec(text))) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex, match.index)));
      const span = document.createElement("span");
      span.className = "persian-run";
      span.textContent = match[0];
      fragment.appendChild(span);
      lastIndex = match.index + match[0].length;
    }

    fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
    node.parentNode.replaceChild(fragment, node);
  });
}

function highlightCode(pre) {
  const target = pre.querySelector("code") || pre;
  if (!target.textContent.trim() || !window.hljs) {
    return;
  }

  try {
    window.hljs.highlightElement(target);
  } catch (error) {
    console.warn("Code highlighting failed", error);
  }
}
