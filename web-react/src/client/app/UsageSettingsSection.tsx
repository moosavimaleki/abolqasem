import { useCallback, useEffect, useState } from "react"
import { Archive, Database, Gauge, Loader2, RefreshCw, Trash2 } from "lucide-react"
import type { RateLimitWindowSnapshot } from "../../shared/types"
import { Button } from "../components/ui/button"
import { Input } from "../components/ui/input"
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "../components/ui/dialog"
import { formatBytes, formatLocalizedPercent, formatRateLimitDurationLocalized, formatRelativeResetTime, selectRateLimitWindows, type ResourceUsageSnapshot, type UsageSnapshot } from "../lib/usage"

function RateWindow({ value, locale }: { value: RateLimitWindowSnapshot; locale: "en" | "fa" }) {
  const used = Math.max(0, Math.min(100, value.usedPercent))
  const remaining = 100 - used
  const resetAfter = formatRelativeResetTime(value.resetsAt, locale)
  return (
    <div className="grid gap-2 rounded-xl border border-border/70 bg-muted/15 px-3 py-3 md:grid-cols-[90px_1fr_auto] md:items-center">
      <div className="text-xs font-medium text-foreground">{formatRateLimitDurationLocalized(value.windowDurationMins, locale)}</div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-foreground/60 transition-[width]" style={{ width: `${used}%` }} />
      </div>
      <div className="flex min-w-28 items-center justify-between gap-3 text-xs">
        <span className="font-medium tabular-nums text-foreground">{locale === "fa" ? `باقی‌مانده ${formatLocalizedPercent(remaining, locale)}` : `${formatLocalizedPercent(remaining, locale)} remaining`}</span>
        {resetAfter ? <span className="text-muted-foreground">{locale === "fa" ? `بازنشانی پس از ${resetAfter}` : `Resets in ${resetAfter}`}</span> : null}
      </div>
    </div>
  )
}

