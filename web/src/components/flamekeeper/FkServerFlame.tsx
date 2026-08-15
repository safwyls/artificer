import { type Server } from "../../lib/api";
import { cn } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip";

/**
 * One server on the icon rail: a hearth-ring coin with the server's
 * initial in the display face. The active server's ring lights flame —
 * the reserved live-state color — and everything else stays
 * stone-on-dark.
 */
export function FkServerFlame({
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
            "flex h-11 w-11 items-center justify-center rounded-full border-2 font-fkdisplay text-base font-semibold transition",
            active
              ? "border-fk-flame bg-fk-panel text-fk-bone shadow-[0_0_8px_rgba(127,195,240,.45)]"
              : "border-fk-stone/60 bg-fk-fog text-fk-stonehi hover:border-fk-stonehi hover:text-fk-bone",
          )}
        >
          {initial}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{server.name}</TooltipContent>
    </Tooltip>
  );
}
