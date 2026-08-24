import { create } from "zustand"
import {
  DEFAULT_CLAUDE_MODEL_OPTIONS,
  DEFAULT_CODEX_MODEL,
  DEFAULT_CODEX_MODEL_OPTIONS,
  normalizeClaudeContextWindow,
  normalizeClaudeModelId,
  normalizeCodexModelId,
  isClaudeReasoningEffort,
  isCodexExecutionMode,
  isCodexReasoningEffort,
  supportsClaudeMaxReasoningEffort,
  type AgentProvider,
  type ChatProviderPreferences,
  type ClaudeModelOptions,
  type CodexModelOptions,
  type DefaultProviderPreference,
  type ProviderPreference,
  type ProviderModelOptionsByProvider,
} from "../../shared/types"

export type { ChatProviderPreferences, DefaultProviderPreference, ProviderPreference }

const LAST_USED_COMPOSER_STORAGE_KEY = "abolqasem:last-used-composer"

export type ComposerState =
  | {
    provider: "claude"
    model: string
    modelOptions: ClaudeModelOptions
    planMode: boolean
  }
  | {
    provider: "codex"
    model: string
    modelOptions: CodexModelOptions
    planMode: boolean
  }

export const NEW_CHAT_COMPOSER_ID = "__new__"

type LegacyPersistedChatPreferencesState = Partial<{
  defaultProvider: string
  providerDefaults: {
    claude?: {
      model?: string
      effort?: string
      modelOptions?: Partial<ClaudeModelOptions>
      planMode?: boolean
    }
    codex?: {
      model?: string
      effort?: string
      modelOptions?: Partial<CodexModelOptions>
      planMode?: boolean
    }
  }
  composerState: PersistedComposerState
  liveProvider: AgentProvider
  livePreferences: {
    claude?: {
      model?: string
      effort?: string
      modelOptions?: Partial<ClaudeModelOptions>
      planMode?: boolean
    }
    codex?: {
      model?: string
      effort?: string
      modelOptions?: Partial<CodexModelOptions>
      planMode?: boolean
    }
  }
}>

type PersistedComposerState =
  | {
    provider: "claude"
    model?: string
    effort?: string
    modelOptions?: Partial<ClaudeModelOptions>
    planMode?: boolean
  }
  | {
    provider: "codex"
    model?: string
    effort?: string
    modelOptions?: Partial<CodexModelOptions>
    planMode?: boolean
  }

type PersistedChatPreferencesState = Pick<
  ChatPreferencesState,
  "defaultProvider" | "providerDefaults" | "chatStates" | "legacyComposerState"
> & LegacyPersistedChatPreferencesState

export function normalizeDefaultProvider(value?: string): DefaultProviderPreference {
  if (value === "claude" || value === "codex") return value
  return "last_used"
}

function normalizeSelectionMode(value?: string): "auto" | "manual" {
  return value === "manual" ? "manual" : "auto"
}

export function normalizeClaudePreference(value?: {
  model?: string
  modelMode?: string
  reasoningEffortMode?: string
  effort?: string
  modelOptions?: Partial<ClaudeModelOptions>
  planMode?: boolean
}): ProviderPreference<ClaudeModelOptions> {
  const reasoningEffort = value?.modelOptions?.reasoningEffort
  const normalizedEffort = isClaudeReasoningEffort(reasoningEffort)
    ? reasoningEffort
    : isClaudeReasoningEffort(value?.effort)
      ? value.effort
      : DEFAULT_CLAUDE_MODEL_OPTIONS.reasoningEffort
  const model = normalizeClaudeModelId(value?.model)
  const contextWindow = normalizeClaudeContextWindow(model, value?.modelOptions?.contextWindow)

  return {
    model,
    modelMode: normalizeSelectionMode(value?.modelMode),
    reasoningEffortMode: normalizeSelectionMode(value?.reasoningEffortMode),
    modelOptions: {
      reasoningEffort: !supportsClaudeMaxReasoningEffort(model) && normalizedEffort === "max" ? "high" : normalizedEffort,
      contextWindow,
    },
    planMode: Boolean(value?.planMode),
  }
}

