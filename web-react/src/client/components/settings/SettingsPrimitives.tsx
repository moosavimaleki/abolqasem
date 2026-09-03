import type { ReactNode } from "react";
import { Info } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "../ui/tooltip";
import { cn } from "../../lib/utils";

export function SettingsInfoHint({
  label,
  children,
  direction,
}: {
  label: string;
  children: ReactNode;
  direction: "rtl" | "ltr";
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Info className="size-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent
        dir={direction}
        side="bottom"
        className="max-w-80 whitespace-normal text-start leading-relaxed"
      >
        {children}
      </TooltipContent>
    </Tooltip>
  );
}

export function SettingsRow({
  title,
  description,
  children,
  bordered = true,
  alignStart = false,
  wide = false,
  anchorId,
}: {
  title: ReactNode;
  description: ReactNode;
  children: ReactNode;
  bordered?: boolean;
  alignStart?: boolean;
  wide?: boolean;
  anchorId?: string;
}) {
  return (
    <div
      id={anchorId}
      className={cn(
        anchorId ? "scroll-mt-24" : undefined,
        bordered ? "border-t border-border" : undefined,
      )}
    >
      <div
        className={cn(
          "flex flex-col gap-4 py-5",
          wide ? "md:gap-3" : "md:flex-row md:justify-between md:gap-8",
          wide
            ? "md:items-stretch"
            : alignStart
              ? "md:items-start"
              : "md:items-center",
        )}
      >
        <div className={cn("min-w-0", wide ? "w-full max-w-none" : "max-w-xl")}>
          <div className="text-sm font-medium text-foreground">{title}</div>
          <div className="mt-1 text-[13px] text-muted-foreground">
            {description}
          </div>
        </div>
        <div
          className={cn(
            "flex items-center justify-start md:shrink-0 md:justify-end",
            wide ? "w-full" : undefined,
          )}
        >
          {children}
        </div>
      </div>
    </div>
  );
}
