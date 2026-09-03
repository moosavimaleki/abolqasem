import { useEffect, useRef, useState } from "react";
import { DownloadCloud, Loader2 } from "lucide-react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type {
  AppSettingsSnapshot,
  UpdateSnapshot,
} from "../../../shared/types";
import { markdownComponents } from "../messages/shared";
import { buttonVariants } from "../ui/button";
import { SettingsHeaderButton } from "../ui/settings-header-button";
import { cn } from "../../lib/utils";
import { useI18n } from "../../i18n/context";

const GITHUB_RELEASES_URL =
  "https://api.github.com/repos/moosavimaleki/abolqasem/releases";
const CHANGELOG_CACHE_TTL_MS = 5 * 60 * 1000;

export type GithubRelease = {
  id: number;
  name: string | null;
  tag_name: string;
  html_url: string;
  published_at: string | null;
  body: string | null;
  prerelease: boolean;
  draft: boolean;
};

export type ChangelogStatus = "idle" | "loading" | "success" | "error";

type ChangelogCache = { expiresAt: number; releases: GithubRelease[] };
type FetchReleases = (input: string, init?: RequestInit) => Promise<Response>;

let changelogCache: ChangelogCache | null = null;

export function resetSettingsPageChangelogCache() {
  changelogCache = null;
}

export async function fetchGithubReleases(
  fetchImpl: FetchReleases = fetch,
): Promise<GithubRelease[]> {
  const response = await fetchImpl(GITHUB_RELEASES_URL, {
    headers: { Accept: "application/vnd.github+json" },
  });
  if (!response.ok) {
    throw new Error(
      `GitHub releases request failed with status ${response.status}`,
    );
  }
  const payload = (await response.json()) as GithubRelease[];
  return payload.filter((release) => !release.draft);
}

export function getCachedChangelog() {
  if (!changelogCache) return null;
  if (Date.now() >= changelogCache.expiresAt) {
    changelogCache = null;
    return null;
  }
  return changelogCache.releases;
}

export function setCachedChangelog(releases: GithubRelease[]) {
  changelogCache = { releases, expiresAt: Date.now() + CHANGELOG_CACHE_TTL_MS };
}

function normalizeVersionForComparison(version: string | null | undefined) {
  return (version ?? "").trim().replace(/^v/i, "");
}

function isDevelopmentVersion(version: string) {
  const normalized = version.trim().toLowerCase();
  return normalized === "dev" || normalized.startsWith("dev-");
}

function splitVersionPrerelease(version: string): [string, string] {
  const [base, prerelease = ""] = version.trim().split("-", 2);
  return [base.trim(), prerelease.trim()];
}

function versionPart(part: string | undefined) {
  const trimmed = (part ?? "").trim();
  return /^\d+$/.test(trimmed) ? Number.parseInt(trimmed, 10) : 0;
}

function compareDottedVersion(left: string, right: string) {
  const leftParts = left.split(".");
  const rightParts = right.split(".");
  for (
    let index = 0;
    index < Math.max(leftParts.length, rightParts.length);
    index += 1
  ) {
    const leftValue = versionPart(leftParts[index]);
    const rightValue = versionPart(rightParts[index]);
    if (leftValue > rightValue) return 1;
    if (leftValue < rightValue) return -1;
  }
  return 0;
}

export function isChangelogReleaseNewer(
  latest: string | null | undefined,
  current: string | null | undefined,
) {
  const latestVersion = normalizeVersionForComparison(latest);
  const currentVersion = normalizeVersionForComparison(current);
  if (!latestVersion || !currentVersion || latestVersion === currentVersion)
    return false;
  if (isDevelopmentVersion(currentVersion)) return true;
  const [latestBase, latestPrerelease] = splitVersionPrerelease(latestVersion);
  const [currentBase, currentPrerelease] =
    splitVersionPrerelease(currentVersion);
  const comparison = compareDottedVersion(latestBase, currentBase);
  if (comparison !== 0) return comparison > 0;
  return currentPrerelease.length > 0 && latestPrerelease.length === 0;
}

export function resolveSettingsAppVersion(
  updateSnapshot: UpdateSnapshot | null | undefined,
  appSettings: Pick<AppSettingsSnapshot, "management"> | null | undefined,
) {
  return (
    updateSnapshot?.currentVersion?.trim() ||
    appSettings?.management?.update?.currentVersion?.trim() ||
    "unknown"
  );
}