export function normalizeCodexPreference(value?: {
  model?: string
  modelMode?: string
  reasoningEffortMode?: string
  effort?: string
  modelOptions?: Partial<CodexModelOptions>
  planMode?: boolean
}): ProviderPreference<CodexModelOptions> {
  const reasoningEffort = value?.modelOptions?.reasoningEffort
  return {
    model: normalizeCodexModelId(value?.model),
    modelMode: normalizeSelectionMode(value?.modelMode),
    reasoningEffortMode: normalizeSelectionMode(value?.reasoningEffortMode),
    modelOptions: {
      reasoningEffort: isCodexReasoningEffort(reasoningEffort)
        ? reasoningEffort
        : isCodexReasoningEffort(value?.effort)
          ? value.effort
          : DEFAULT_CODEX_MODEL_OPTIONS.reasoningEffort,
      fastMode: typeof value?.modelOptions?.fastMode === "boolean"
        ? value.modelOptions.fastMode
        : DEFAULT_CODEX_MODEL_OPTIONS.fastMode,
      ...(isCodexExecutionMode(value?.modelOptions?.executionMode)
        ? { executionMode: value.modelOptions.executionMode }
        : {}),
    },
    planMode: Boolean(value?.planMode),
  }
}

function forcePersistedCodexCompatiblePreference<T extends {
  model?: string
  effort?: string
  modelOptions?: Partial<CodexModelOptions>
  planMode?: boolean
}>(value?: T): T | undefined {
  if (!value) return value
  return {
    ...value,
    model: DEFAULT_CODEX_MODEL,
  }
}

function forcePersistedCodexCompatibleComposerState<T extends PersistedComposerState | ComposerState>(value?: T): T | undefined {
  if (!value || value.provider !== "codex") return value
  return {
    ...value,
    model: DEFAULT_CODEX_MODEL,
  }
}

function forcePersistedCodexCompatibleChatStates(
  value?: Record<string, PersistedComposerState | ComposerState>
): Record<string, PersistedComposerState | ComposerState> | undefined {
  if (!value) return value

  return Object.fromEntries(
    Object.entries(value).map(([chatId, composerState]) => [
      chatId,
      forcePersistedCodexCompatibleComposerState(composerState) ?? composerState,
    ])
  )
}

export function createDefaultProviderDefaults(): ChatProviderPreferences {
  return {
    claude: {
      model: "claude-opus-4-7",
      modelMode: "auto",
      reasoningEffortMode: "auto",
      modelOptions: { ...DEFAULT_CLAUDE_MODEL_OPTIONS },
      planMode: false,
    },
    codex: {
      model: DEFAULT_CODEX_MODEL,
      modelMode: "auto",
      reasoningEffortMode: "auto",
      modelOptions: { ...DEFAULT_CODEX_MODEL_OPTIONS },
      planMode: false,
    },
  }
}

export function normalizeProviderDefaults(value?: {
  claude?: {
    model?: string
    modelMode?: string
    reasoningEffortMode?: string
    effort?: string
    modelOptions?: Partial<ClaudeModelOptions>
    planMode?: boolean
  }
  codex?: {
    model?: string
    modelMode?: string
    reasoningEffortMode?: string
    effort?: string
    modelOptions?: Partial<CodexModelOptions>
    planMode?: boolean
  }
}): ChatProviderPreferences {
  return {
    claude: normalizeClaudePreference(value?.claude),
    codex: normalizeCodexPreference(value?.codex),
  }
}

function normalizeProviderPreference<TProvider extends AgentProvider>(
  provider: TProvider,
  value: Partial<ProviderPreference<ProviderModelOptionsByProvider[TProvider]>> & { effort?: string }
): ProviderPreference<ProviderModelOptionsByProvider[TProvider]> {
  switch (provider) {
    case "claude":
      return normalizeClaudePreference(value as Partial<ProviderPreference<ClaudeModelOptions>> & { effort?: string }) as ProviderPreference<ProviderModelOptionsByProvider[TProvider]>
    case "codex":
    default:
      return normalizeCodexPreference(value as Partial<ProviderPreference<CodexModelOptions>> & { effort?: string }) as ProviderPreference<ProviderModelOptionsByProvider[TProvider]>
  }
}

