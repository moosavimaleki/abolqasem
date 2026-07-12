import { afterEach, describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { RefreshCw } from "lucide-react"
import {
  ChangelogSection,
  fetchGithubReleases,
  formatPublishedDate,
  getCachedChangelog,
  isChangelogReleaseNewer,
  getKeybindingsSubtitle,
  loadChangelog,
  resetSettingsPageChangelogCache,
  providerModelCatalogResetPatch,
  resolveSettingsSectionId,
  resolveSettingsAppVersion,
  setCachedChangelog,
  shouldPreviewChatSoundChange,
  McpSection,
  SkillsSection,
} from "./SettingsPage"
import { SettingsHeaderButton } from "../components/ui/settings-header-button"
import type { UpdateSnapshot } from "../../shared/types"

const SAMPLE_RELEASES = [
  {
    id: 1,
    name: "v0.8.1",
    tag_name: "v0.8.1",
    html_url: "https://github.com/moosavimaleki/abolqasem/releases/tag/v0.8.1",
    published_at: "2026-03-19T16:53:08Z",
    body: "## Improvements\n- Better cursor color",
    prerelease: false,
    draft: false,
  },
  {
    id: 2,
    name: null,
    tag_name: "v0.9.0-beta.1",
    html_url: "https://github.com/moosavimaleki/abolqasem/releases/tag/v0.9.0-beta.1",
    published_at: "2026-03-20T12:00:00Z",
    body: "",
    prerelease: true,
    draft: false,
  },
]

afterEach(() => {
  resetSettingsPageChangelogCache()
})

function createUpdateSnapshot(overrides: Partial<UpdateSnapshot> = {}): UpdateSnapshot {
  return {
    currentVersion: "1.0.0",
    latestVersion: "1.1.0",
    status: "available",
    updateAvailable: true,
    lastCheckedAt: 123,
    error: null,
    installAction: "restart",
    reloadRequestedAt: null,
    ...overrides,
  }
}

describe("fetchGithubReleases", () => {
  test("filters draft releases and sends the GitHub accept header", async () => {
    let requestedUrl = ""
    let requestedAcceptHeader = ""

    const releases = await fetchGithubReleases(async (input, init) => {
      requestedUrl = String(input)
      requestedAcceptHeader = String(new Headers(init?.headers).get("Accept"))

      return new Response(JSON.stringify([
        SAMPLE_RELEASES[0],
        { ...SAMPLE_RELEASES[1], draft: true },
      ]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })

    expect(requestedUrl).toBe("https://api.github.com/repos/moosavimaleki/abolqasem/releases")
    expect(requestedAcceptHeader).toBe("application/vnd.github+json")
    expect(releases).toEqual([SAMPLE_RELEASES[0]])
  })

  test("throws on non-200 responses", async () => {
    await expect(fetchGithubReleases(async () => new Response("nope", { status: 403 }))).rejects.toThrow(
      "GitHub releases request failed with status 403"
    )
  })
})

describe("changelog cache", () => {
  test("reuses cached releases inside the ttl window", () => {
    const originalNow = Date.now
    Date.now = () => 1_000

    setCachedChangelog([SAMPLE_RELEASES[0]])
    expect(getCachedChangelog()).toEqual([SAMPLE_RELEASES[0]])

    Date.now = () => 1_000 + 4 * 60 * 1000
    expect(getCachedChangelog()).toEqual([SAMPLE_RELEASES[0]])

    Date.now = originalNow
  })

  test("expires cached releases after the ttl window", () => {
    const originalNow = Date.now
    Date.now = () => 2_000

    setCachedChangelog([SAMPLE_RELEASES[0]])
    Date.now = () => 2_000 + 5 * 60 * 1000 + 1

    expect(getCachedChangelog()).toBeNull()

    Date.now = originalNow
  })

  test("force refresh bypasses the in-memory cache", async () => {
    setCachedChangelog([SAMPLE_RELEASES[0]])

    const releases = await loadChangelog({
      force: true,
      fetchImpl: async () => new Response(JSON.stringify([SAMPLE_RELEASES[1]]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    })

    expect(releases).toEqual([SAMPLE_RELEASES[1]])
  })
})

describe("isChangelogReleaseNewer", () => {
  test("allows dev-local builds to install the latest stable release", () => {
    expect(isChangelogReleaseNewer("1.0.4", "dev-local")).toBe(true)
  })

  test("compares release and prerelease versions", () => {
    expect(isChangelogReleaseNewer("1.0.4", "1.0.3")).toBe(true)
    expect(isChangelogReleaseNewer("1.0.3", "1.0.4")).toBe(false)
    expect(isChangelogReleaseNewer("1.0.4", "1.0.4-beta.1")).toBe(true)
  })
})

describe("resolveSettingsSectionId", () => {
  test("accepts known settings sections", () => {
    expect(resolveSettingsSectionId("general")).toBe("general")
    expect(resolveSettingsSectionId("providers")).toBe("providers")
    expect(resolveSettingsSectionId("changelog")).toBe("changelog")
    expect(resolveSettingsSectionId("keybindings")).toBe("keybindings")
    expect(resolveSettingsSectionId("skills")).toBe("skills")
  })

  test("rejects unknown settings sections", () => {
    expect(resolveSettingsSectionId("page-1")).toBeNull()
    expect(resolveSettingsSectionId("page-2")).toBeNull()
    expect(resolveSettingsSectionId("page-3")).toBeNull()
    expect(resolveSettingsSectionId("nope")).toBeNull()
    expect(resolveSettingsSectionId(undefined)).toBeNull()
  })
})

describe("providerModelCatalogResetPatch", () => {
  test("clears the saved override so runtime-discovered models become visible", () => {
    expect(providerModelCatalogResetPatch("codex")).toEqual({
      providerModelCatalog: {
        codex: { catalogModels: [], customModels: [] },
      },
    })
  })
})

describe("resolveSettingsAppVersion", () => {
  test("uses the backend app version instead of the web package version", () => {
    const appSettings = {
      management: {
        update: createUpdateSnapshot({ currentVersion: "2.4.6" }),
      },
    } as never

    expect(resolveSettingsAppVersion(null, appSettings)).toBe("2.4.6")
    expect(resolveSettingsAppVersion(createUpdateSnapshot({ currentVersion: "2.4.7" }), appSettings)).toBe("2.4.7")
    expect(resolveSettingsAppVersion(null, appSettings)).not.toBe("0.1.0")
  })

  test("falls back to unknown when the backend version is unavailable", () => {
    expect(resolveSettingsAppVersion(null, null)).toBe("unknown")
  })
})

describe("SkillsSection", () => {
  test("renders installed and discover sections", () => {
    const html = renderToStaticMarkup(
      <SkillsSection
        state={{
          connectionStatus: "connected",
          socket: {
            command: async () => ({ skills: [] }),
          } as never,
        }}
      />
    )

    expect(html).toContain("نصب شده")
    expect(html).toContain("جست‌وجو")
    expect(html).toContain("جست‌وجوی مهارت‌ها")
  })
})

describe("McpSection", () => {
  test("renders configured servers and registry discover controls", () => {
    const html = renderToStaticMarkup(
      <McpSection
        state={{
          connectionStatus: "connected",
          socket: {
            command: async () => ({ configPaths: { codex: "", claude: "", gemini: "" }, servers: [] }),
          } as never,
        }}
      />
    )

    expect(html).toContain("سرورهای MCP تنظیم‌شده")
    expect(html).toContain("رجیستری")
    expect(html).toContain("جست‌وجوی MCP در رجیستری")
    expect(html).toContain("جست‌وجوی رجیستری MCP")
  })
})

describe("getKeybindingsSubtitle", () => {
  test("renders the active keybindings path", () => {
    expect(getKeybindingsSubtitle("~/.abolqasem-dev/keybindings.json")).toBe(
      "Edit global app shortcuts stored in ~/.abolqasem-dev/keybindings.json."
    )
  })
})

describe("shouldPreviewChatSoundChange", () => {
  test("previews only when the selected value actually changes", () => {
    expect(shouldPreviewChatSoundChange("always", "always")).toBe(false)
    expect(shouldPreviewChatSoundChange("always", "never")).toBe(true)
    expect(shouldPreviewChatSoundChange("never", "unfocused")).toBe(true)
    expect(shouldPreviewChatSoundChange("funk", "glass")).toBe(true)
  })
})

describe("SettingsHeaderButton", () => {
  test("renders shared header button content and icon", () => {
    const html = renderToStaticMarkup(
      <SettingsHeaderButton icon={<RefreshCw className="size-3.5" />}>
        Check for updates
      </SettingsHeaderButton>
    )

    expect(html).toContain("Check for updates")
    expect(html).toContain("lucide-refresh-cw")
    expect(html).toContain("gap-1.5")
  })

  test("supports the default variant for the update action", () => {
    const html = renderToStaticMarkup(
      <SettingsHeaderButton variant="default" >
        Update
      </SettingsHeaderButton>
    )

    expect(html).toContain("Update")
    expect(html).toContain("bg-primary")
  })
})

describe("ChangelogSection", () => {
  test("renders version highlights, release cards, markdown, links, and prerelease badges", () => {
    const html = renderToStaticMarkup(
      <ChangelogSection
        status="success"
        releases={SAMPLE_RELEASES}
        error={null}
        onRetry={() => {}}
        updateSnapshot={createUpdateSnapshot({ latestVersion: "0.8.1", currentVersion: "0.8.1" })}
        currentVersion="1.0.0"
        onInstallUpdate={() => {}}
        onCheckForUpdates={() => {}}
      />
    )

    expect(html).not.toContain("شما در حال اجرای این نسخه از Abolqasem هستید.")
    expect(html).toContain("نسخه فعلی")
    expect(html).toContain("به‌روزرسانی")
    expect(html).toContain("v0.8.1")
    expect(html).toContain("Better cursor color")
    expect(html).toContain('aria-label="مشاهده نسخه در GitHub"')
    expect(html).toContain("https://github.com/moosavimaleki/abolqasem/releases/tag/v0.8.1")
    expect(html).toContain("پیش‌انتشار")
    expect(html).toContain("یادداشت انتشاری ارائه نشده است.")
    expect(html).toContain(formatPublishedDate("2026-03-19T16:53:08Z"))
    expect(html).not.toContain("مشاهده در GitHub")
  })

  test("renders an error state with retry action", () => {
    const html = renderToStaticMarkup(
      <ChangelogSection
        status="error"
        releases={[]}
        error="GitHub said no"
        onRetry={() => {}}
        updateSnapshot={createUpdateSnapshot({ updateAvailable: false, status: "error", error: "GitHub said no" })}
        currentVersion="1.0.0"
        onInstallUpdate={() => {}}
        onCheckForUpdates={() => {}}
      />
    )

    expect(html).toContain("بارگذاری تغییرات نسخه‌ها ممکن نبود")
    expect(html).toContain("GitHub said no")
    expect(html).toContain("تلاش دوباره")
  })

  test("renders check-for-updates when no update is available", () => {
    const html = renderToStaticMarkup(
      <ChangelogSection
        status="success"
        releases={SAMPLE_RELEASES}
        error={null}
        onRetry={() => {}}
        updateSnapshot={createUpdateSnapshot({
          latestVersion: "1.0.0",
          status: "up_to_date",
          updateAvailable: false,
        })}
        currentVersion="1.0.0"
        onInstallUpdate={() => {}}
        onCheckForUpdates={() => {}}
      />
    )

    expect(html).toContain("بررسی به‌روزرسانی")
    expect(html).not.toContain(">به‌روزرسانی<")
  })

  test("renders update action for dev-local from changelog releases when server check errored", () => {
    const html = renderToStaticMarkup(
      <ChangelogSection
        status="success"
        releases={SAMPLE_RELEASES}
        error={null}
        onRetry={() => {}}
        updateSnapshot={createUpdateSnapshot({
          currentVersion: "dev-local",
          latestVersion: null,
          status: "error",
          updateAvailable: false,
        })}
        currentVersion="dev-local"
        onInstallUpdate={() => {}}
        onCheckForUpdates={() => {}}
      />
    )

    expect(html).toContain("به‌روزرسانی")
    expect(html).not.toContain("بررسی به‌روزرسانی")
  })

  test("disables the update action while updating", () => {
    const html = renderToStaticMarkup(
      <ChangelogSection
        status="success"
        releases={SAMPLE_RELEASES}
        error={null}
        onRetry={() => {}}
        updateSnapshot={createUpdateSnapshot({
          latestVersion: "0.8.1",
          status: "restart_pending",
        })}
        currentVersion="1.0.0"
        onInstallUpdate={() => {}}
        onCheckForUpdates={() => {}}
      />
    )

    expect(html).toContain("disabled")
    expect(html).toContain("در حال به‌روزرسانی")
  })
})
