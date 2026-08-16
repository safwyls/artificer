import { type Server } from "../../lib/api";
import { cn } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";

/**
 * One server on the icon rail: a rune-ring coin with the server's initial
 * in the display face. The active server's ring lights rune — the reserved
 * live-state color — and everything else stays brass-on-dark.
 */
export function WkServerRune({
  server,
  active,
  onClick,
}: {
  server: Server;
  active: boolean;
  onClick: () => void;
}) {
  const initial = (server.name.trim()[0] ?? "?").toUpperCase();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          onClick={onClick}
          aria-current={active ? "true" : undefined}
          className={cn(
            "flex h-11 w-11 items-center justify-center rounded-full border-2 font-wkdisplay text-base font-semibold transition",
            active
              ? "border-wk-rune bg-wk-panel text-wk-parchment shadow-[0_0_8px_rgba(82,216,208,.45)]"
              : "border-wk-brass/60 bg-wk-raise text-wk-brasshi hover:border-wk-brasshi hover:text-wk-parchment",
          )}
        >
          {initial}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{server.name}</TooltipContent>
    </Tooltip>
  );
}
