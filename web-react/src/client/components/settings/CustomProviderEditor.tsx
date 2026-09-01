import { useMemo, useState } from "react"
import { Loader2, Plus, Save, TestTube2 } from "lucide-react"
import type { AppLocale, AppSettingsSnapshot } from "../../../shared/types"
import { Button } from "../ui/button"
import { Input } from "../ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select"

type ProviderModel = AppSettingsSnapshot["codexBackend"]["customProviders"][string]["models"][number]

type ProviderDraft = {
  id: string
  name: string
  baseUrl: string
  apiKey: string
  headers: string
  models: ProviderModel[]
}

function emptyDraft(): ProviderDraft {
  return { id: "", name: "", baseUrl: "", apiKey: "", headers: "", models: [] }
}

function parseHeaders(value: string): Record<string, string> {
  if (!value.trim()) return {}
  const raw = JSON.parse(value) as unknown
  if (!raw || Array.isArray(raw) || typeof raw !== "object") throw new Error("Headers must be a JSON object.")
  const headers: Record<string, string> = {}
  for (const [name, headerValue] of Object.entries(raw)) {
    if (typeof headerValue !== "string") throw new Error("Header values must be strings.")
    if (name.trim() && headerValue.trim()) headers[name.trim()] = headerValue.trim()
  }
  return headers
}

function localized(locale: AppLocale, fa: string, en: string) { return locale === "fa" ? fa : en }

