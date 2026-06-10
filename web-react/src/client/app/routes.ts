export const APP_ROUTE_PREFIX = "/_"
export const HOOK_NOTIFICATION_SETTINGS_HASH = "hook-notifications"

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

export function hookNotificationSettingsRoute() {
  return `${settingsRoute("general")}#${HOOK_NOTIFICATION_SETTINGS_HASH}`
}

export function isInternalAppPath(pathname: string) {
  return pathname === APP_ROUTE_PREFIX || pathname.startsWith(`${APP_ROUTE_PREFIX}/`)
}
