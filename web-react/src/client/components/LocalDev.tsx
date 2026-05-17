import { useMemo, useState, type ComponentType, type ReactNode } from "react"
import {
  ArrowLeftRight,
  Check,
  ChevronRight,
  CodeXml,
  Copy,
  Folder,
  Loader2,
  Monitor,
  Plus,
  SquarePen,
  Terminal,
} from "lucide-react"
import { getCliInvocation, LOCAL_UI_URL, SDK_CLIENT_APP } from "../../shared/branding"
import type { LocalProjectsSnapshot } from "../../shared/types"
import type { SocketStatus } from "../app/socket"
import { PageHeader } from "../app/PageHeader"
import { getPathBasename } from "../lib/formatters"
import { cn } from "../lib/utils"
import { NewProjectModal } from "./NewProjectModal"
import { Button } from "./ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"
import { useI18n } from "../i18n/context"

interface LocalDevProps {
  connectionStatus: SocketStatus
  ready: boolean
  snapshot: LocalProjectsSnapshot | null
  startingLocalPath: string | null
  commandError: string | null
  newProjectOpen: boolean
  onNewProjectOpenChange: (open: boolean) => void
  onOpenProject: (localPath: string) => Promise<void>
  onCreateProject: (project: { mode: "new" | "existing"; localPath: string; title: string }) => Promise<void>
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Button
      variant="ghost"
      size="icon"
      className="h-8 w-8 text-muted-foreground hover:text-foreground"
      onClick={() => void handleCopy()}
    >
      {copied ? <Check className="h-4 w-4 text-green-400" /> : <Copy className="h-4 w-4" />}
    </Button>
  )
}

function CodeBlock({ children }: { children: string }) {
  return (
    <div dir="ltr" className="group grid grid-cols-[1fr_auto] items-center rounded-xl border border-border bg-background p-1.5 ps-3 font-mono text-sm text-foreground">
      <pre className="inline-flex items-center gap-2 overflow-x-auto">
        <ChevronRight className="inline h-4 w-4 opacity-40" />
        <code>{children}</code>
      </pre>
      <CopyButton text={children} />
    </div>
  )
}

function InfoCard({ children }: { children: ReactNode }) {
  return <div className="bg-card border border-border rounded-2xl p-4">{children}</div>
}

function SectionHeader({ children }: { children: ReactNode }) {
  return (
    <h2 className="text-[13px] font-medium text-muted-foreground uppercase tracking-wider mb-3">
      {children}
    </h2>
  )
}

function HowItWorksItem({
  icon: Icon,
  title,
  subtitle,
  iconClassName,
}: {
  icon: ComponentType<{ className?: string }>
  title: string
  subtitle: string
  iconClassName?: string
}) {
  return (
    <div className="flex flex-col items-center gap-0">
      <div className="p-3 mb-2 rounded-xl bg-background border border-border">
        <Icon className={iconClassName || "h-8 w-8 text-muted-foreground"} />
      </div>
      <span className="text-sm font-medium">{title}</span>
      <span className="text-xs text-muted-foreground">{subtitle}</span>
    </div>
  )
}

function HowItWorksConnector() {
  return <ArrowLeftRight className="h-4 w-4 text-muted-foreground" />
}

function Step({
  number,
  title,
  children,
}: {
  number: number
  title: string
  children: ReactNode
}) {
  return (
    <div className="flex gap-4">
      <div className="flex-1 min-w-0">
        <div className="grid grid-cols-[auto_1fr] items-baseline gap-3">
          <div className="flex-shrink-0 flex items-center justify-center font-medium text-logo">{number}.</div>
          <h3 className="font-medium text-foreground mb-2">{title}</h3>
        </div>
        <div className="text-muted-foreground text-sm space-y-3">{children}</div>
      </div>
    </div>
  )
}

