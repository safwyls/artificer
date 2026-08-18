import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { AlertTriangle, HardDrive } from "lucide-react";
import { api, errorDetail, type HostContainer, type HostImage, type HostOverview } from "../lib/api";
import { cn } from "../lib/utils";

/** The host dashboard: what Anvil holds on the machine this console deploys
 * to. Containers Wildskeeper cannot see any other way — other consoles'
 * servers, hand-run stacks, orphans of its own — plus every published port
 * and the images spending the host's disk. Read-only on purpose: every
 * mutation goes through the flow that owns it (the wizard, a server page's
 * power and delete verbs), so this page can show everything without being
 * able to break anything. */

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

/** What one container row is to this console, as a label: a linked server,
 * an orphan of ours, another console's server, or unmanaged. */
export function containerKind(c: HostContainer): "registered" | "orphan" | "foreign" | "unmanaged" {
  if (!c.managed) return "unmanaged";
  if (!c.mine) return "foreign";
  return c.serverId ? "registered" : "orphan";
}

function StateBadge({ c }: { c: HostContainer }) {
  const state = c.state || (c.running ? "running" : "stopped");
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-semibold",
        c.running ? "bg-wk-ok/10 text-wk-ok" : "bg-wk-raise text-wk-mist",
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full", c.running ? "bg-wk-ok" : "bg-wk-mist/50")} />
      {state}
    </span>
  );
}

function KindBadge({ c }: { c: HostContainer }) {
  switch (containerKind(c)) {
    case "registered":
      return (
        <Link
          to={`/servers/${c.serverId}`}
          className="text-xs font-semibold text-wk-brasshi hover:underline"
        >
          {c.serverName}
        </Link>
      );
    case "orphan":
      // Ours per Anvil's labels, registered nowhere — exactly what the
      // wizard's "adopt an existing container" flow exists to fix.
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-wk-brasshi/15 px-2 py-0.5 text-xs font-semibold text-wk-brasshi">
          <AlertTriangle className="h-3 w-3" />
          not registered — adoptable
        </span>
      );
    case "foreign":
      return <span className="text-xs text-wk-parchment/50">{c.owner}</span>;
    case "unmanaged":
      return <span className="text-xs text-wk-parchment/40">not managed by Anvil</span>;
  }
}

function SectionError({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 rounded-xl border border-wk-ember/30 bg-wk-ember/10 p-3 text-sm text-wk-ember">
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
      <span>{message}</span>
    </div>
  );
}

function SummaryTile({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="rounded-2xl border border-wk-edge bg-wk-panel p-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-wk-parchment/45">{label}</p>
      <p className="mt-1 text-xl font-bold text-wk-parchment">{value}</p>
      {detail && <p className="mt-0.5 truncate text-xs text-wk-parchment/50">{detail}</p>}
    </div>
  );
}

function portsLabel(c: HostContainer): string {
  return (c.ports ?? []).map((p) => `${p.host}${p.proto === "udp" ? "/udp" : ""}`).join(", ");
}

function imageName(img: HostImage): string {
  if (img.tags.length > 0) return img.tags.join(", ");
  return `${img.id.replace(/^sha256:/, "").slice(0, 12)} (untagged)`;
}

