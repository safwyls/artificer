import { NavLink, Outlet } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Globe, Image, Laptop, Database, Users } from "lucide-react";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";
import { useCustodyStream } from "../lib/live";
import { cn } from "../lib/utils";

/**
 * The shell every signed-in page sits in: the sidebar names what this
 * deployment has, the footer names who you are and whether the vault is
 * still talking. Admin-only destinations are rendered only for admins —
 * a nav item that answers "Failed to load users" is worse than no item.
 */
const NAV = [
  { to: "/", label: "Worlds", icon: Globe, end: true, admin: false },
  { to: "/companion", label: "Companion", icon: Laptop, end: false, admin: false },
  { to: "/admin/users", label: "Users", icon: Users, end: false, admin: true },
  { to: "/admin/artwork", label: "Cover art", icon: Image, end: false, admin: true },
  { to: "/admin/catalogue", label: "Save catalogue", icon: Database, end: false, admin: true },
];

export function AppShell() {
  const { username, isAdmin, logout } = useAuth();
  // One stream for the whole app, opened here rather than per page: the
  // live dot lives in this footer, and a page that mounts and unmounts
  // shouldn't take the connection with it.
  const { live } = useCustodyStream();
  const version = useQuery({ queryKey: ["version"], queryFn: api.version, staleTime: Infinity });

  const nav = NAV.filter((item) => !item.admin || isAdmin);
  const liveDot = (
    <span
      className={cn("inline-block h-[7px] w-[7px] rounded-full", live ? "bg-ok" : "bg-mist")}
      aria-hidden
    />
  );

  return (
    <div className="flex min-h-screen flex-col md:flex-row">
      {/* Mobile chrome: the sidebar's job in two slim rows — identity and
          sign-out up top, the nav as one scrollable pill row beneath. Pinned:
          the nav and the live dot are why the bar exists, and both are
          useless once scrolled away. The build rides next to the wordmark —
          name and build are one identity, as on the login page. */}
      <header className="sticky top-0 z-40 border-b border-edge bg-well md:hidden">
        <div className="flex items-center gap-2.5 px-4 pb-2 pt-[max(0.75rem,env(safe-area-inset-top))]">
          <span className="text-[19px] tracking-[0.06em] text-gold">Reliquary</span>
          <span className="font-mono text-[10px] text-mist">{version.data?.version ?? "…"}</span>
          {liveDot}
          <span className="ml-auto min-w-0 truncate text-[12px] text-mist">{username}</span>
          <button
            className="text-[12px] text-mist underline-offset-2 hover:text-parchment hover:underline"
            onClick={() => logout()}
          >
            sign out
          </button>
        </div>
        <nav className="flex items-center gap-1.5 overflow-x-auto px-4 pb-2.5">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border px-3 py-1 text-[13px] no-underline",
                  isActive
                    ? "border-edge bg-panel text-goldhi"
                    : "border-transparent text-mist",
                )
              }
            >
              <item.icon className="h-3.5 w-3.5" aria-hidden />
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>
      </header>
      <nav className="hidden w-56 flex-none flex-col border-r border-edge bg-well py-6 md:flex">
        <div className="border-b border-edge px-5 pb-5">
          <div className="text-[21px] tracking-[0.06em] text-gold">Reliquary</div>
          <div className="mt-0.5 text-[12px] text-mist">the vault of shared worlds</div>
        </div>
        <div className="flex flex-col gap-0.5 px-2.5 py-3.5">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2.5 rounded px-3 py-2 text-[15px] no-underline",
                  isActive
                    ? "border border-edge bg-panel text-goldhi"
                    : "border border-transparent text-mist hover:text-parchment",
                )
              }
            >
              <item.icon className="h-4 w-4" aria-hidden />
              <span>{item.label}</span>
              {item.admin ? (
                <span className="ml-auto text-[10px] tracking-[0.08em] text-rune">ADMIN</span>
              ) : null}
            </NavLink>
          ))}
        </div>
        <div className="mt-auto flex flex-col gap-1.5 border-t border-edge px-5 pt-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-[26px] w-[26px] items-center justify-center rounded-full border border-gold bg-panel text-[12px] text-gold">
              {(username ?? "?").slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0">
              <div className="truncate text-[13px]">{username}</div>
              <div className="text-[11px] text-mist">
                {isAdmin ? "admin" : "user"} ·{" "}
                <button className="underline-offset-2 hover:text-parchment hover:underline" onClick={() => logout()}>
                  sign out
                </button>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-1.5 font-mono text-[11px] text-mist">
            {liveDot}
            <span>{live ? "live" : "reconnecting…"}</span>
            <span>·</span>
            {/* On the login page too: a bug report about save sync should be
                able to name the build without anyone opening a container. */}
            <span>reliquary {version.data?.version ?? "…"}</span>
          </div>
        </div>
      </nav>
      <main className="flex min-w-0 flex-1 flex-col">
        <Outlet />
      </main>
    </div>
  );
}

/** The page header every screen wears: a gold title, a line saying what the
 * screen is for, and room on the right for its one primary action. */
export function PageHeader({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2.5 border-b border-edge px-4 pb-3.5 pt-4 md:px-8 md:pb-[18px] md:pt-[26px]">
      <div>
        <h1>{title}</h1>
        {subtitle ? <div className="mt-0.5 text-[13px] text-mist">{subtitle}</div> : null}
      </div>
      {children}
    </div>
  );
}
