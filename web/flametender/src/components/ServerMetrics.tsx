import { useQuery } from "@tanstack/react-query";
import { api, ApiError } from "../lib/api";
import { formatUptime } from "../lib/palette";

export function ServerMetrics({
  serverId,
  onPlayersClick,
}: {
  serverId: number;
  /** When set, the players-online card becomes a shortcut to the roster —
   * handy on mobile, where the list is a long scroll below the fold. */
  onPlayersClick?: () => void;
}) {
  const metricsQuery = useQuery({
    queryKey: ["server-metrics", serverId],
    queryFn: () => api.serverMetrics(serverId),
    retry: false,
    refetchInterval: 15_000,
  });

  if (metricsQuery.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading metrics...</p>;
  }
  if (metricsQuery.isError) {
    const err = metricsQuery.error;
    if (err instanceof ApiError && err.status === 400) {
      return (
        <p className="text-sm text-muted-foreground">
          Metrics require the REST API — this server is configured RCON-only.
        </p>
      );
    }
    return <p className="text-sm text-destructive">Could not reach server.</p>;
  }

  const m = metricsQuery.data;
  if (!m) return null;

  const stats = [
    {
      label: "Players online",
      value: `${m.currentplayernum} / ${m.maxplayernum}`,
      color: "text-ft-ok",
      onClick: onPlayersClick,
    },
    { label: "Server tick", value: `${m.serverframetime.toFixed(1)} ms`, color: "text-ft-flame" },
    { label: "In-game days", value: String(m.days), color: "text-ft-stonehi" },
    { label: "Uptime", value: formatUptime(m.uptime), color: "text-ft-bone" },
  ];

  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      {stats.map((s) => {
        const className = "rounded-2xl border border-ft-edge bg-ft-panel p-4 text-left lg:p-5";
        const body = (
          <>
            <p className="text-xs font-semibold uppercase tracking-wide text-ft-bone/50">{s.label}</p>
            <p className={`mt-2 font-mono text-lg font-bold lg:text-2xl ${s.color}`}>{s.value}</p>
          </>
        );
        return s.onClick ? (
          <button
            key={s.label}
            onClick={s.onClick}
            className={`${className} transition-colors hover:border-ft-ok/40 hover:bg-ft-ok/5`}
            title="Jump to the player list"
          >
            {body}
          </button>
        ) : (
          <div key={s.label} className={className}>
            {body}
          </div>
        );
      })}
    </div>
  );
}