function logChatPreferences(message: string, details?: unknown) {
  if (details === undefined) {
    console.info(`[chat-preferences] ${message}`)
    return
  }

  console.info(`[chat-preferences] ${message}`, details)
}

function composerFromProviderDefaults(
  provider: AgentProvider,
  providerDefaults: ChatProviderPreferences
): ComposerState {
  switch (provider) {
    case "claude": {
      const preference = providerDefaults.claude
      return {
        provider: "claude",
        model: preference.model,
        modelOptions: { ...preference.modelOptions },
        planMode: preference.planMode,
      }
    }
    case "codex":
    default: {
      const preference = providerDefaults.codex
      return {
        provider: "codex",
        model: preference.model,
        modelOptions: { ...preference.modelOptions },
        planMode: preference.planMode,
      }
    }
  }
}

function cloneComposerState(state: ComposerState): ComposerState {
  return {
    ...state,
    modelOptions: { ...state.modelOptions },
  } as ComposerState
}

function sameComposerState(left: ComposerState | undefined, right: ComposerState): boolean {
  if (!left || left.provider !== right.provider) return false
  if (left.model !== right.model || left.planMode !== right.planMode) return false

  if (left.provider === "claude" && right.provider === "claude") {
    return left.modelOptions.reasoningEffort === right.modelOptions.reasoningEffort
      && left.modelOptions.contextWindow === right.modelOptions.contextWindow
  }

  if (left.provider === "codex" && right.provider === "codex") {
    return left.modelOptions.reasoningEffort === right.modelOptions.reasoningEffort
      && left.modelOptions.fastMode === right.modelOptions.fastMode
      && (left.modelOptions.executionMode ?? DEFAULT_CODEX_MODEL_OPTIONS.executionMode)
        === (right.modelOptions.executionMode ?? DEFAULT_CODEX_MODEL_OPTIONS.executionMode)
  }

  return false
}

function normalizeComposerState(
  value: PersistedComposerState | undefined,
  providerDefaults: ChatProviderPreferences,
  legacyLiveProvider?: AgentProvider,
  legacyLivePreferences?: LegacyPersistedChatPreferencesState["livePreferences"]
): ComposerState {
  if (value?.provider === "claude") {
    const preference = normalizeClaudePreference(value)
    return {
      provider: "claude",
      model: preference.model,
      modelOptions: preference.modelOptions,
      planMode: preference.planMode,
    }
  }

  if (value?.provider === "codex") {
    const preference = normalizeCodexPreference(value)
    return {
      provider: "codex",
      model: preference.model,
      modelOptions: preference.modelOptions,
      planMode: preference.planMode,
    }
  }

  if (legacyLiveProvider === "claude") {
    const preference = normalizeClaudePreference(legacyLivePreferences?.claude)
    return {
      provider: "claude",
      model: preference.model,
      modelOptions: preference.modelOptions,
      planMode: preference.planMode,
    }
  }

  if (legacyLiveProvider === "codex") {
    const preference = normalizeCodexPreference(legacyLivePreferences?.codex)
    return {
      provider: "codex",
      model: preference.model,
      modelOptions: preference.modelOptions,
      planMode: preference.planMode,
    }
  }

  return composerFromProviderDefaults("claude", providerDefaults)
}

function normalizePersistedComposerState(
  value: PersistedComposerState | ComposerState | undefined,
  providerDefaults: ChatProviderPreferences
): ComposerState | null {
  if (!value) return null
  return normalizeComposerState(value, providerDefaults)
}

function readStoredLastUsedComposerState(providerDefaults: ChatProviderPreferences): ComposerState | null {
  if (typeof window === "undefined") return null

  try {
    const raw = window.localStorage.getItem(LAST_USED_COMPOSER_STORAGE_KEY)
    if (!raw) return null
    return normalizePersistedComposerState(JSON.parse(raw) as PersistedComposerState, providerDefaults)
  } catch {
    return null
  }
}

function writeStoredLastUsedComposerState(composerState: ComposerState) {
  if (typeof window === "undefined") return

  try {
    window.localStorage.setItem(LAST_USED_COMPOSER_STORAGE_KEY, JSON.stringify(cloneComposerState(composerState)))
  } catch {
    // Last-used state still works for the current tab when localStorage is unavailable.
  }
}

