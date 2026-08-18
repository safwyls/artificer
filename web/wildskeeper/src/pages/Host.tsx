import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { AlertTriangle, HardDrive, Search } from "lucide-react";
import { api, errorDetail, type HostContainer, type HostImage, type HostOverview } from "../lib/api";
import { cn } from "../lib/utils";

/** The host dashboard: the containers Anvil manages on the machine this
 * console deploys to — this console's and the other consoles' — and the
 * images behind them. Scoped to Anvil's own stack on purpose: on a shared
 * box (a NAS running dozens of unrelated apps) the rest of the machine is
 * not this console's to show. Read-only on purpose too: every mutation
 * goes through the flow that owns it (the wizard, a server page's power
 * and delete verbs), so this page can see without being able to break. */

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
 * an orphan of ours, or another console's server. ("unmanaged" survives as
 * a defensive label — the API scopes those rows out.) */
export function containerKind(c: HostContainer): "registered" | "orphan" | "foreign" | "unmanaged" {
  if (!c.managed) return "unmanaged";
  if (!c.mine) return "foreign";
  return c.serverId ? "registered" : "orphan";
}

type SortDir = 1 | -1;

function SortHeader({
  label,
  active,
  dir,
  onClick,
}: {
  label: string;
  active: boolean;
  dir: SortDir;
  onClick: () => void;
}) {
  return (
    <th className="px-4 py-2.5 font-semibold">
      <button
        onClick={onClick}
        className={cn("uppercase tracking-wide hover:text-wk-parchment", active && "text-wk-parchment")}
      >
        {label}
        {active && <span aria-hidden> {dir === 1 ? "▲" : "▼"}</span>}
      </button>
    </th>
  );
}

function FilterInput({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <label className="flex items-center gap-2 rounded-full border border-wk-edge bg-wk-panel px-3 py-1.5">
      <Search className="h-3.5 w-3.5 text-wk-parchment/40" />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-40 bg-transparent text-xs text-wk-parchment outline-none placeholder:text-wk-parchment/40"
      />
    </label>
  );
}

function StateBadge({ c }: { c: HostContainer }) {
  const state = c.state || (c.running ? "running" : "stopped");
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-semibold",
        c.running ? "bg-wk-ok/10 text-wk-ok" : "bg-wk-raise text-wk-parchment/60",
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

type ContainerSortKey = "name" | "state" | "image";

function Containers({ overview }: { overview: HostOverview }) {
  const [filter, setFilter] = useState("");
  const [sortKey, setSortKey] = useState<ContainerSortKey>("name");
  const [dir, setDir] = useState<SortDir>(1);

  const sortBy = (key: ContainerSortKey) => {
    setDir(key === sortKey ? ((-dir) as SortDir) : 1);
    setSortKey(key);
  };

  const all = overview.containers ?? [];
  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const kept = all.filter(
      (c) =>
        !q ||
        [c.name, c.image, c.serverName, c.owner, c.slug].some((f) => f?.toLowerCase().includes(q)),
    );
    const val = (c: HostContainer) =>
      sortKey === "name" ? c.name : sortKey === "image" ? c.image : c.running ? `0${c.name}` : `1${c.name}`;
    return [...kept].sort((a, b) => dir * val(a).localeCompare(val(b)));
  }, [all, filter, sortKey, dir]);

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-bold uppercase tracking-wide text-wk-parchment/60">Containers</h2>
        {all.length > 0 && <FilterInput value={filter} onChange={setFilter} placeholder="Filter containers…" />}
      </div>
      {overview.fleetError && <SectionError message={overview.fleetError} />}
      {!overview.fleetError && all.length === 0 && (
        <p className="text-sm text-wk-parchment/50">Anvil manages nothing on this host yet.</p>
      )}
      {all.length > 0 && rows.length === 0 && (
        <p className="text-sm text-wk-parchment/50">Nothing matches "{filter}".</p>
      )}
      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-wk-edge bg-wk-panel">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-wk-edge text-xs uppercase tracking-wide text-wk-parchment/45">
                <SortHeader label="Container" active={sortKey === "name"} dir={dir} onClick={() => sortBy("name")} />
                <SortHeader label="State" active={sortKey === "state"} dir={dir} onClick={() => sortBy("state")} />
                <th className="px-4 py-2.5 font-semibold">Server</th>
                <SortHeader label="Image" active={sortKey === "image"} dir={dir} onClick={() => sortBy("image")} />
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

