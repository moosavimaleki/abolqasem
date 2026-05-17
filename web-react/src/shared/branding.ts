export const APP_NAME = "Abolqasem"
export const CLI_COMMAND = "ai-agent-manager"
export const LOCAL_UI_URL = "http://127.0.0.1:9090/"
export const DATA_ROOT_NAME = ".abolqasem"
export const DEV_DATA_ROOT_NAME = ".abolqasem-dev"
export const PACKAGE_NAME = "abolqasem-code"
export const RUNTIME_PROFILE_ENV_VAR = "ABOLQASEM_RUNTIME_PROFILE"
// Read version from package.json — JSON import works in both Bun and Vite
import pkg from "../../package.json"
export const APP_VERSION = pkg.version
export const SDK_CLIENT_APP = `abolqasem/${pkg.version}`
export const LOG_PREFIX = "[abolqasem]"
export const DEFAULT_NEW_PROJECT_ROOT = `~/${APP_NAME}`

export type RuntimeProfile = "dev" | "prod"

type RuntimeEnv = Record<string, string | undefined> | undefined

function getRuntimeEnv(): RuntimeEnv {
  const candidate = globalThis as typeof globalThis & {
    process?: {
      env?: Record<string, string | undefined>
    }
  }
  return candidate.process?.env
}

export function getRuntimeProfile(env: RuntimeEnv = getRuntimeEnv()): RuntimeProfile {
  return env?.[RUNTIME_PROFILE_ENV_VAR]?.trim().toLowerCase() === "dev" ? "dev" : "prod"
}

export function getDataRootName(env: RuntimeEnv = getRuntimeEnv()) {
  return getRuntimeProfile(env) === "dev" ? DEV_DATA_ROOT_NAME : DATA_ROOT_NAME
}

export function getDataRootDir(homeDir: string, env: RuntimeEnv = getRuntimeEnv()) {
  return `${homeDir}/${getDataRootName(env)}`
}

export function getDataRootDirDisplay(env: RuntimeEnv = getRuntimeEnv()) {
  return `~/${getDataRootName(env)}`
}

export function getDataDir(homeDir: string, env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDir(homeDir, env)}/data`
}

export function getDataDirDisplay(env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDirDisplay(env)}/data`
}

export function getSettingsFilePath(homeDir: string, env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataDir(homeDir, env)}/settings.json`
}

export function getSettingsFilePathDisplay(env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataDirDisplay(env)}/settings.json`
}

export function getKeybindingsFilePath(homeDir: string, env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDir(homeDir, env)}/keybindings.json`
}

export function getKeybindingsFilePathDisplay(env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDirDisplay(env)}/keybindings.json`
}

export function getLlmProviderFilePath(homeDir: string, env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDir(homeDir, env)}/llm-provider.json`
}

export function getLlmProviderFilePathDisplay(env: RuntimeEnv = getRuntimeEnv()) {
  return `${getDataRootDirDisplay(env)}/llm-provider.json`
}

export function getCliInvocation(arg?: string) {
  return arg ? `${CLI_COMMAND} ${arg}` : CLI_COMMAND
}