function ProjectCard({
  localPath,
  loading,
  onClick,
}: {
  localPath: string
  loading: boolean
  onClick: () => void
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          className={cn(
            "group flex w-full items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 text-start transition-colors hover:border-primary/30 hover:bg-muted/50",
            loading && "opacity-50 cursor-not-allowed"
          )}
          disabled={loading}
          onClick={onClick}
        >
          <Folder className="h-4 w-4 text-muted-foreground flex-shrink-0" />
          <span className="font-medium text-foreground truncate flex-1">
            {getPathBasename(localPath)}
          </span>
          {loading ? (
            <Loader2 className="h-4 w-4 text-muted-foreground group-hover:text-primary animate-spin flex-shrink-0" />
          ) : (
            <SquarePen className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0" />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>
        <p>{localPath}</p>
      </TooltipContent>
    </Tooltip>
  )
}

export function LocalDev({
  connectionStatus,
  ready,
  snapshot,
  startingLocalPath,
  commandError,
  newProjectOpen,
  onNewProjectOpenChange,
  onOpenProject,
  onCreateProject,
}: LocalDevProps) {
  const { t, direction } = useI18n()
  const projects = useMemo(() => snapshot?.projects ?? [], [snapshot?.projects])
  const isConnecting = connectionStatus === "connecting" || !ready
  const isConnected = connectionStatus === "connected" && ready

  return (
    <div className="flex-1 flex flex-col min-w-0 bg-background overflow-y-auto [direction:ltr]">
      <div className="flex min-h-full flex-col" dir={direction}>
      {!isConnected ? (
        <>
          <PageHeader
            narrow
            icon={CodeXml}
            title={isConnecting ? t.localDev.connectingTitle() : t.localDev.connectTitle()}
            subtitle={isConnecting
              ? t.localDev.connectingSubtitle()
              : t.localDev.disconnectedSubtitle()}
          />
          <div className="max-w-2xl w-full mx-auto pb-12 px-6">
            <SectionHeader>{t.localDev.status}</SectionHeader>
            <div className="mb-8">
              <InfoCard>
                <div className="flex items-center gap-3">
                  <Loader2 className="h-4 w-4 text-muted-foreground animate-spin" />
                  <span className="text-sm text-muted-foreground">
                    {isConnecting ? (
                      t.localDev.connectingServer()
                    ) : (
                      <>
                        {t.localDev.notConnectedPrefix} <code className="bg-background border border-border rounded-md mx-0.5 p-1 font-mono text-xs text-foreground">{getCliInvocation("open")}</code> {t.localDev.notConnectedSuffix}
                      </>
                    )}
                  </span>
                </div>
              </InfoCard>
            </div>

            {!isConnecting ? (
              <div className="mb-10">
              <SectionHeader>{t.localDev.howItWorks}</SectionHeader>
              <InfoCard>
                <div className="flex items-center justify-around gap-6 py-4 px-2">
                  <HowItWorksItem icon={Terminal} title={t.localDev.cliTitle()} subtitle={t.localDev.cliSubtitle} />
                  <HowItWorksConnector />
                  <HowItWorksItem icon={Monitor} title={t.localDev.serverTitle()} subtitle={t.localDev.localWebSocket} />
                  <HowItWorksConnector />
                  <HowItWorksItem icon={CodeXml} title={t.localDev.uiTitle()} subtitle={t.localDev.projectChat} />
                </div>
              </InfoCard>
              </div>
            ) : null}

            {!isConnecting ? (
              <div className="mb-10">
              <SectionHeader>{t.localDev.setup}</SectionHeader>
              <InfoCard>
                <div className="space-y-4">
                  <Step number={1} title={t.localDev.startApp()}>
                    <p>{t.localDev.runCommand}</p>
                    <CodeBlock>{getCliInvocation("open")}</CodeBlock>
                  </Step>

                  <Step number={2} title={t.localDev.openLocalUi}>
                    <p>{t.localDev.openLocalUiDescription()}</p>
                    <CodeBlock>{LOCAL_UI_URL}</CodeBlock>
                  </Step>

                  <div className="mt-8">
                    <h3 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-4">{t.localDev.notes}</h3>
                    <div className="space-y-3 text-sm">
                      <div className="flex gap-4">
                        <code className="font-mono text-foreground whitespace-nowrap">{getCliInvocation("status")}</code>
                        <span className="text-muted-foreground">{t.localDev.checkStatus}</span>
                      </div>
                      <div className="flex gap-4">
                        <code className="font-mono text-foreground whitespace-nowrap">{getCliInvocation("service start")}</code>
                        <span className="text-muted-foreground">{t.localDev.startBackgroundService}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </InfoCard>
              </div>
            ) : null}
          </div>
        </>
      ) : (
        <>
          <PageHeader
            title={snapshot?.machine.displayName ?? t.localDev.localProjects}
            subtitle={t.localDev.connectedSubtitle()}
          />

          <div className="w-full px-6 mb-10">
            <div className="flex items-baseline justify-between mb-3">
              <h2 className="text-[13px] font-medium text-muted-foreground uppercase tracking-wider">{t.localDev.projects}</h2>
              <Button variant="default" size="sm" onClick={() => onNewProjectOpenChange(true)}>
                <Plus className="h-4 w-4 me-1.5" />
                {t.localDev.addProject}
              </Button>
            </div>
            {projects.length > 0 ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-4 3xl:grid-cols-5 gap-2">
                {projects.map((project) => (
                  <ProjectCard
                    key={project.localPath}
                    localPath={project.localPath}
                    loading={startingLocalPath === project.localPath}
                    onClick={() => {
                      void onOpenProject(project.localPath)
                    }}
                  />
                ))}
              </div>
            ) : (
              <InfoCard>
                <p className="text-sm text-muted-foreground">
                  {t.localDev.noProjects}
                </p>
              </InfoCard>
            )}
            {commandError ? (
              <div className="text-sm text-destructive border border-destructive/20 bg-destructive/5 rounded-xl px-4 py-3 mt-4">
                {commandError}
              </div>
            ) : null}
          </div>
        </>
      )}

      <NewProjectModal
        open={newProjectOpen}
        onOpenChange={onNewProjectOpenChange}
        onConfirm={(project) => {
          void onCreateProject(project)
        }}
      />

      <div className="py-4 text-center">
        <span className="text-xs text-muted-foreground/50">v{SDK_CLIENT_APP.split("/")[1]}</span>
      </div>
      </div>
    </div>
  )
}
