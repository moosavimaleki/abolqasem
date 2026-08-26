import { useCallback, useEffect, useState } from "react"
import { Database, Gauge, Loader2, RefreshCw, Trash2 } from "lucide-react"
import type { RateLimitWindowSnapshot } from "../../shared/types"
import { Button } from "../components/ui/button"
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "../components/ui/dialog"
import { formatBytes, formatRateLimitDuration, type ResourceUsageSnapshot, type UsageSnapshot } from "../lib/usage"

function RateWindow({ value, locale }: { value: RateLimitWindowSnapshot; locale: "en" | "fa" }) {
  const used = Math.max(0, Math.min(100, value.usedPercent))
  const reset = value.resetsAt ? new Date(value.resetsAt * 1000) : null
  return (
    <div className="grid gap-2 rounded-xl border border-border/70 bg-muted/15 px-3 py-3 md:grid-cols-[90px_1fr_auto] md:items-center">
      <div className="text-xs font-medium text-foreground">{formatRateLimitDuration(value.windowDurationMins)}</div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-foreground/60 transition-[width]" style={{ width: `${used}%` }} />
      </div>
      <div className="flex min-w-28 items-center justify-between gap-3 text-xs">
        <span className="font-medium text-foreground">{Math.round(used)}%</span>
        {reset ? <span className="text-muted-foreground">{reset.toLocaleString(locale === "fa" ? "fa-IR" : undefined)}</span> : null}
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

  if (loading && !usage && !resources) {
    return <div className="flex min-h-52 items-center justify-center text-muted-foreground"><Loader2 className="size-4 animate-spin" /></div>
  }

  const codex = usage?.codex
  const windows = [codex?.rate_limits.primary, codex?.rate_limits.secondary].filter((item): item is RateLimitWindowSnapshot => Boolean(item))

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

      <section className="rounded-2xl border border-border bg-card/30 p-4" aria-labelledby="cache-usage-title">
        <div className="mb-4 flex items-center gap-2"><Database className="size-4 text-muted-foreground" /><h2 id="cache-usage-title" className="font-medium">{fa ? "کش و فضای ذخیره‌سازی ابوالقاسم" : "Abolqasem cache and storage"}</h2></div>
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-xl bg-muted/25 p-3"><div className="text-xs text-muted-foreground">{fa ? "کش قابل پاک‌سازی" : "Clearable cache"}</div><div className="mt-1 font-medium">{formatBytes(resources?.storage.cache_bytes ?? 0)}</div></div>
          <div className="rounded-xl bg-muted/25 p-3"><div className="text-xs text-muted-foreground">{fa ? "اتچمنت‌ها" : "Attachments"}</div><div className="mt-1 font-medium">{formatBytes(resources?.storage.upload_bytes ?? 0)}</div></div>
          <div className="rounded-xl bg-muted/25 p-3"><div className="text-xs text-muted-foreground">{fa ? "کل داده برنامه" : "Total app data"}</div><div className="mt-1 font-medium">{formatBytes(resources?.storage.total_bytes ?? 0)}</div></div>
        </div>
        <div className="mt-4 flex items-center justify-between gap-4">
          <p className="text-xs text-muted-foreground">{fa ? "فقط ایندکس‌های جست‌وجوی قابل بازسازی پاک می‌شوند؛ سشن‌ها، checkpointها و فایل‌ها باقی می‌مانند." : "Only rebuildable search indexes are cleared; sessions, checkpoints, and files remain."}</p>
          <Button variant="outline" size="sm" onClick={() => setConfirmClear(true)} disabled={(resources?.storage.cache_bytes ?? 0) === 0}>
            <Trash2 className="size-4" />{fa ? "پاک‌سازی کش" : "Clear cache"}
          </Button>
        </div>
      </section>

      <Dialog open={confirmClear} onOpenChange={setConfirmClear}>
        <DialogContent size="sm">
          <DialogBody><DialogTitle>{fa ? "کش جست‌وجو پاک شود؟" : "Clear search cache?"}</DialogTitle><DialogDescription>{fa ? "ایندکس‌ها هنگام جست‌وجوی بعدی دوباره ساخته می‌شوند." : "Indexes will be rebuilt on the next search."}</DialogDescription></DialogBody>
          <DialogFooter><Button variant="ghost" onClick={() => setConfirmClear(false)}>{fa ? "انصراف" : "Cancel"}</Button><Button variant="destructive" onClick={() => void clearCache()} disabled={clearing}>{clearing ? <Loader2 className="size-4 animate-spin" /> : null}{fa ? "پاک کن" : "Clear"}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
