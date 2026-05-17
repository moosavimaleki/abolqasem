import type { FilePreviewResponse } from "../file-preview/FilePreviewPanel"

export type ProjectFileType = "file" | "directory"

export interface ProjectFileEntry {
  name: string
  path: string
  type: ProjectFileType
  size?: number
  modifiedAt?: string
  mimeType?: string
  language?: string
  hasChildren?: boolean
}

export interface ProjectFileListResponse {
  projectId: string
  path: string
  entries: ProjectFileEntry[]
  truncated: boolean
  limit: number
}

const TREE_LIMIT = 600
const SEARCH_LIMIT = 120
const PROJECT_FILE_CACHE_TTL_MS = 10_000

type CacheEntry<T> = {
  expiresAt: number
  value: T
}

const treeCache = new Map<string, CacheEntry<ProjectFileListResponse>>()
const searchCache = new Map<string, CacheEntry<ProjectFileListResponse>>()
const previewCache = new Map<string, CacheEntry<FilePreviewResponse>>()

function cacheKey(parts: readonly string[]) {
  return parts.join("\u0000")
}

function readCache<T>(cache: Map<string, CacheEntry<T>>, key: string): T | null {
  const entry = cache.get(key)
  if (!entry) return null
  if (Date.now() > entry.expiresAt) {
    cache.delete(key)
    return null
  }
  return entry.value
}

function writeCache<T>(cache: Map<string, CacheEntry<T>>, key: string, value: T) {
  cache.set(key, {
    expiresAt: Date.now() + PROJECT_FILE_CACHE_TTL_MS,
    value,
  })
}

async function readJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, {
    signal,
    headers: { Accept: "application/json" },
    cache: "no-store",
  })
  if (!response.ok) throw new Error(await response.text() || `Request failed with ${response.status}`)
  return response.json() as Promise<T>
}

export function invalidateProjectFiles(projectId: string) {
  const prefix = `${projectId}\u0000`
  for (const cache of [treeCache, searchCache, previewCache]) {
    for (const key of cache.keys()) {
      if (key.startsWith(prefix)) cache.delete(key)
    }
  }
}

export function fileTreeURL(projectId: string, path: string, options: { refresh?: boolean } = {}) {
  const params = new URLSearchParams({ limit: String(TREE_LIMIT) })
  if (path) params.set("path", path)
  if (options.refresh) params.set("refresh", "1")
  return `/api/projects/${encodeURIComponent(projectId)}/files/tree?${params.toString()}`
}

export function fileSearchURL(projectId: string, query: string) {
  const params = new URLSearchParams({ q: query, limit: String(SEARCH_LIMIT) })
  return `/api/projects/${encodeURIComponent(projectId)}/files/search?${params.toString()}`
}

export function projectFilePreviewURL(projectId: string, path: string) {
  const params = new URLSearchParams({ path, full: "1" })
  return `/api/projects/${encodeURIComponent(projectId)}/files/preview?${params.toString()}`
}

export async function readProjectFileTree(
  projectId: string,
  path: string,
  options: { signal?: AbortSignal; force?: boolean } = {},
) {
  const key = cacheKey([projectId, path])
  if (!options.force) {
    const cached = readCache(treeCache, key)
    if (cached) return cached
  }
  const payload = await readJSON<ProjectFileListResponse>(fileTreeURL(projectId, path, { refresh: options.force }), options.signal)
  writeCache(treeCache, key, payload)
  return payload
}

export async function searchProjectFiles(
  projectId: string,
  query: string,
  options: { signal?: AbortSignal } = {},
) {
  const normalizedQuery = query.trim()
  const key = cacheKey([projectId, normalizedQuery.toLowerCase()])
  const cached = readCache(searchCache, key)
  if (cached) return cached
  const payload = await readJSON<ProjectFileListResponse>(fileSearchURL(projectId, normalizedQuery), options.signal)
  writeCache(searchCache, key, payload)
  return payload
}

export async function readProjectFilePreview(
  projectId: string,
  path: string,
  options: { signal?: AbortSignal } = {},
) {
  const key = cacheKey([projectId, path])
  const cached = readCache(previewCache, key)
  if (cached) return cached
  const payload = await readJSON<FilePreviewResponse>(projectFilePreviewURL(projectId, path), options.signal)
  writeCache(previewCache, key, payload)
  return payload
}
