import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "../lib/api";
import { fmtCount, fmtTime } from "../lib/format";
import { PageHeader } from "../components/AppShell";
import { Button } from "../components/ui/button";

/**
 * Where a game keeps its saves, from the Ludusavi manifest — fetched once
 * here so no player's machine pulls 17 MB of YAML. Without it, companions
 * fall back to Steam Cloud paths and a folder search: fewer games get a
 * suggested folder, and nothing else changes.
 */
export function AdminCatalogue() {
  const queryClient = useQueryClient();
  const status = useQuery({ queryKey: ["savehints"], queryFn: api.saveHintsStatus });
  const refresh = useMutation({
    mutationFn: api.refreshSaveHints,
    onSuccess: (out) => {
      if (out.refreshed) toast.success(`catalogue refreshed: ${fmtCount(out.status?.games ?? 0)} games`);
      else toast.error(`refresh failed: ${out.error ?? "unknown"}`);
      queryClient.invalidateQueries({ queryKey: ["savehints"] });
    },
    onError: (err) => {
      toast.error(errorDetail(err));
      queryClient.invalidateQueries({ queryKey: ["savehints"] });
    },
  });

  const st = status.data?.status ?? {};
  const rows: [string, React.ReactNode][] = [
    [
      "Catalogue",
      st.loaded ? (
        <span>
          <span className="text-ok">{fmtCount(st.games ?? 0)} games</span> · {fmtCount(st.steamIds ?? 0)} Steam ids
        </span>
      ) : st.refreshing ? (
        "loading…"
      ) : (
        <span className="text-mist">not loaded</span>
      ),
    ],
  ];
  if (st.fetchedAt) rows.push(["Fetched", fmtTime(st.fetchedAt)]);
  if (st.lastError) rows.push(["Last error", <span className="text-ember">{st.lastError}</span>]);
  if (st.url) rows.push(["Source", <span className="font-mono break-all">{st.url}</span>]);

  return (
    <>
      <PageHeader
        title="Save catalogue"
        subtitle="Companions ask this service where a game keeps its saves, so nobody's machine pulls the 17 MB catalogue."
      />
      <div className="flex max-w-3xl flex-col gap-4 px-4 py-5 md:px-8 md:py-6">
        <div className="rounded-panel border border-edge bg-panel px-5 py-4">
          <h2 className="mb-3 text-[12px] uppercase tracking-[0.12em] text-gold">Status</h2>
          <dl className="grid grid-cols-[minmax(6rem,9rem)_minmax(0,1fr)] gap-x-4 gap-y-2 text-[13px] [overflow-wrap:anywhere]">
            {rows.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-mist">{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
          <div className="mt-4">
            <Button
              variant="quiet"
              disabled={refresh.isPending}
              onClick={() => {
                toast.info("fetching the save-location catalogue — this pulls about 17 MB…");
                refresh.mutate();
              }}
            >
              Refresh now
            </Button>
          </div>
        </div>
        <p className="text-[12px] italic text-mist">
          Data from the{" "}
          <a href="https://github.com/mtkennerly/ludusavi-manifest" target="_blank" rel="noreferrer">
            Ludusavi manifest
          </a>{" "}
          (MIT), refreshed weekly.
        </p>
      </div>
    </>
  );
}
