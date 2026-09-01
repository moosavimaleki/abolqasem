import { useEffect, useMemo, useState } from "react";
import { Loader2, RefreshCw } from "lucide-react";
import type { AppLocale } from "../../../shared/types";
import { Button } from "../ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";

type HistorySample = { at?: string; windows?: Record<string, number> };
type HistoryPoint = { at?: string; value?: number };
type Account = { name: string; active?: boolean };
type Range = "7d" | "30d" | "90d" | "all";
type Timezone = "UTC" | "local" | "+03:30";

const tr = (locale: AppLocale, fa: string, en: string) =>
  locale === "fa" ? fa : en;
const ranges: Array<{ value: Range; fa: string; en: string }> = [
  { value: "7d", fa: "۷ روز", en: "7 days" },
  { value: "30d", fa: "۳۰ روز", en: "30 days" },
  { value: "90d", fa: "۹۰ روز", en: "90 days" },
  { value: "all", fa: "همه", en: "All" },
];

// The API already bounds chart responses, but retain a small client-side guard
// for older servers and proxied responses. Sampling keeps both endpoints so a
// trend can never lose its oldest/newest reading.
export function limitHistoryPoints<T>(points: T[], limit: number): T[] {
  if (!Number.isFinite(limit) || limit <= 0) return [];
  if (points.length <= limit) return points;
  if (limit === 1) return [points[points.length - 1]];

  const lastIndex = points.length - 1;
  return Array.from(
    { length: limit },
    (_, index) => points[Math.round((index * lastIndex) / (limit - 1))],
  );
}

function formatDate(
  value: string | undefined,
  locale: AppLocale,
  timezone: Timezone,
) {
  const date = value ? new Date(value) : null;
  if (!date || !Number.isFinite(date.getTime())) return "—";
  const timeZone =
    timezone === "UTC"
      ? "UTC"
      : timezone === "+03:30"
        ? "Asia/Tehran"
        : undefined;
  return new Intl.DateTimeFormat(locale === "fa" ? "fa-IR" : undefined, {
    dateStyle: "short",
    timeStyle: "short",
    timeZone,
  }).format(date);
}

