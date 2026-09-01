import { create } from "zustand";
import type { AppSettingsPatch, AppSettingsSnapshot } from "../../shared/types";
import { DEFAULT_OPENCODE_MODEL } from "../../shared/types";

type AppSettingsHydrationStatus = "idle" | "loading" | "ready" | "error";

const defaultOpenCodePreference = {
  model: DEFAULT_OPENCODE_MODEL,
  modelMode: "auto" as const,
  reasoningEffortMode: "auto" as const,
  modelOptions: {},
  planMode: false,
};

const defaultCodexSessionMonitor = {
  enabled: false,
  intervalSeconds: 21600,
  dryRun: true,
  chromeRoot: "",
};

function normalizeAppSettingsSnapshot(
  settings: AppSettingsSnapshot,
): AppSettingsSnapshot {
  return {
    ...settings,
    codexBackend: {
      ...settings.codexBackend,
      sessionMonitor: {
        ...defaultCodexSessionMonitor,
        ...settings.codexBackend?.sessionMonitor,
      },
    },
  };
}

interface AppSettingsStoreState {
  settings: AppSettingsSnapshot | null;
  hydrationStatus: AppSettingsHydrationStatus;
  setHydrationStatus: (status: AppSettingsHydrationStatus) => void;
  setFromServer: (settings: AppSettingsSnapshot) => void;
  applyOptimisticPatch: (patch: AppSettingsPatch) => void;
}

export function mergeAppSettingsPatch(
  settings: AppSettingsSnapshot,
  patch: AppSettingsPatch,
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
    codexBackend: {
      ...settings.codexBackend,
      ...patch.codexBackend,
      maintenance: {
        ...settings.codexBackend.maintenance,
        ...patch.codexBackend?.maintenance,
      },
      sessionMonitor: {
        // Old servers did not include this nested object in their snapshot.
        // Keep a local settings edit from crashing during a rolling upgrade.
        ...defaultCodexSessionMonitor,
        ...settings.codexBackend.sessionMonitor,
        ...patch.codexBackend?.sessionMonitor,
      },
      customProviders: {
        ...settings.codexBackend.customProviders,
        ...(patch.codexBackend?.customProviders ?? {}),
      },
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
      warningThresholdBytes:
        settings.diskManagement?.warningThresholdBytes ?? 2 * 1024 ** 3,
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
      opencode: {
        ...defaultOpenCodePreference,
        ...settings.providerDefaults.opencode,
        ...patch.providerDefaults?.opencode,
        modelOptions: {
          ...(settings.providerDefaults.opencode?.modelOptions ?? {}),
          ...patch.providerDefaults?.opencode?.modelOptions,
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
      opencode: {
        ...(settings.providerModelCatalog?.opencode ?? {
          catalogModels: [],
          discoveredModels: [],
          customModels: [],
        }),
        ...patch.providerModelCatalog?.opencode,
      },
    },
  };
}

export const useAppSettingsStore = create<AppSettingsStoreState>()((set) => ({
  settings: null,
  hydrationStatus: "idle",
  setHydrationStatus: (hydrationStatus) => set({ hydrationStatus }),
  setFromServer: (settings) =>
    set({
      settings: normalizeAppSettingsSnapshot(settings),
      hydrationStatus: "ready",
    }),
  applyOptimisticPatch: (patch) =>
    set((state) => ({
      settings: state.settings
        ? mergeAppSettingsPatch(state.settings, patch)
        : state.settings,
    })),
}));
