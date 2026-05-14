const STORAGE_KEY = "ai-agent-manager.reader-settings.v3";

const DEFAULT_SETTINGS = {
  font: "vazirmatn",
  fontSize: 18,
  lineHeight: 1.9,
  theme: "night",
};

export function loadSettings() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return { ...DEFAULT_SETTINGS };
    }
    return { ...DEFAULT_SETTINGS, ...JSON.parse(raw) };
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(settings) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
}

export function applySettings(settings) {
  document.body.dataset.theme = settings.theme;
  document.body.dataset.readerFont = settings.font;
  document.documentElement.style.setProperty("--reader-font-family", fontStack(settings.font));
  document.documentElement.style.setProperty("--content-font-size", `${settings.fontSize}px`);
  document.documentElement.style.setProperty("--reader-line-height", String(settings.lineHeight));
}

export function syncSettingsUI(settings, root) {
  root.querySelectorAll("[data-setting]").forEach((group) => {
    const key = group.dataset.setting;
    group.querySelectorAll("[data-value]").forEach((button) => {
      button.classList.toggle("active", String(button.dataset.value) === String(settings[key]));
    });
  });

  const fontSizeValue = root.querySelector("#font-size-value");
  if (fontSizeValue) {
    const displayedSize = settings.font === "bzar" ? settings.fontSize + 2 : settings.fontSize;
    fontSizeValue.textContent = String(displayedSize);
  }
}

export function clampFontSize(value) {
  return Math.max(14, Math.min(26, value));
}

function fontStack(font) {
  switch (font) {
    case "bzar":
      return '"BZarLocal", "VazirLocal", Tahoma, sans-serif';
    case "iranian-sans":
      return '"IranianSansLocal", "VazirLocal", Tahoma, sans-serif';
    default:
      return '"VazirLocal", Tahoma, sans-serif';
  }
}