type ImageSortKey = "name" | "size" | "used";

function Images({ overview }: { overview: HostOverview }) {
  const [filter, setFilter] = useState("");
  const [sortKey, setSortKey] = useState<ImageSortKey>("size");
  const [dir, setDir] = useState<SortDir>(1);

  const sortBy = (key: ImageSortKey) => {
    setDir(key === sortKey ? ((-dir) as SortDir) : 1);
    setSortKey(key);
  };

  const all = overview.images ?? [];
  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const kept = all.filter(
      (img) =>
        !q ||
        img.tags.some((t) => t.toLowerCase().includes(q)) ||
        img.containers.some((c) => c.toLowerCase().includes(q)) ||
        img.id.toLowerCase().includes(q),
    );
    return [...kept].sort((a, b) => {
      // Size and use count sort biggest-first on the first click — that is
      // the question those columns answer; names sort a-to-z.
      if (sortKey === "size") return dir * (b.size - a.size);
      if (sortKey === "used") return dir * (b.containers.length - a.containers.length);
      return dir * imageName(a).localeCompare(imageName(b));
    });
  }, [all, filter, sortKey, dir]);

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-bold uppercase tracking-wide text-wk-parchment/60">Images</h2>
        {all.length > 0 && <FilterInput value={filter} onChange={setFilter} placeholder="Filter images…" />}
      </div>
      {overview.imagesError && <SectionError message={overview.imagesError} />}
      {!overview.imagesError && all.length === 0 && (
        <p className="text-sm text-wk-parchment/50">No Anvil images on this host.</p>
      )}
      {all.length > 0 && rows.length === 0 && (
        <p className="text-sm text-wk-parchment/50">Nothing matches "{filter}".</p>
      )}
      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-wk-edge bg-wk-panel">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-wk-edge text-xs uppercase tracking-wide text-wk-parchment/45">
                <SortHeader label="Image" active={sortKey === "name"} dir={dir} onClick={() => sortBy("name")} />
                <SortHeader label="Size" active={sortKey === "size"} dir={dir} onClick={() => sortBy("size")} />
                <SortHeader label="Used by" active={sortKey === "used"} dir={dir} onClick={() => sortBy("used")} />
              </tr>
            </thead>
            <tbody>
              {rows.map((img) => (
                <tr key={img.id} className="border-b border-wk-edge/50 last:border-0">
                  <td className="px-4 py-2.5 font-mono text-xs text-wk-parchment">
                    {imageName(img)}
                    {img.tags.length === 0 && (
                      <span className="ml-2 rounded-full bg-wk-raise px-2 py-0.5 font-sans text-[11px] font-semibold text-wk-parchment/50">
                        untagged
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

  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b border-wk-edge bg-wk-bg px-4 py-5 lg:px-8 lg:py-6">
        <div>
          <h1 className="font-display text-xl font-extrabold lg:text-2xl">Host</h1>
          <p className="mt-0.5 text-sm text-wk-parchment/50">
            The containers Anvil manages on this machine — every console's — and the images behind them
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
                label="Managed containers"
                value={overview.fleetError ? "—" : `${runningCount} / ${containers.length}`}
                detail={overview.fleetError ? undefined : "running / total under Anvil"}
              />
              <SummaryTile
                label="Anvil images on disk"
                value={overview.imagesError ? "—" : formatBytes(imagesBytes)}
                detail={overview.imagesError ? undefined : `${images.length} image${images.length === 1 ? "" : "s"}`}
              />
            </section>
            {overview.healthError && <SectionError message={overview.healthError} />}

            <Containers overview={overview} />
            <Images overview={overview} />
          </>
        )}
      </div>
    </div>
  );
}