function normalizeChatStates(
  value: Record<string, PersistedComposerState | ComposerState> | undefined,
  providerDefaults: ChatProviderPreferences
): Record<string, ComposerState> {
  if (!value) return {}

  return Object.fromEntries(
    Object.entries(value).map(([chatId, composerState]) => [
      chatId,
      normalizeComposerState(composerState, providerDefaults),
    ])
  )
}

function createComposerStateForNewChat(args: {
  defaultProvider: DefaultProviderPreference
  providerDefaults: ChatProviderPreferences
  sourceState?: ComposerState | null
  legacyComposerState?: ComposerState | null
}): ComposerState {
  if (args.defaultProvider === "last_used") {
    if (args.sourceState) {
      return cloneComposerState(args.sourceState)
    }

    if (args.legacyComposerState) {
      return cloneComposerState(args.legacyComposerState)
    }

    return composerFromProviderDefaults("claude", args.providerDefaults)
  }

  return composerFromProviderDefaults(args.defaultProvider, args.providerDefaults)
}

function getStoredComposerState(
  state: Pick<ChatPreferencesState, "chatStates" | "defaultProvider" | "providerDefaults" | "legacyComposerState">,
  chatId: string
): ComposerState {
  const existingState = state.chatStates[chatId]
  if (existingState) {
    return existingState
  }

  return createComposerStateForNewChat({
    defaultProvider: state.defaultProvider,
    providerDefaults: state.providerDefaults,
    legacyComposerState: state.legacyComposerState,
  })
}

function withChatComposerStateAndLastUsed(
  state: Pick<ChatPreferencesState, "chatStates" | "defaultProvider" | "providerDefaults" | "legacyComposerState">,
  chatId: string,
  transform: (composerState: ComposerState) => ComposerState
) {
  const currentComposerState = getStoredComposerState(state, chatId)
  const nextComposerState = transform(currentComposerState)
  return withLastUsedComposerState(state, nextComposerState, {
    ...state.chatStates,
    [chatId]: nextComposerState,
  })
}

function withLastUsedComposerState(
  state: Pick<ChatPreferencesState, "chatStates" | "defaultProvider" | "providerDefaults" | "legacyComposerState">,
  composerState: ComposerState,
  chatStates: Record<string, ComposerState> = state.chatStates
) {
  const lastUsedComposerState = cloneComposerState(composerState)
  writeStoredLastUsedComposerState(lastUsedComposerState)
  const currentNewChatState = state.chatStates[NEW_CHAT_COMPOSER_ID]
  const oldNewChatFallback = createComposerStateForNewChat({
    defaultProvider: state.defaultProvider,
    providerDefaults: state.providerDefaults,
    legacyComposerState: state.legacyComposerState,
  })
  const nextNewChatFallback = createComposerStateForNewChat({
    defaultProvider: state.defaultProvider,
    providerDefaults: state.providerDefaults,
    legacyComposerState: lastUsedComposerState,
  })
  const shouldRefreshNewChatState = state.defaultProvider === "last_used"
    && (!currentNewChatState || sameComposerState(currentNewChatState, oldNewChatFallback))

  return {
    legacyComposerState: lastUsedComposerState,
    chatStates: shouldRefreshNewChatState
      ? {
        ...chatStates,
        [NEW_CHAT_COMPOSER_ID]: nextNewChatFallback,
      }
      : chatStates,
  }
}

function withDefaultProvider(
  state: Pick<ChatPreferencesState, "chatStates" | "defaultProvider" | "providerDefaults" | "legacyComposerState">,
  defaultProvider: DefaultProviderPreference,
  providerDefaults: ChatProviderPreferences = state.providerDefaults
) {
  const oldNewChatFallback = createComposerStateForNewChat({
    defaultProvider: state.defaultProvider,
    providerDefaults: state.providerDefaults,
    legacyComposerState: state.legacyComposerState,
  })
  const nextNewChatFallback = createComposerStateForNewChat({
    defaultProvider,
    providerDefaults,
    legacyComposerState: state.legacyComposerState,
  })
  const chatStates = Object.fromEntries(
    Object.entries(state.chatStates).map(([chatId, composerState]) => [
      chatId,
      sameComposerState(composerState, oldNewChatFallback) ? nextNewChatFallback : composerState,
    ])
  )

  return {
    defaultProvider,
    providerDefaults,
    chatStates,
  }
}