export function CustomProviderEditor({
  locale,
  providers,
  activeProviderID,
  onActivate,
  onRefresh,
}: {
  locale: AppLocale
  providers: AppSettingsSnapshot["codexBackend"]["customProviders"]
  activeProviderID: string
  onActivate: (providerID: string) => Promise<void>
  onRefresh: () => Promise<void>
}) {
  const [selectedID, setSelectedID] = useState(activeProviderID)
  const [draft, setDraft] = useState<ProviderDraft>(emptyDraft)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const configuredIDs = useMemo(() => Object.keys(providers).sort(), [providers])

  async function request(action: "" | "test", discover: boolean) {
    const headers = parseHeaders(draft.headers)
    const response = await fetch(`/api/custom-providers/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({
        provider: { id: draft.id, name: draft.name, baseUrl: draft.baseUrl, wireApi: "responses", models: draft.models },
        headers,
        apiKey: draft.apiKey,
        discover,
      }),
    })
    if (!response.ok) throw new Error(await response.text())
    return await response.json() as { models?: ProviderModel[] }
  }

  async function testProvider() {
    setTesting(true)
    setError(null)
    setNotice(null)
    try {
      const result = await request("test", true)
      if (result.models?.length) setDraft((current) => ({ ...current, models: result.models ?? current.models }))
      setNotice(result.models?.length
        ? localized(locale, `${result.models.length} مدل پیدا شد؛ قبل از ذخیره می‌توانید aliasها را تغییر دهید.`, `${result.models.length} models discovered; adjust aliases before saving if needed.`)
        : localized(locale, "اتصال تأیید شد.", "Connection verified."))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setTesting(false)
    }
  }

  async function saveProvider() {
    setSaving(true)
    setError(null)
    setNotice(null)
    try {
      await request("", false)
      await onRefresh()
      setSelectedID(draft.id.trim())
      setNotice(localized(locale, "Provider ذخیره شد؛ برای استفاده، آن را فعال کنید.", "Provider saved. Activate it when you are ready to use it."))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSaving(false)
    }
  }

  async function activateProvider(id: string) {
    if (!id || saving || testing) return
    setSaving(true)
    setError(null)
    try {
      await onActivate(id)
      setSelectedID(id)
      setNotice(localized(locale, "Provider برای turnهای جدید فعال شد.", "Provider is active for new turns."))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSaving(false)
    }
  }

  const busy = saving || testing
  return (
    <section className="grid gap-3 rounded-lg border border-border bg-card/30 p-3" dir={locale === "fa" ? "rtl" : "ltr"}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{localized(locale, "Custom Provider", "Custom Provider")}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{localized(locale, "فقط اینجا alias و mapping مدل نمایش داده می‌شود؛ در Manager مدل‌ها native هستند.", "Only this mode exposes model aliases and mappings; Manager retains native models.")}</p>
        </div>
        <div className="flex items-center gap-2">
          {configuredIDs.length > 0 ? (
            <Select value={selectedID || activeProviderID} onValueChange={setSelectedID} disabled={busy}>
              <SelectTrigger className="h-8 min-w-[150px]"><SelectValue placeholder={localized(locale, "Provider ذخیره‌شده", "Saved provider")} /></SelectTrigger>
              <SelectContent>{configuredIDs.map((id) => <SelectItem key={id} value={id}>{providers[id]?.name || id}</SelectItem>)}</SelectContent>
            </Select>
          ) : null}
          <Button type="button" size="sm" variant="outline" disabled={busy || !(selectedID || activeProviderID)} onClick={() => void activateProvider(selectedID || activeProviderID)}>
            {saving ? <Loader2 className="size-3.5 animate-spin" /> : null}{localized(locale, "استفاده از این Provider", "Use this provider")}
          </Button>
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <Input value={draft.id} disabled={busy} dir="ltr" placeholder={localized(locale, "شناسهٔ پایدار (مثلاً company)", "Stable ID (for example, company)")} onChange={(event) => setDraft((current) => ({ ...current, id: event.target.value }))} />
        <Input value={draft.name} disabled={busy} placeholder={localized(locale, "نام نمایشی", "Display name")} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} />
        <Input value={draft.baseUrl} disabled={busy} dir="ltr" placeholder="https://api.example.com/v1" onChange={(event) => setDraft((current) => ({ ...current, baseUrl: event.target.value }))} />
        <Input value={draft.apiKey} disabled={busy} dir="ltr" type="password" autoComplete="new-password" placeholder={localized(locale, "API key (اختیاری)", "API key (optional)")} onChange={(event) => setDraft((current) => ({ ...current, apiKey: event.target.value }))} />
      </div>

      <details className="rounded-md border border-border/70 px-2 py-1.5">
        <summary className="cursor-pointer text-xs font-medium">{localized(locale, "تنظیمات پیشرفته: header و mapping مدل", "Advanced: headers and model mappings")}</summary>
        <div className="mt-2 grid gap-2">
          <textarea value={draft.headers} disabled={busy} dir="ltr" className="min-h-16 w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:ring-1 focus-visible:ring-ring" placeholder='{"X-Organization":"example"}' onChange={(event) => setDraft((current) => ({ ...current, headers: event.target.value }))} />
          {draft.models.length > 0 ? draft.models.map((model, index) => (
            <div className="grid gap-2 sm:grid-cols-2" key={`${model.id}-${index}`}>
              <Input value={model.id} disabled={busy} dir="ltr" placeholder={localized(locale, "شناسه در UI", "UI model ID")} onChange={(event) => setDraft((current) => ({ ...current, models: current.models.map((candidate, candidateIndex) => candidateIndex === index ? { ...candidate, id: event.target.value } : candidate) }))} />
              <Input value={model.upstreamId} disabled={busy} dir="ltr" placeholder={localized(locale, "شناسهٔ upstream", "Upstream model ID")} onChange={(event) => setDraft((current) => ({ ...current, models: current.models.map((candidate, candidateIndex) => candidateIndex === index ? { ...candidate, upstreamId: event.target.value } : candidate) }))} />
            </div>
          )) : <p className="text-xs text-muted-foreground">{localized(locale, "مدلی اضافه نشده است؛ هر شناسه مدل بدون mapping همان‌طور که هست ارسال می‌شود.", "No models are mapped; any model ID is sent through unchanged.")}</p>}
          <Button type="button" size="sm" variant="ghost" className="w-fit" disabled={busy} onClick={() => setDraft((current) => ({ ...current, models: [...current.models, { id: "", upstreamId: "" }] }))}><Plus className="size-3.5" />{localized(locale, "افزودن mapping", "Add mapping")}</Button>
        </div>
      </details>

      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" variant="outline" disabled={busy} onClick={() => void testProvider()}>{testing ? <Loader2 className="size-3.5 animate-spin" /> : <TestTube2 className="size-3.5" />}{localized(locale, "تست و کشف مدل", "Test & discover models")}</Button>
        <Button type="button" size="sm" disabled={busy} onClick={() => void saveProvider()}>{saving ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}{localized(locale, "ذخیرهٔ Provider", "Save provider")}</Button>
        {notice ? <span className="text-xs text-emerald-600 dark:text-emerald-400">{notice}</span> : null}
        {error ? <span className="text-xs text-destructive">{error}</span> : null}
      </div>
    </section>
  )
}
