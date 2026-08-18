import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Download, HardDrive, LogOut, Plus, Users as UsersIcon } from "lucide-react";
import { type Server } from "../lib/api";
import { useAuth } from "../lib/auth";
import { usePwaInstall } from "../lib/pwa";
import { cn } from "../lib/utils";
import { FtServerFlame } from "./flametender/FtServerFlame";
import { AddServerFlow } from "./AddServerFlow";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

/** Desktop icon rail: sigil coin, one hearth coin per server, add button, logout. */
export function ServerRail({ servers, activeServerId }: { servers: Server[]; activeServerId: number | null }) {
  const { username, logout, isAdmin } = useAuth();
  const pwa = usePwaInstall();
  const navigate = useNavigate();
  const location = useLocation();
  const [addOpen, setAddOpen] = useState(false);

  const goToServer = (id: number) => navigate(`/servers/${id}`);

  return (
    <aside className="flex w-[72px] shrink-0 flex-col items-center gap-3 border-r border-black/20 bg-ft-void py-4">
      <div className="mb-2 flex h-9 w-9 items-center justify-center rounded-full border border-ft-stone bg-ft-panel font-ftdisplay text-base font-bold text-ft-flame" title="Flametender">F</div>

      {servers.map((server) => (
        <FtServerFlame
          key={server.id}
          server={server}
          active={server.id === activeServerId}
          onClick={() => goToServer(server.id)}
        />
      ))}

      {/* Creating servers is an admin endpoint; don't offer it to others. */}
      {isAdmin && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={() => setAddOpen(true)}
              className="mt-1 flex h-11 w-11 items-center justify-center rounded-full border-2 border-dashed border-white/20 text-ft-bone/40 transition hover:border-white/40 hover:text-ft-bone/70"
            >
              <Plus className="h-5 w-5" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">Add server</TooltipContent>
        </Tooltip>
      )}

      <div className="flex-1" />

      {isAdmin && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={() => navigate("/host")}
              className={cn(
                "flex h-10 w-10 items-center justify-center rounded-full transition",
                location.pathname === "/host"
                  ? "bg-ft-panel text-ft-bone"
                  : "text-ft-bone/40 hover:bg-ft-panel hover:text-ft-bone",
              )}
            >
              <HardDrive className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">Host</TooltipContent>
        </Tooltip>
      )}

      {isAdmin && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={() => navigate("/users")}
              className={cn(
                "flex h-10 w-10 items-center justify-center rounded-full transition",
                location.pathname === "/users"
                  ? "bg-ft-panel text-ft-bone"
                  : "text-ft-bone/40 hover:bg-ft-panel hover:text-ft-bone",
              )}
            >
              <UsersIcon className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">Users</TooltipContent>
        </Tooltip>
      )}

      {/* Only when the browser has actually offered — an install button
          that does nothing is worse than no button. */}
      {pwa.available && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              onClick={() => pwa.install()}
              className="flex h-10 w-10 items-center justify-center rounded-full text-ft-flame/70 transition hover:bg-ft-panel hover:text-ft-flame"
            >
              <Download className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">Install Flametender</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <button
            onClick={() => logout()}
            className="flex h-10 w-10 items-center justify-center rounded-full text-ft-bone/40 transition hover:bg-ft-panel hover:text-ft-bone"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right">Log out {username}</TooltipContent>
      </Tooltip>

      <AddServerFlow open={addOpen} onOpenChange={setAddOpen} />
    </aside>
  );
}
