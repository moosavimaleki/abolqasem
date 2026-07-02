import { memo, useEffect, useMemo, useState } from "react"
import type { TranscriptIndexItem } from "../../../shared/types"
import { cn } from "../../lib/utils"

const DEFAULT_PREVIEW_DELAY_MS = 650

export type MessageIndexItem = TranscriptIndexItem & {
  loaded: boolean
  isSearchMatch?: boolean
}

export interface ConversationMinimapProps {
  items: readonly MessageIndexItem[]
  loading?: boolean
  onScrollToMessage: (item: MessageIndexItem) => void | Promise<void>
  side?: "left" | "right"
  className?: string
  topOffsetPx?: number
  bottomOffsetPx?: number
  ariaLabel?: string
  previewDelayMs?: number
}

interface PositionedItem extends MessageIndexItem {
  topPercent: number
}

export function buildPositionedItems(items: readonly MessageIndexItem[]): PositionedItem[] {
  const users = [...items]
    .filter((item) => item.role === "user")
    .sort((left, right) => left.sequence - right.sequence)
  const count = Math.max(1, users.length)

  return users.map((item, index) => ({
    ...item,
    topPercent: ((index + 0.5) / count) * 100,
  }))
}

function buildPositionedItemMap(items: readonly PositionedItem[]) {
  const map = new Map<string, PositionedItem>()
  for (const item of items) {
    map.set(item.id, item)
  }
  return map
}

function minimapItemLabel(item: MessageIndexItem) {
  return `User message ${item.sequence + 1}`
}

export const ConversationMinimap = memo(function ConversationMinimap({
  items,
  loading = false,
  onScrollToMessage,
  side = "right",
  className,
  topOffsetPx = 0,
  bottomOffsetPx = 0,
  ariaLabel = "Conversation minimap",
  previewDelayMs = DEFAULT_PREVIEW_DELAY_MS,
}: ConversationMinimapProps) {
  const [pendingMessageId, setPendingMessageId] = useState<string | null>(null)
  const [hoveredMessageId, setHoveredMessageId] = useState<string | null>(null)
  const [previewMessageId, setPreviewMessageId] = useState<string | null>(null)
  const [flashedMessageId, setFlashedMessageId] = useState<string | null>(null)

  const positionedItems = useMemo(() => buildPositionedItems(items), [items])
  const itemMap = useMemo(() => buildPositionedItemMap(positionedItems), [positionedItems])
  const previewItem = previewMessageId ? itemMap.get(previewMessageId) ?? null : null

  useEffect(() => {
    if (!hoveredMessageId) {
      setPreviewMessageId(null)
      return
    }

    const hoveredItem = itemMap.get(hoveredMessageId)
    if (!hoveredItem?.preview?.trim()) {
      setPreviewMessageId(null)
      return
    }

    const timeoutId = window.setTimeout(() => {
      setPreviewMessageId(hoveredMessageId)
    }, previewDelayMs)
    return () => window.clearTimeout(timeoutId)
  }, [hoveredMessageId, itemMap, previewDelayMs])

  useEffect(() => {
    if (!flashedMessageId) return
    const timeoutId = window.setTimeout(() => {
      setFlashedMessageId((current) => current === flashedMessageId ? null : current)
    }, 1400)
    return () => window.clearTimeout(timeoutId)
  }, [flashedMessageId])

  if (loading) {
    return (
      <div
        className={cn("pointer-events-none absolute z-20 hidden min-[940px]:block", className)}
        style={{
          top: topOffsetPx,
          bottom: bottomOffsetPx,
          left: side === "left" ? 8 : undefined,
          right: side === "right" ? 8 : undefined,
        }}
        aria-label={ariaLabel}
        aria-busy="true"
        data-side={side}
      >
        <div className="pointer-events-auto relative h-full w-11 overflow-hidden rounded-[22px] border border-white/8 bg-white/[0.07] shadow-[0_18px_44px_rgba(3,8,20,0.28)] backdrop-blur-md">
          <div
            className="absolute inset-0 -translate-x-full bg-gradient-to-r from-transparent via-white/18 to-transparent animate-[abolqasem-minimap-skeleton-shine_900ms_ease-in-out_infinite]"
            aria-hidden="true"
          />
        </div>
      </div>
    )
  }

  if (positionedItems.length === 0) {
    return null
  }

  async function handleMessageSelect(item: MessageIndexItem) {
    setPendingMessageId(item.id)
    try {
      await onScrollToMessage(item)
      setFlashedMessageId(item.id)
    } finally {
      setPendingMessageId((current) => current === item.id ? null : current)
    }
  }

  return (
    <div
      className={cn("pointer-events-none absolute z-20 hidden min-[940px]:block", className)}
      style={{
        top: topOffsetPx,
        bottom: bottomOffsetPx,
        left: side === "left" ? 8 : undefined,
        right: side === "right" ? 8 : undefined,
      }}
      aria-label={ariaLabel}
      data-side={side}
    >
      <div className="pointer-events-auto relative h-full w-11 rounded-[22px] bg-black/18 shadow-[0_18px_44px_rgba(3,8,20,0.28)] backdrop-blur-md">
        <div className="absolute inset-y-3 left-1/2 w-[24px] -translate-x-1/2">
          {positionedItems.map((item) => {
            const isPending = item.id === pendingMessageId
            const isFlashed = item.id === flashedMessageId

            return (
              <button
                key={item.id}
                type="button"
                className="absolute flex h-3 w-[26px] -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-transparent p-0 outline-none"
                style={{
                  left: "50%",
                  top: `${item.topPercent}%`,
                }}
                aria-label={minimapItemLabel(item)}
                onClick={() => void handleMessageSelect(item)}
                onMouseEnter={() => setHoveredMessageId(item.id)}
                onMouseLeave={() => {
                  setHoveredMessageId((current) => current === item.id ? null : current)
                  setPreviewMessageId((current) => current === item.id ? null : current)
                }}
              >
                <span
                  className={cn(
                    "block h-[2px] w-[20px] rounded-full bg-white/48",
                    isPending && "animate-pulse",
                    isFlashed && "shadow-[0_0_10px_rgba(255,255,255,0.26)]",
                  )}
                  aria-hidden="true"
                />
              </button>
            )
          })}
        </div>

        {previewItem?.preview ? (
          <div
            className={cn(
              "pointer-events-none absolute z-10 w-52 -translate-y-1/2 rounded-2xl bg-black/78 px-3 py-2 text-[11px] leading-5 text-white/82 shadow-[0_16px_40px_rgba(2,6,23,0.44)] backdrop-blur-md",
              side === "left" ? "left-full ml-2" : "right-full mr-2",
            )}
            style={{ top: `${previewItem.topPercent}%` }}
            dir="auto"
          >
            {previewItem.preview}
          </div>
        ) : null}
      </div>
    </div>
  )
})