interface ChatPreferencesState {
  defaultProvider: DefaultProviderPreference
  providerDefaults: ChatProviderPreferences
  chatStates: Record<string, ComposerState>
  legacyComposerState: ComposerState | null
  setDefaultProvider: (provider: DefaultProviderPreference) => void
  syncProviderDefaults: (defaultProvider: DefaultProviderPreference, providerDefaults: ChatProviderPreferences) => void
  setProviderDefaultModel: (provider: AgentProvider, model: string) => void
  setProviderDefaultModelOptions: <TProvider extends AgentProvider>(
    provider: TProvider,
    modelOptions: Partial<ProviderModelOptionsByProvider[TProvider]>
  ) => void
  setProviderDefaultPlanMode: (provider: AgentProvider, planMode: boolean) => void
  getComposerState: (chatId: string) => ComposerState
  initializeComposerForChat: (chatId: string, options?: { sourceState?: ComposerState | null }) => void
  setComposerState: (chatId: string, composerState: ComposerState) => void
  setChatComposerProvider: (chatId: string, provider: AgentProvider) => void
  setChatComposerModel: (chatId: string, model: string) => void
  setChatComposerModelOptions: (
    chatId: string,
    modelOptions: Partial<ClaudeModelOptions> | Partial<CodexModelOptions>
  ) => void
  setChatComposerPlanMode: (chatId: string, planMode: boolean) => void
  resetChatComposerFromProvider: (chatId: string, provider: AgentProvider) => void
}

export function migrateChatPreferencesState(
  persistedState: Partial<PersistedChatPreferencesState> | undefined
): Pick<ChatPreferencesState, "defaultProvider" | "providerDefaults" | "chatStates" | "legacyComposerState"> {
  const providerDefaults = normalizeProviderDefaults({
    ...persistedState?.providerDefaults,
    codex: forcePersistedCodexCompatiblePreference(persistedState?.providerDefaults?.codex),
  })
  const legacyComposerState = normalizePersistedComposerState(
    forcePersistedCodexCompatibleComposerState(persistedState?.legacyComposerState ?? persistedState?.composerState),
    providerDefaults
  )
  const legacyLiveComposerState = persistedState?.liveProvider
    ? normalizeComposerState(
      undefined,
      providerDefaults,
      persistedState.liveProvider,
      {
        ...persistedState?.livePreferences,
        codex: forcePersistedCodexCompatiblePreference(persistedState?.livePreferences?.codex),
      }
    )
    : null

  return {
    defaultProvider: normalizeDefaultProvider(persistedState?.defaultProvider),
    providerDefaults,
    chatStates: normalizeChatStates(forcePersistedCodexCompatibleChatStates(persistedState?.chatStates), providerDefaults),
    legacyComposerState: legacyComposerState ?? legacyLiveComposerState,
  }
}