export function UsageSettingsSection({ locale }: { locale: "en" | "fa" }) {
  const fa = locale === "fa"
  const [usage, setUsage] = useState<UsageSnapshot | null>(null)
  const [resources, setResources] = useState<ResourceUsageSnapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [clearing, setClearing] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)
  const [confirmKind, setConfirmKind] = useState<"cache" | "checkpoints" | "archives" | "attachments" | null>(null)
  const [thresholdGB, setThresholdGB] = useState("2")
  const [autoCleanup, setAutoCleanup] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async (force = false) => {
    setLoading(true)
    setError(null)
    try {
      const [usageResponse, resourcesResponse] = await Promise.all([
        force
          ? fetch("/api/usage/refresh", { method: "POST", cache: "no-store" })
          : fetch("/api/usage", { cache: "no-store" }),
        fetch("/api/resources", { cache: "no-store" }),
      ])
      if (!usageResponse.ok || !resourcesResponse.ok) throw new Error(fa ? "خواندن آمار مصرف ناموفق بود" : "Could not load usage data")
      setUsage(await usageResponse.json() as UsageSnapshot)
      setResources(await resourcesResponse.json() as ResourceUsageSnapshot)
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError))
    } finally {
      setLoading(false)
    }
  }, [fa])

  useEffect(() => { void refresh(false) }, [refresh])

  useEffect(() => {
    void fetch("/api/settings", { cache: "no-store" }).then(async (response) => response.ok ? await response.json() as { disk_management?: { warning_threshold_bytes?: number; auto_cleanup?: boolean } } : null).then((settings) => {
      const policy = settings?.disk_management
      if (!policy) return
      setThresholdGB(String(Math.max(0.25, (policy.warning_threshold_bytes ?? 2 * 1024 ** 3) / 1024 ** 3)))
      setAutoCleanup(policy.auto_cleanup === true)
    }).catch(() => undefined)
  }, [])

  const clearCache = useCallback(async () => {
    setClearing(true)
    setError(null)
    try {
      const response = await fetch("/api/resources/cache", { method: "DELETE" })
      if (!response.ok) throw new Error(await response.text())
      const payload = await response.json() as { resources: ResourceUsageSnapshot }
      setResources(payload.resources)
      setConfirmClear(false)
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError))
    } finally {
      setClearing(false)
    }
  }, [])

  const clearResource = useCallback(async (kind: "cache" | "checkpoints" | "archives" | "attachments") => {
    setClearing(true)
    setError(null)
    try {
      const response = await fetch(`/api/resources/${kind}`, { method: "DELETE" })
      if (!response.ok) throw new Error(await response.text())
      const payload = await response.json() as { resources: ResourceUsageSnapshot }
      setResources(payload.resources)
      setConfirmKind(null)
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError))
    } finally {
      setClearing(false)
    }
  }, [])

  const saveDiskPolicy = useCallback(async (nextThresholdGB: string, nextAutoCleanup: boolean) => {
    const threshold = Math.max(0.25, Number(nextThresholdGB) || 2)
    const response = await fetch("/api/settings", { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ disk_management: { warning_threshold_bytes: Math.round(threshold * 1024 ** 3), auto_cleanup: nextAutoCleanup } }) })
    if (!response.ok) throw new Error(await response.text())
  }, [])

  if (loading && !usage && !resources) {
    return <div className="flex min-h-52 items-center justify-center text-muted-foreground"><Loader2 className="size-4 animate-spin" /></div>
  }

  const codex = usage?.codex
  const windows = selectRateLimitWindows(codex?.rate_limits ?? null)

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={() => void refresh(true)} disabled={loading}>
          {loading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
          {fa ? "به‌روزرسانی" : "Refresh"}
        </Button>
      </div>
      {error ? <div className="rounded-xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">{error}</div> : null}

      <section className="rounded-2xl border border-border bg-card/30 p-4" aria-labelledby="codex-usage-title">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2"><Gauge className="size-4 text-muted-foreground" /><h2 id="codex-usage-title" className="font-medium">{fa ? "مصرف Codex" : "Codex usage"}</h2></div>
          {codex?.rate_limits.planType ? <span className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">{codex.rate_limits.planType}</span> : null}
        </div>
        {windows.length ? <div className="space-y-2">{windows.map((window, index) => <RateWindow key={`${window.windowDurationMins}-${index}`} value={window} locale={locale} />)}</div> : (
          <p className="text-sm text-muted-foreground">{fa ? "هنوز اطلاعات محدودیت از app-server دریافت نشده است." : "No app-server limit snapshot has been received yet."}</p>
        )}
        {codex?.updated_at ? <div className="mt-3 text-xs text-muted-foreground">{fa ? "ثبت‌شده" : "Recorded"}: {new Date(codex.updated_at).toLocaleString(fa ? "fa-IR" : undefined)}</div> : null}
      </section>

      <section className="rounded-2xl border border-border bg-card/30 p-3 sm:p-4" aria-labelledby="cache-usage-title">
        <div className="mb-3 flex items-center gap-2"><Database className="size-4 text-muted-foreground" /><h2 id="cache-usage-title" className="font-medium">{fa ? "کش و فضای ذخیره‌سازی ابوالقاسم" : "Abolqasem cache and storage"}</h2></div>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-5">
          {[
            [fa ? "کش قابل پاک‌سازی" : "Clearable cache", resources?.storage.cache_bytes ?? 0],
            [fa ? "اتچمنت‌ها" : "Attachments", resources?.storage.upload_bytes ?? 0],
            [fa ? "چک‌پوینت‌ها" : "Checkpoints", resources?.storage.checkpoint_bytes ?? 0],
            [fa ? "آرشیو سشن‌ها" : "Archived sessions", resources?.storage.archive_bytes ?? 0],
            [fa ? "کل داده برنامه" : "Total app data", resources?.storage.total_bytes ?? 0],
          ].map(([label, bytes]) => <div key={String(label)} className="min-w-0 rounded-xl bg-muted/25 px-3 py-2.5"><div className="truncate text-xs text-muted-foreground">{label}</div><div className="mt-1 font-medium tabular-nums">{formatBytes(Number(bytes))}</div></div>)}
        </div>
        <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <Button variant="outline" size="sm" className="w-full" onClick={() => setConfirmKind("cache")} disabled={(resources?.storage.cache_bytes ?? 0) === 0}><Trash2 className="size-4" />{fa ? "پاک‌سازی کش" : "Clear cache"}</Button>
          <Button variant="outline" size="sm" className="w-full" onClick={() => setConfirmKind("attachments")} disabled={(resources?.storage.upload_bytes ?? 0) === 0}><Trash2 className="size-4" />{fa ? "حذف پیوست‌ها" : "Delete attachments"}</Button>
          <Button variant="outline" size="sm" className="w-full" onClick={() => setConfirmKind("checkpoints")} disabled={(resources?.storage.checkpoint_bytes ?? 0) === 0}><Trash2 className="size-4" />{fa ? "پاک‌سازی چک‌پوینت‌ها" : "Clear checkpoints"}</Button>
          <Button variant="outline" size="sm" className="w-full" onClick={() => setConfirmKind("archives")} disabled={(resources?.storage.archive_bytes ?? 0) === 0}><Archive className="size-4" />{fa ? "پاک‌سازی آرشیو سشن‌ها" : "Clear archived sessions"}</Button>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">{fa ? "پیوست‌ها فقط با اقدام دستی حذف می‌شوند؛ سشن‌های فعال دست‌نخورده می‌مانند." : "Attachments require an explicit delete; active sessions remain untouched."}</p>
      </section>

      <section className="rounded-2xl border border-border bg-card/30 p-4" aria-labelledby="disk-policy-title">
        <div className="mb-3 flex items-center gap-2"><Database className="size-4 text-muted-foreground" /><h2 id="disk-policy-title" className="font-medium">{fa ? "سیاست هشدار دیسک" : "Disk warning policy"}</h2></div>
        <div className="grid gap-3 md:grid-cols-[11rem_minmax(0,1fr)_auto] md:items-end">
          <label className="grid gap-1.5 text-sm font-medium"><span>{fa ? "هشدار از حجم" : "Warn above"}</span><div className="flex items-center gap-2"><Input className="h-9" dir="ltr" type="number" min="0.25" step="0.25" value={thresholdGB} onChange={(event) => setThresholdGB(event.target.value)} onBlur={() => void saveDiskPolicy(thresholdGB, autoCleanup).catch((nextError) => setError(String(nextError)))} /><span className="text-xs text-muted-foreground">GB</span></div></label>
          <label className="flex min-h-9 items-center gap-2 text-sm"><input type="checkbox" checked={autoCleanup} onChange={(event) => { setAutoCleanup(event.target.checked); void saveDiskPolicy(thresholdGB, event.target.checked).catch((nextError) => setError(String(nextError))) }} /><span>{fa ? "پس از عبور از حد، کش و چک‌پوینت‌ها خودکار پاک شوند" : "Automatically clear cache and checkpoints above the limit"}</span></label>
          <Button variant="outline" size="sm" onClick={() => void refresh(true)} disabled={loading}>{fa ? "بررسی فضا" : "Check now"}</Button>
        </div>
      </section>

      <Dialog open={confirmClear} onOpenChange={setConfirmClear}>
        <DialogContent size="sm">
          <DialogBody><DialogTitle>{fa ? "کش جست‌وجو پاک شود؟" : "Clear search cache?"}</DialogTitle><DialogDescription>{fa ? "ایندکس‌ها هنگام جست‌وجوی بعدی دوباره ساخته می‌شوند." : "Indexes will be rebuilt on the next search."}</DialogDescription></DialogBody>
          <DialogFooter><Button variant="ghost" onClick={() => setConfirmClear(false)}>{fa ? "انصراف" : "Cancel"}</Button><Button variant="destructive" onClick={() => void clearCache()} disabled={clearing}>{clearing ? <Loader2 className="size-4 animate-spin" /> : null}{fa ? "پاک کن" : "Clear"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={confirmKind !== null} onOpenChange={(open) => { if (!open) setConfirmKind(null) }}>
        <DialogContent size="sm"><DialogBody><DialogTitle>{fa ? "این داده‌ها پاک شوند؟" : "Remove these files?"}</DialogTitle><DialogDescription>{fa ? (confirmKind === "attachments" ? "پیوست‌های ذخیره‌شده حذف می‌شوند و دیگر در چت‌ها قابل باز کردن نیستند. این عملیات قابل بازگشت نیست." : "این عملیات قابل بازگشت نیست؛ سشن‌های فعال و پیوست‌ها دست‌نخورده می‌مانند.") : (confirmKind === "attachments" ? "Stored attachments will no longer be available in chats. This cannot be undone." : "This cannot be undone. Active sessions and attachments remain untouched.")}</DialogDescription></DialogBody><DialogFooter><Button variant="ghost" onClick={() => setConfirmKind(null)}>{fa ? "انصراف" : "Cancel"}</Button><Button variant="destructive" onClick={() => { if (confirmKind) void clearResource(confirmKind) }} disabled={clearing}>{clearing ? <Loader2 className="size-4 animate-spin" /> : null}{fa ? "پاک کن" : "Remove"}</Button></DialogFooter></DialogContent>
      </Dialog>
    </div>
  )
}
