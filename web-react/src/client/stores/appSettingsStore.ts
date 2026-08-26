import { create } from "zustand"
import type { AppSettingsPatch, AppSettingsSnapshot } from "../../shared/types"

type AppSettingsHydrationStatus = "idle" | "loading" | "ready" | "error"

interface AppSettingsStoreState {
  settings: AppSettingsSnapshot | null
  hydrationStatus: AppSettingsHydrationStatus
  setHydrationStatus: (status: AppSettingsHydrationStatus) => void
  setFromServer: (settings: AppSettingsSnapshot) => void
  applyOptimisticPatch: (patch: AppSettingsPatch) => void
}

export function mergeAppSettingsPatch(
  settings: AppSettingsSnapshot,
  patch: AppSettingsPatch
): AppSettingsSnapshot {
  return {
    ...settings,
    ...patch,
    terminal: {
      ...settings.terminal,
      ...patch.terminal,
    },
    editor: {
      ...settings.editor,
      ...patch.editor,
    },
    providerProxy: {
      ...settings.providerProxy,
      ...patch.providerProxy,
    },
    providerExecutables: {
      ...settings.providerExecutables,
      ...(patch.providerExecutables ?? {}),
    },
    commitMessageGenerator: {
      ...settings.commitMessageGenerator,
      ...patch.commitMessageGenerator,
    },
    diskManagement: {
      warningThresholdBytes: settings.diskManagement?.warningThresholdBytes ?? 2 * 1024 ** 3,
      autoCleanup: settings.diskManagement?.autoCleanup ?? false,
      ...patch.diskManagement,
    },
    providerDefaults: {
      claude: {
        ...settings.providerDefaults.claude,
        ...patch.providerDefaults?.claude,
        modelOptions: {
          ...settings.providerDefaults.claude.modelOptions,
          ...patch.providerDefaults?.claude?.modelOptions,
        },
      },
      codex: {
        ...settings.providerDefaults.codex,
        ...patch.providerDefaults?.codex,
        modelOptions: {
          ...settings.providerDefaults.codex.modelOptions,
          ...patch.providerDefaults?.codex?.modelOptions,
        },
      },
    },
    providerModelCatalog: {
      ...settings.providerModelCatalog,
      ...patch.providerModelCatalog,
      claude: {
        ...settings.providerModelCatalog.claude,
        ...patch.providerModelCatalog?.claude,
      },
      codex: {
        ...settings.providerModelCatalog.codex,
        ...patch.providerModelCatalog?.codex,
      },
    },
  }
}

export const useAppSettingsStore = create<AppSettingsStoreState>()((set) => ({
  settings: null,
  hydrationStatus: "idle",
  setHydrationStatus: (hydrationStatus) => set({ hydrationStatus }),
  setFromServer: (settings) => set({ settings, hydrationStatus: "ready" }),
  applyOptimisticPatch: (patch) =>
    set((state) => ({
      settings: state.settings ? mergeAppSettingsPatch(state.settings, patch) : state.settings,
    })),
}))
