export const APP_ROUTE_PREFIX = "/_"

export function appRoute(path: string) {
  const normalized = path.startsWith("/") ? path : `/${path}`
  return `${APP_ROUTE_PREFIX}${normalized}`
}

export function chatRoute(chatId: string) {
  return appRoute(`/chat/${encodeURIComponent(chatId)}`)
}

export function settingsRoute(sectionId = "general") {
  return appRoute(`/settings/${encodeURIComponent(sectionId)}`)
}

export function isInternalAppPath(pathname: string) {
  return pathname === APP_ROUTE_PREFIX || pathname.startsWith(`${APP_ROUTE_PREFIX}/`)
}