export function UsageHistoryChart({ locale }: { locale: AppLocale }) {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [account, setAccount] = useState("");
  const [samples, setSamples] = useState<HistorySample[]>([]);
  const [windowName, setWindowName] = useState("");
  const [range, setRange] = useState<Range>("7d");
  const [timezone, setTimezone] = useState<Timezone>("local");
  const [points, setPoints] = useState<HistoryPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch("/api/codex-manager/accounts", {
          cache: "no-store",
        });
        if (!response.ok) throw new Error(await response.text());
        const payload = (await response.json()) as { accounts?: Account[] };
        const next = payload.accounts ?? [];
        if (cancelled) return;
        setAccounts(next);
        setAccount((current) =>
          current && next.some((item) => item.name === current)
            ? current
            : (next.find((item) => item.active)?.name ?? next[0]?.name ?? ""),
        );
      } catch (cause) {
        if (!cancelled)
          setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  useEffect(() => {
    if (!account) {
      setSamples([]);
      setPoints([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(
          `/api/codex-manager/history?account=${encodeURIComponent(account)}&range=${range}&limit=500`,
          { cache: "no-store" },
        );
        if (!response.ok) throw new Error(await response.text());
        const payload = (await response.json()) as { items?: HistorySample[] };
        if (!cancelled)
          setSamples(limitHistoryPoints(payload.items ?? [], 500));
      } catch (cause) {
        if (!cancelled)
          setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [account, range, refreshKey]);

  const windows = useMemo(() => {
    const names = new Set<string>();
    samples.forEach((sample) =>
      Object.keys(sample.windows ?? {}).forEach((name) => names.add(name)),
    );
    return [...names].sort();
  }, [samples]);
  useEffect(() => {
    if (!windowName || !windows.includes(windowName))
      setWindowName(windows[0] ?? "");
  }, [windowName, windows]);

  useEffect(() => {
    if (!account || !windowName) {
      setPoints([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const query = new URLSearchParams({
          account,
          window: windowName,
          range,
          timezone,
          limit: "120",
        });
        const response = await fetch(
          `/api/codex-manager/history?${query.toString()}`,
          { cache: "no-store" },
        );
        if (!response.ok) throw new Error(await response.text());
        const payload = (await response.json()) as {
          series?: { points?: HistoryPoint[] };
        };
        if (!cancelled)
          setPoints(limitHistoryPoints(payload.series?.points ?? [], 120));
      } catch (cause) {
        if (!cancelled)
          setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [account, windowName, range, timezone, refreshKey]);

  const values = points.map((point) =>
    Math.max(0, Math.min(100, Number(point.value) || 0)),
  );
  const width = 640;
  const height = 150;
  const pad = 14;
  const coordinates = values
    .map(
      (value, index) =>
        `${pad + (index * (width - pad * 2)) / Math.max(1, values.length - 1)},${height - pad - (value * (height - pad * 2)) / 100}`,
    )
    .join(" ");
  const hasData = samples.length > 0;

  return (
    <section
      className="grid gap-3 rounded-2xl border border-border bg-card/30 p-4"
      dir={locale === "fa" ? "rtl" : "ltr"}
      aria-labelledby="codex-usage-history-title"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 id="codex-usage-history-title" className="text-sm font-semibold">
            {tr(locale, "تاریخچه مصرف حساب‌ها", "Account usage history")}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {tr(
              locale,
              "نشان می‌دهد سهمیهٔ انتخاب‌شده در روزهای گذشته چگونه مصرف شده است؛ از این نمودار برای تشخیص زمان مناسبِ جابه‌جایی حساب استفاده کنید.",
              "Shows how the selected allowance has been used over recent days, so you can decide when switching accounts makes sense.",
            )}
          </p>
        </div>
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={() => setRefreshKey((value) => value + 1)}
          disabled={loading}
          aria-label={tr(locale, "به‌روزرسانی نمودار", "Refresh chart")}
        >
          {loading ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <RefreshCw className="size-3.5" />
          )}
        </Button>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <Select value={account || undefined} onValueChange={setAccount}>
          <SelectTrigger aria-label={tr(locale, "حساب", "Account")}>
            <SelectValue
              placeholder={tr(locale, "انتخاب حساب", "Select account")}
            />
          </SelectTrigger>
          <SelectContent dir={locale === "fa" ? "rtl" : "ltr"}>
            {accounts.map((item) => (
              <SelectItem key={item.name} value={item.name}>
                {item.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={windowName || undefined}
          onValueChange={setWindowName}
          disabled={!hasData || windows.length === 0}
        >
          <SelectTrigger aria-label={tr(locale, "بازه سهمیه", "Quota window")}>
            <SelectValue
              placeholder={tr(locale, "بازه سهمیه", "Quota window")}
            />
          </SelectTrigger>
          <SelectContent dir="ltr">
            {windows.map((name) => (
              <SelectItem key={name} value={name}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={range}
          onValueChange={(value) => setRange(value as Range)}
        >
          <SelectTrigger aria-label={tr(locale, "محدوده زمانی", "Time range")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent dir={locale === "fa" ? "rtl" : "ltr"}>
            {ranges.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {locale === "fa" ? item.fa : item.en}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={timezone}
          onValueChange={(value) => setTimezone(value as Timezone)}
        >
          <SelectTrigger aria-label={tr(locale, "منطقه زمانی", "Timezone")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent dir={locale === "fa" ? "rtl" : "ltr"}>
            <SelectItem value="local">
              {tr(locale, "زمان محلی", "Local time")}
            </SelectItem>
            <SelectItem value="UTC">UTC</SelectItem>
            <SelectItem value="+03:30">UTC+03:30</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {error ? (
        <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
          {error}
        </p>
      ) : loading && !hasData ? (
        <div
          className="h-40 animate-pulse rounded-lg bg-muted/25"
          aria-label={tr(locale, "در حال بارگذاری", "Loading")}
        />
      ) : points.length === 0 ? (
        <p className="rounded-md border border-dashed border-border px-3 py-8 text-center text-xs text-muted-foreground">
          {tr(
            locale,
            "برای این انتخاب هنوز داده‌ای ثبت نشده است.",
            "No samples for this selection yet.",
          )}
        </p>
      ) : (
        <div className="rounded-lg border border-border/70 bg-background/40 p-2">
          <svg
            viewBox={`0 0 ${width} ${height}`}
            className="h-40 w-full"
            role="img"
            aria-label={tr(
              locale,
              "نمودار درصد باقی‌مانده",
              "Remaining quota chart",
            )}
          >
            <line
              x1={pad}
              x2={width - pad}
              y1={height - pad}
              y2={height - pad}
              stroke="currentColor"
              className="text-border"
            />
            <polyline
              fill="none"
              stroke="currentColor"
              strokeWidth="3"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="text-primary"
              points={coordinates}
            />
          </svg>
          <div className="flex justify-between text-[11px] text-muted-foreground">
            <span>{formatDate(points[0]?.at, locale, timezone)}</span>
            <span>{Math.round(values[values.length - 1] ?? 0)}%</span>
          </div>
        </div>
      )}
    </section>
  );
}