function Containers({ overview }: { overview: HostOverview }) {
  const rows = overview.containers ?? [];
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-bold uppercase tracking-wide text-wk-parchment/60">Containers</h2>
      {overview.fleetError && <SectionError message={overview.fleetError} />}
      {!overview.fleetError && rows.length === 0 && (
        <p className="text-sm text-wk-parchment/50">Nothing is running on this host.</p>
      )}
      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-wk-edge bg-wk-panel">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-wk-edge text-xs uppercase tracking-wide text-wk-parchment/45">
                <th className="px-4 py-2.5 font-semibold">Container</th>
                <th className="px-4 py-2.5 font-semibold">State</th>
                <th className="px-4 py-2.5 font-semibold">Server</th>
                <th className="px-4 py-2.5 font-semibold">Image</th>
                <th className="px-4 py-2.5 font-semibold">Host ports</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c) => (
                <tr key={c.name} className="border-b border-wk-edge/50 last:border-0">
                  <td className="px-4 py-2.5">
                    <span className="font-mono text-xs font-semibold text-wk-parchment">{c.name}</span>
                    {c.dataDir && <p className="font-mono text-[11px] text-wk-parchment/40">{c.dataDir}</p>}
                  </td>
                  <td className="px-4 py-2.5">
                    <StateBadge c={c} />
                    {c.status && <p className="mt-0.5 text-[11px] text-wk-parchment/45">{c.status}</p>}
                  </td>
                  <td className="px-4 py-2.5">
                    <KindBadge c={c} />
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-wk-parchment/60">{c.image}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-wk-parchment/60">{portsLabel(c) || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Images({ overview }: { overview: HostOverview }) {
  const rows = overview.images ?? [];
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-bold uppercase tracking-wide text-wk-parchment/60">Images</h2>
      {overview.imagesError && <SectionError message={overview.imagesError} />}
      {!overview.imagesError && rows.length === 0 && (
        <p className="text-sm text-wk-parchment/50">No images on this host.</p>
      )}
      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-wk-edge bg-wk-panel">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-wk-edge text-xs uppercase tracking-wide text-wk-parchment/45">
                <th className="px-4 py-2.5 font-semibold">Image</th>
                <th className="px-4 py-2.5 font-semibold">Size</th>
                <th className="px-4 py-2.5 font-semibold">Used by</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((img) => (
                <tr key={img.id} className="border-b border-wk-edge/50 last:border-0">
                  <td className="px-4 py-2.5 font-mono text-xs text-wk-parchment">
                    {imageName(img)}
                    {img.tags.length === 0 && (
                      <span className="ml-2 rounded-full bg-wk-raise px-2 py-0.5 font-sans text-[11px] font-semibold text-wk-parchment/50">
                        dangling
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-wk-parchment/70">{formatBytes(img.size)}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-wk-parchment/60">
                    {img.containers.length > 0 ? img.containers.join(", ") : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Ports({ overview }: { overview: HostOverview }) {
  const rows = overview.ports ?? [];
  if (overview.fleetError || rows.length === 0) return null;
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-bold uppercase tracking-wide text-wk-parchment/60">Published ports</h2>
      <div className="flex flex-wrap gap-2">
        {rows.map((p) => (
          <span
            key={`${p.port}/${p.proto}/${p.container}`}
            className="rounded-full border border-wk-edge bg-wk-panel px-3 py-1 font-mono text-xs text-wk-parchment/70"
            title={p.container}
          >
            {p.port}/{p.proto} <span className="text-wk-parchment/40">→ {p.container}</span>
          </span>
        ))}
      </div>
    </section>
  );
}

export function Host() {
  const overviewQuery = useQuery({
    queryKey: ["host"],
    queryFn: api.hostOverview,
    refetchInterval: 15_000,
  });
  const overview = overviewQuery.data;

  const containers = overview?.containers ?? [];
  const runningCount = containers.filter((c) => c.running).length;
  const images = overview?.images ?? [];
  const imagesBytes = images.reduce((sum, i) => sum + i.size, 0);
  const danglingCount = images.filter((i) => i.tags.length === 0).length;

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b border-wk-edge bg-wk-bg px-4 py-5 lg:px-8 lg:py-6">
        <div>
          <h1 className="font-display text-xl font-extrabold lg:text-2xl">Host</h1>
          <p className="mt-0.5 text-sm text-wk-parchment/50">
            What Anvil holds on this machine — every console's containers, the ports they publish, the images on disk
          </p>
        </div>
        <HardDrive className="h-6 w-6 text-wk-parchment/30" />
      </header>

      <div className="space-y-8 p-4 lg:p-8">
        {overviewQuery.isLoading && <p className="text-sm text-wk-parchment/50">Asking Anvil…</p>}
        {overviewQuery.isError && (
          <SectionError message={errorDetail(overviewQuery.error) ?? "Failed to load the host overview"} />
        )}

        {overview && !overview.available && (
          <div className="rounded-2xl border border-wk-edge bg-wk-panel p-6">
            <p className="font-semibold text-wk-parchment">No host service connected</p>
            <p className="mt-1 max-w-prose text-sm text-wk-parchment/60">{overview.reason}</p>
          </div>
        )}

        {overview?.available && (
          <>
            <section className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <SummaryTile
                label="Anvil"
                value={overview.health ? (overview.health.version || "connected") : "unreachable"}
                detail={overview.healthError ?? overview.anvilURL}
              />
              <SummaryTile
                label="Docker socket"
                value={overview.health ? (overview.health.dockerOk ? "answering" : "not answering") : "—"}
                detail={overview.health?.dataRoot && `data root ${overview.health.dataRoot}`}
              />
              <SummaryTile
                label="Containers"
                value={overview.fleetError ? "—" : `${runningCount} / ${containers.length}`}
                detail={overview.fleetError ? undefined : "running / total on the host"}
              />
              <SummaryTile
                label="Images on disk"
                value={overview.imagesError ? "—" : formatBytes(imagesBytes)}
                detail={
                  overview.imagesError
                    ? undefined
                    : `${images.length} image${images.length === 1 ? "" : "s"}${danglingCount ? `, ${danglingCount} dangling` : ""}`
                }
              />
            </section>
            {overview.healthError && <SectionError message={overview.healthError} />}

            <Containers overview={overview} />
            <Ports overview={overview} />
            <Images overview={overview} />
          </>
        )}
      </div>
    </div>
  );
}