export async function loadChangelog(options?: {
  force?: boolean;
  fetchImpl?: FetchReleases;
}) {
  const cached = options?.force ? null : getCachedChangelog();
  if (cached) return cached;
  const releases = await fetchGithubReleases(options?.fetchImpl);
  setCachedChangelog(releases);
  return releases;
}

export function formatPublishedDate(value: string | null) {
  if (!value) return "Unpublished";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "Unknown date";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(parsed);
}

export function ChangelogSection({
  status,
  releases,
  error,
  onRetry,
  updateSnapshot,
  currentVersion,
  onInstallUpdate,
  onCheckForUpdates,
}: {
  status: ChangelogStatus;
  releases: GithubRelease[];
  error: string | null;
  onRetry: () => void;
  updateSnapshot: UpdateSnapshot | null;
  currentVersion: string;
  onInstallUpdate: () => void | Promise<void>;
  onCheckForUpdates: () => void | Promise<void>;
}) {
  const { t } = useI18n();
  const [checkPending, setCheckPending] = useState(false);
  const [installPending, setInstallPending] = useState(false);
  const mountedRef = useRef(true);
  useEffect(
    () => () => {
      mountedRef.current = false;
    },
    [],
  );

  const latestStableRelease = releases.find((release) => !release.prerelease);
  const latestVersion =
    updateSnapshot?.latestVersion ??
    latestStableRelease?.tag_name ??
    releases[0]?.tag_name ??
    t.common.unknown;
  const currentVersionLabel = updateSnapshot?.currentVersion ?? currentVersion;
  const isChecking = checkPending || updateSnapshot?.status === "checking";
  const isUpdating =
    installPending ||
    updateSnapshot?.status === "updating" ||
    updateSnapshot?.status === "restart_pending";
  const canInstallUpdate =
    updateSnapshot?.updateAvailable === true ||
    (status === "success" &&
      isChangelogReleaseNewer(
        latestStableRelease?.tag_name,
        currentVersionLabel,
      ));
  const normalizedLatestVersion = latestVersion.replace(/^v/i, "");
  const normalizedCurrentVersion = currentVersionLabel.replace(/^v/i, "");

  const runWithPending = (
    action: () => void | Promise<void>,
    setPending: (pending: boolean) => void,
  ) => {
    setPending(true);
    void Promise.resolve()
      .then(action)
      .finally(() => {
        if (mountedRef.current) setPending(false);
      });
  };

  return (
    <div className="space-y-4">
      {status === "loading" || status === "idle" ? (
        <div className="flex min-h-[180px] items-center justify-center rounded-2xl border border-border bg-card/40 px-6 py-8 text-sm text-muted-foreground">
          <div className="flex items-center gap-3">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>{t.common.loading}…</span>
          </div>
        </div>
      ) : null}
      {status === "error" ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="text-sm font-medium text-foreground">
                {t.settings.couldNotLoadChangelog}
              </div>
              <div className="mt-1 text-sm text-muted-foreground">
                {error ?? t.settings.unableLoadChangelog}
              </div>
            </div>
            <button
              type="button"
              onClick={onRetry}
              className="rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground transition-colors hover:bg-muted"
            >
              {t.common.retry}
            </button>
          </div>
        </div>
      ) : null}
      {status === "success" && releases.length === 0 ? (
        <div className="rounded-lg border border-border bg-card/30 px-6 py-8">
          <div className="text-sm font-medium text-foreground">
            {t.settings.changelog}
          </div>
          <div className="mt-2 text-sm text-muted-foreground">
            {t.settings.noPublishedReleases}
          </div>
        </div>
      ) : null}
      {!canInstallUpdate && status === "success" ? (
        <div className="flex justify-end">
          <SettingsHeaderButton
            variant="outline"
            onClick={() => {
              if (!isChecking && !isUpdating)
                runWithPending(onCheckForUpdates, setCheckPending);
            }}
            disabled={isChecking || isUpdating}
          >
            {isChecking ? `${t.common.loading}…` : t.settings.checkForUpdates}
          </SettingsHeaderButton>
        </div>
      ) : null}
      {status === "success"
        ? releases.map((release) => {
            const normalizedTag = release.tag_name.replace(/^v/i, "");
            const isLatestRelease = normalizedTag === normalizedLatestVersion;
            const isCurrentRelease = normalizedTag === normalizedCurrentVersion;
            return (
              <article
                key={release.id}
                className={cn(
                  "rounded-xl border bg-card/30 py-4 pe-4 ps-6",
                  isLatestRelease ? "border-border bg-muted" : "border-border",
                )}
              >
                <div className="flex flex-row items-center min-w-0 flex-1 gap-3 ">
                  <div className="flex flex-row items-center min-w-0 flex-1 gap-2 ">
                    <div className="text-lg font-semibold tracking-[-0.2px] text-foreground">
                      {release.name?.trim() || release.tag_name}
                    </div>
                    <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm text-muted-foreground">
                      <span>{formatPublishedDate(release.published_at)}</span>
                      {release.prerelease ? (
                        <span className="rounded-full border border-border px-2.5 py-1 uppercase tracking-wide">
                          {t.settings.prerelease}
                        </span>
                      ) : null}
                    </div>
                  </div>
                  <div className="flex flex-row items-center justify-end min-w-0 flex-1 gap-2 ">
                    <a
                      href={release.html_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={t.settings.viewReleaseOnGithub}
                      className={cn(
                        buttonVariants({ variant: "ghost", size: "icon-sm" }),
                        "h-8 w-8 shrink-0 rounded-md hover:!bg-transparent hover:border-border/0",
                      )}
                    >
                      <GitHubIcon className="h-4 w-4" />
                    </a>
                    {isCurrentRelease ? (
                      <span className="bg-transparent border border-border text-secondary-foreground h-9 rounded-full px-3 text-sm h-auto gap-1.5 px-3 py-1.5">
                        {t.settings.currentRelease}
                      </span>
                    ) : null}
                    {isLatestRelease && canInstallUpdate ? (
                      <SettingsHeaderButton
                        variant="default"
                        onClick={() => {
                          if (!isChecking && !isUpdating)
                            runWithPending(onInstallUpdate, setInstallPending);
                        }}
                        disabled={isChecking || isUpdating}
                      >
                        <div className="flex flex-row items-center justify-center gap-2">
                          <DownloadCloud className="size-4" />
                          {isUpdating ? t.settings.updating : t.settings.update}
                        </div>
                      </SettingsHeaderButton>
                    ) : null}
                  </div>
                </div>
                {release.body?.trim() ? (
                  <div className="prose prose-sm mt-5 max-w-none text-foreground dark:prose-invert">
                    <Markdown
                      remarkPlugins={[remarkGfm]}
                      components={markdownComponents}
                    >
                      {release.body}
                    </Markdown>
                  </div>
                ) : (
                  <div className="mt-5 text-sm text-muted-foreground">
                    {t.settings.noReleaseNotes}
                  </div>
                )}
              </article>
            );
          })
        : null}
    </div>
  );
}

function GitHubIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      className={className}
    >
      <path d="M12 .5C5.649.5.5 5.649.5 12A11.5 11.5 0 0 0 8.36 22.04c.575.106.785-.25.785-.556 0-.274-.01-1-.015-1.962-3.181.691-3.853-1.532-3.853-1.532-.52-1.322-1.27-1.674-1.27-1.674-1.038-.71.08-.695.08-.695 1.148.08 1.752 1.178 1.752 1.178 1.02 1.748 2.676 1.243 3.328.95.103-.738.399-1.243.725-1.53-2.54-.289-5.211-1.27-5.211-5.65 0-1.248.446-2.27 1.177-3.07-.118-.288-.51-1.45.112-3.024 0 0 .96-.307 3.145 1.173A10.91 10.91 0 0 1 12 6.03c.973.004 1.954.132 2.87.387 2.182-1.48 3.14-1.173 3.14-1.173.625 1.573.233 2.736.115 3.024.734.8 1.175 1.822 1.175 3.07 0 4.39-2.676 5.358-5.224 5.642.41.353.776 1.05.776 2.117 0 1.528-.014 2.761-.014 3.136 0 .309.207.668.79.555A11.502 11.502 0 0 0 23.5 12C23.5 5.649 18.351.5 12 .5Z" />
    </svg>
  );
}
