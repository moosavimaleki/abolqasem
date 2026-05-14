import { buildReaderTOC, renderMessageContent } from "./content-renderer.js";

export function renderReaderDocument(options) {
  const {
    contentRoot,
    tocRoot,
    scrollRoot,
    message,
    title = "حالت خواندن",
    search = "",
  } = options;

  contentRoot.replaceChildren();
  const article = document.createElement("article");
  article.className = "reader-article";

  const hasPrimaryHeading = /<h1\b/i.test(message.html || "");
  if (!hasPrimaryHeading) {
    const heading = document.createElement("h1");
    heading.textContent = title;
    article.appendChild(heading);
  }

  article.appendChild(renderMessageContent(message, "reader", search));
  contentRoot.appendChild(article);
  buildReaderTOC(contentRoot, tocRoot);
  if (scrollRoot) {
    scrollRoot.scrollTop = 0;
  }
  return article;
}
