import { Suspense, lazy, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { cn } from "../lib/utils";

/**
 * The advisor's site-wide presence: a floating bubble in the bottom-right
 * of every authed view, opening a chat window above it. This file stays
 * deliberately light — the chat itself (AdvisorPanel, with the breeding
 * table and stat catalogs behind it) loads lazily on first open, so pages
 * that never touch the advisor never download it.
 *
 * Anubis fronts the bubble — the scholarly workhorse suits an advisor. The
 * avatar is served straight from the pal-icon set the app already ships,
 * not imported through the paldex module (which would drag the full
 * catalog into the main chunk).
 */
export const ADVISOR_AVATAR = `${import.meta.env.BASE_URL}pal-icons/anubis.webp`;

const AdvisorPanel = lazy(() => import("./AdvisorPanel").then((m) => ({ default: m.AdvisorPanel })));

export function AdvisorOverlay({ serverId }: { serverId: number | null }) {
  const [open, setOpen] = useState(false);
  // Expanded turns the floating window into a centered review pane over a
  // dimmed backdrop — for reading back a long conversation.
  const [expanded, setExpanded] = useState(false);
  // Once opened, the panel stays mounted (hidden) so closing the bubble
  // doesn't discard the conversation.
  const [everOpened, setEverOpened] = useState(false);
  const queryClient = useQueryClient();

  const statusQuery = useQuery({
    queryKey: ["advisor-status", serverId],
    queryFn: () => api.advisorStatus(serverId!),
    enabled: serverId !== null,
    retry: false,
    staleTime: Infinity,
  });
  const status = statusQuery.data;

  // No bubble without a server to talk about, or when the status endpoint
  // isn't there (the demo). Otherwise everyone gets one: even with no
  // shared key, any user can bring their own from inside the panel.
  if (serverId === null || !status) return null;

  return (
    <>
      {everOpened && (
        <div
          className={cn(
            expanded
              ? "fixed inset-0 z-40 flex items-center justify-center bg-ink/30 p-3 lg:p-10"
              : "fixed bottom-36 right-4 z-40 lg:bottom-[5.5rem] lg:right-6",
            !open && "hidden",
          )}
          onClick={
            // Clicking the dimmed backdrop shrinks back to the floating
            // window — it doesn't close the chat.
            expanded ? (e) => e.target === e.currentTarget && setExpanded(false) : undefined
          }
        >
          <Suspense fallback={null}>
            {/* Keyed by server: a different server is a different roster,
                so the conversation starts over. */}
            <AdvisorPanel
              key={serverId}
              serverId={serverId}
              status={status}
              onStatusChange={(s) => queryClient.setQueryData(["advisor-status", serverId], s)}
              onClose={() => {
                setOpen(false);
                setExpanded(false);
              }}
              expanded={expanded}
              onToggleExpand={() => setExpanded((x) => !x)}
            />
          </Suspense>
        </div>
      )}
      <div className="fixed bottom-20 right-4 z-40 flex items-center gap-2 lg:bottom-6 lg:right-6">
        {!open && (
          <span className="relative rounded-full border border-ink/10 bg-white px-3 py-1 font-display text-xs font-bold text-ink/70 shadow-md">
            Ask Anubis
            {/* The speech-bubble tail: a rotated square poking out toward
                the avatar, borrowing the pill's own border. */}
            <span
              aria-hidden
              className="absolute -right-1 top-1/2 h-2 w-2 -translate-y-1/2 rotate-45 border-r border-t border-ink/10 bg-white"
            />
          </span>
        )}
        <button
          onClick={() => {
            setOpen(!open);
            setEverOpened(true);
          }}
          aria-label={open ? "Close Ask Anubis" : "Ask Anubis"}
          title="Ask Anubis"
          className={cn(
            "h-12 w-12 rounded-full border-2 bg-paper shadow-lg transition-transform hover:scale-105",
            open ? "border-brand-red" : "border-brand-amber",
          )}
        >
          <img src={ADVISOR_AVATAR} alt="" className="h-full w-full rounded-full object-contain p-1" />
        </button>
      </div>
    </>
  );
}
