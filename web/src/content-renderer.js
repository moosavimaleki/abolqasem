import { copyText, escapeHTML, escapeRegExp } from "./utils.js";

export function renderMessageContent(message, mode = "chat", search = "") {
  const body = document.createElement("div");
  body.className = `content-body ${contentDirection(message)}`;
  body.dataset.mode = mode;
  body.innerHTML = message.html || escapeHTML(message.text || "");

  enhanceContent(body);
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

    highlightCode(pre);

    const frame = document.createElement("div");
    frame.className = "code-frame";

    const action = document.createElement("button");
    action.type = "button";
    action.className = "inline-icon";
    action.dataset.icon = "content_copy";
    action.setAttribute("aria-label", "کپی کد");
    action.addEventListener("click", (event) => {
      event.stopPropagation();
      copyText(pre.innerText);
    });

    pre.parentNode.insertBefore(frame, pre);
    frame.append(pre, action);
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
