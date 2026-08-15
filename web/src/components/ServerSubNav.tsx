import { useState } from "react";
import { NavLink } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { EyeOff, Pencil, Trash2 } from "lucide-react";
import { api, type Server } from "../lib/api";
import { useAuth } from "../lib/auth";
import { formatUptime } from "../lib/palette";
import { FEATURE_ROUTES, featureLabel } from "../lib/games";
import { canSeeFeature, featureOff, serverFeatures } from "../lib/visibility";
import { cn } from "../lib/utils";
import { Badge } from "./ui/badge";
import { ServerFormDialog } from "./ServerFormDialog";
import { DeleteServerDialog } from "./DeleteServerDialog";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors",
    isActive
      ? "border border-fk-spore/25 bg-fk-spore/15 font-semibold text-fk-spore"
      : "text-fk-bone/60 hover:bg-fk-panel hover:text-fk-bone",
  );

/** Desktop second column: the active server's identity + view navigation. */
export function ServerSubNav({ server }: { server: Server }) {
  const { can, isAdmin } = useAuth();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const infoQuery = useQuery({
    queryKey: ["server-info", server.id],
    queryFn: () => api.serverInfo(server.id),
    retry: false,
    staleTime: 15_000,
  });
  // Uptime footer needs REST metrics; RCON-only servers just omit the footer.
  const metricsQuery = useQuery({
    queryKey: ["server-metrics", server.id],
    queryFn: () => api.serverMetrics(server.id),
    retry: false,
    refetchInterval: 60_000,
  });

  const transport = infoQuery.data?.transport;
  // The game port is what a player would type; there is no admin port to
  // show, since the dashboard reaches this game through its agent.
  const port = server.gamePort;

  return (
    <aside className="flex w-56 shrink-0 flex-col border-r border-black/20 bg-fk-fog text-fk-bone">
      <div className="group border-b border-white/10 px-5 py-5">
        <div className="flex items-start justify-between gap-2">
          <p className="min-w-0 flex-1 truncate font-display text-lg font-bold leading-tight">{server.name}</p>
          {/* Server edit/delete are admin endpoints; don't offer them to others. */}
          {isAdmin && (
            <span className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
              <button
                className="rounded p-1 text-fk-bone/50 hover:bg-fk-panel hover:text-fk-bone"
                title="Edit server"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              <button
                className="rounded p-1 text-fk-bone/50 hover:bg-fk-panel hover:text-fk-spore"
                title="Remove server"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </span>
          )}
        </div>
        <div className="mt-1.5 flex items-center gap-1.5">
          <span
            className={cn(
              "h-1.5 w-1.5 shrink-0 rounded-full",
              infoQuery.isSuccess ? "bg-fk-ok" : infoQuery.isError ? "bg-fk-spore" : "bg-fk-panel",
            )}
          />
          <span className="truncate font-mono text-xs text-fk-bone/60">
            {server.host}:{port}
          </span>
          {transport && (
            <Badge
              variant="outline"
              className="border-fk-flame/40 bg-fk-flame/15 px-1 py-0 font-mono text-[10px] text-fk-flame"
            >
              {transport.toUpperCase()}
            </Badge>
          )}
        </div>
      </div>

      <nav className="flex-1 space-y-1 px-3 py-4">
        <NavLink to={`/servers/${server.id}`} end className={navLinkClass}>
          Dashboard
        </NavLink>
        {/* The views come from the server's own game, so a game without a
            given view simply has no link rather than a dead one. */}
        {serverFeatures(server).map((feature) =>
          canSeeFeature(server, feature, isAdmin) ? (
            <NavLink
              key={feature}
              to={`/servers/${server.id}/${FEATURE_ROUTES[feature]}`}
              className={navLinkClass}
            >
              {featureLabel(server, feature)}
              {/* Admins keep the link and get told it's off for everyone
                  else — otherwise the only sign would be its absence from a
                  menu they can still use. */}
              {featureOff(server, feature) && (
                <EyeOff className="ml-auto h-3.5 w-3.5 shrink-0 text-fk-bone/30" aria-label="Hidden from everyone else" />
              )}
            </NavLink>
          ) : null,
        )}
        <NavLink to={`/servers/${server.id}/activity`} className={navLinkClass}>
          Activity
        </NavLink>
        <NavLink to={`/servers/${server.id}/automation`} className={navLinkClass}>
          Automation
        </NavLink>
        {can("settings") && (
          <NavLink to={`/servers/${server.id}/settings`} className={navLinkClass}>
            Settings
          </NavLink>
        )}
      </nav>

      {metricsQuery.isSuccess && (
        <div className="border-t border-white/10 px-5 py-4 font-mono text-xs text-fk-bone/40">
          Uptime · {formatUptime(metricsQuery.data.uptime)}
        </div>
      )}

      <ServerFormDialog open={editOpen} onOpenChange={setEditOpen} mode="edit" server={server} />
      <DeleteServerDialog server={server} open={deleteOpen} onOpenChange={setDeleteOpen} />
    </aside>
  );
}