export const useChatPreferencesStore = create<ChatPreferencesState>()(
  (set, get) => {
    const initialProviderDefaults = createDefaultProviderDefaults()

    return {
      defaultProvider: "last_used",
      providerDefaults: initialProviderDefaults,
      chatStates: {},
      legacyComposerState: readStoredLastUsedComposerState(initialProviderDefaults),
      setDefaultProvider: (defaultProvider) => set((state) => withDefaultProvider(state, defaultProvider)),
      syncProviderDefaults: (defaultProvider, providerDefaults) =>
        set((state) => withDefaultProvider(state, defaultProvider, providerDefaults)),
      setProviderDefaultModel: (provider, model) =>
        set((state) => ({
          providerDefaults: {
            ...state.providerDefaults,
            [provider]: normalizeProviderPreference(provider, {
              ...state.providerDefaults[provider],
              model,
              modelMode: "manual",
            }),
          },
        })),
      setProviderDefaultModelOptions: (provider, modelOptions) =>
        set((state) => {
          if (provider === "claude") {
            return {
              providerDefaults: {
                ...state.providerDefaults,
                claude: normalizeClaudePreference({
                  ...state.providerDefaults.claude,
                  reasoningEffortMode: "manual",
                  modelOptions: {
                    ...state.providerDefaults.claude.modelOptions,
                    ...modelOptions as Partial<ClaudeModelOptions>,
                  },
                }),
              },
            }
          }
          if (provider === "codex") {
            return {
              providerDefaults: {
                ...state.providerDefaults,
                codex: normalizeCodexPreference({
                  ...state.providerDefaults.codex,
                  reasoningEffortMode: "manual",
                  modelOptions: {
                    ...state.providerDefaults.codex.modelOptions,
                    ...modelOptions as Partial<CodexModelOptions>,
                  },
                }),
              },
            }
          }
          return state
        }),
      setProviderDefaultPlanMode: (provider, planMode) =>
        set((state) => ({
          providerDefaults: {
            ...state.providerDefaults,
            [provider]: {
              ...state.providerDefaults[provider],
              planMode,
            },
          },
        })),
      getComposerState: (chatId) => cloneComposerState(getStoredComposerState(get(), chatId)),
      initializeComposerForChat: (chatId, options) =>
        set((state) => {
          if (state.chatStates[chatId]) {
            return state
          }

          const composerState = createComposerStateForNewChat({
            defaultProvider: state.defaultProvider,
            providerDefaults: state.providerDefaults,
            sourceState: options?.sourceState,
            legacyComposerState: state.legacyComposerState,
          })

          logChatPreferences("initializeComposerForChat", { chatId, composerState })

          return {
            chatStates: {
              ...state.chatStates,
              [chatId]: composerState,
            },
          }
        }),
      setComposerState: (chatId, composerState) =>
        set((state) => {
          const preference = normalizeProviderPreference(composerState.provider, composerState)
          const normalizedComposerState = {
            provider: composerState.provider,
            model: preference.model,
            modelOptions: preference.modelOptions,
            planMode: composerState.planMode,
          } as ComposerState
          return withLastUsedComposerState(state, normalizedComposerState, {
            ...state.chatStates,
            [chatId]: normalizedComposerState,
          })
        }),
      setChatComposerProvider: (chatId, provider) =>
        set((state) => withChatComposerStateAndLastUsed(state, chatId, () => composerFromProviderDefaults(provider, state.providerDefaults))),
      setChatComposerModel: (chatId, model) =>
        set((state) => withChatComposerStateAndLastUsed(state, chatId, (composerState) => {
          const preference = normalizeProviderPreference(composerState.provider, { ...composerState, model })
          return {
            provider: composerState.provider,
            model: preference.model,
            modelOptions: preference.modelOptions,
            planMode: composerState.planMode,
          } as ComposerState
        })),
      setChatComposerModelOptions: (chatId, modelOptions) =>
        set((state) => withChatComposerStateAndLastUsed(state, chatId, (composerState) => {
          if (composerState.provider === "claude") {
            const preference = normalizeClaudePreference({
              ...composerState,
              modelOptions: {
                ...composerState.modelOptions,
                ...modelOptions as Partial<ClaudeModelOptions>,
              },
            })
            return {
              provider: "claude",
              model: composerState.model,
              modelOptions: preference.modelOptions,
              planMode: composerState.planMode,
            }
          }
          if (composerState.provider === "codex") {
            const preference = normalizeCodexPreference({
              ...composerState,
              modelOptions: {
                ...composerState.modelOptions,
                ...modelOptions as Partial<CodexModelOptions>,
              },
            })
            return {
              provider: "codex",
              model: composerState.model,
              modelOptions: preference.modelOptions,
              planMode: composerState.planMode,
            }
          }
          return composerState
        })),
      setChatComposerPlanMode: (chatId, planMode) =>
        set((state) => withChatComposerStateAndLastUsed(state, chatId, (composerState) => ({
          ...composerState,
          planMode,
        }))),
      resetChatComposerFromProvider: (chatId, provider) =>
        set((state) => withChatComposerStateAndLastUsed(state, chatId, () => composerFromProviderDefaults(provider, state.providerDefaults))),
    }
  }
)
