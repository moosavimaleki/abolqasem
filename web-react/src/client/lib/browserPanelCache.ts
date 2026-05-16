import type { LocalHttpServerInfo, ProjectQuickAction } from "../../shared/protocol"
import type { KannaSocket } from "../app/socket"

const localHttpServersCacheByProjectId = new Map<string, LocalHttpServerInfo[]>()
const localHttpServersRequestByProjectId = new Map<string, Promise<LocalHttpServerInfo[]>>()

const quickActionsCacheByProjectId = new Map<string, ProjectQuickAction[]>()
const quickActionsRequestByProjectId = new Map<string, Promise<ProjectQuickAction[]>>()

function visibleLocalHttpServers(servers: LocalHttpServerInfo[]) {
  return servers.filter((server) => server.status >= 200 && server.status < 400)
}

function localHttpServerCacheKey(projectId?: string) {
  return projectId || "__global__"
}

export function getCachedLocalHttpServers(projectId?: string) {
  return localHttpServersCacheByProjectId.get(localHttpServerCacheKey(projectId)) ?? null
}

export function refreshCachedLocalHttpServers(socket: KannaSocket, projectId?: string) {
  const cacheKey = localHttpServerCacheKey(projectId)
  const existingRequest = localHttpServersRequestByProjectId.get(cacheKey)
  if (existingRequest) return existingRequest

  const request = socket.command<LocalHttpServerInfo[]>({
    type: "browser.listLocalHttpServers",
    projectId,
  }).then((servers) => {
    const visibleServers = visibleLocalHttpServers(servers)
    localHttpServersCacheByProjectId.set(cacheKey, visibleServers)
    return visibleServers
  }).finally(() => {
    localHttpServersRequestByProjectId.delete(cacheKey)
  })

  localHttpServersRequestByProjectId.set(cacheKey, request)
  return request
}

export function removeCachedLocalHttpServer(port: number, projectId?: string) {
  const cacheKey = localHttpServerCacheKey(projectId)
  const nextServers = (localHttpServersCacheByProjectId.get(cacheKey) ?? []).filter((server) => server.port !== port)
  localHttpServersCacheByProjectId.set(cacheKey, nextServers)
  return nextServers
}

export function getCachedProjectQuickActions(projectId: string) {
  return quickActionsCacheByProjectId.get(projectId)
}

export function refreshCachedProjectQuickActions(socket: KannaSocket, projectId: string) {
  const existingRequest = quickActionsRequestByProjectId.get(projectId)
  if (existingRequest) return existingRequest

  const request = socket.command<ProjectQuickAction[]>({
    type: "project.readQuickActions",
    projectId,
  }).then((actions) => {
    quickActionsCacheByProjectId.set(projectId, actions)
    return actions
  }).finally(() => {
    quickActionsRequestByProjectId.delete(projectId)
  })

  quickActionsRequestByProjectId.set(projectId, request)
  return request
}

export function writeCachedProjectQuickActions(socket: KannaSocket, projectId: string, actions: ProjectQuickAction[]) {
  quickActionsCacheByProjectId.set(projectId, actions)
  return socket.command<ProjectQuickAction[]>({
    type: "project.writeQuickActions",
    projectId,
    quickActions: actions,
  }).then((savedActions) => {
    quickActionsCacheByProjectId.set(projectId, savedActions)
    return savedActions
  })
}
