import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff, Plus, X } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type RoleGroup } from "../../lib/api";
import { cn } from "../../lib/utils";
import { FtNote, FtPanel } from "./FtPanel";

/**
 * The role-group editor (docs/design.md, "The moderation surface").
 *
 * `userGroups` is the whole of Enshrouded's permission system: a joining
 * player types a group's password and holds that group's rights for the
 * session. There is no live admin channel, so this file *is* the access
 * control — and it is also where the passwords are, which is why the
 * panel sits behind the settings grant with the rest of the config.
 *
 * Group cards rather than a CRUD table, and capability chips rather than a
 * checkbox grid: the question an operator actually arrives with is "who
 * has admin here", and a badge row answers it at a scan.
 */

type Capability = {
  key: keyof Pick<
    RoleGroup,
    "canKickBan" | "canAccessInventories" | "canEditBase" | "canExtendBase" | "canEditWorld"
  >;
  label: string;
};

// canKickBan leads because it is the one that means admin.
const CAPABILITIES: Capability[] = [
  { key: "canKickBan", label: "Kick/ban" },
  { key: "canAccessInventories", label: "Inventories" },
  { key: "canEditBase", label: "Edit base" },
  { key: "canExtendBase", label: "Extend base" },
  { key: "canEditWorld", label: "Edit world" },
];

function newGroup(): RoleGroup {
  return {
    index: -1,
    name: "",
    password: "",
    canKickBan: false,
    // A new group's sensible default is a player role: everything except
    // the one capability that hands out admin.
    canAccessInventories: true,
    canEditBase: true,
    canExtendBase: true,
    canEditWorld: true,
    reservedSlots: 0,
  };
}

function CapabilityChip({
  label,
  on,
  admin,
  onToggle,
}: {
  label: string;
  on: boolean;
  admin?: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={onToggle}
      className={cn(
        "rounded-sm border px-2 py-0.5 text-[11px] tracking-[0.04em] transition",
        on
          ? "border-ft-flamedim bg-ft-flamedim/25 text-ft-flame"
          : "border-ft-edge text-ft-lichen hover:border-ft-stone hover:text-ft-bone",
        // The admin chip is the one worth spotting from across the list.
        admin && "font-bold",
        admin && on && "text-ft-flamehi",
      )}
    >
      {label}
    </button>
  );
}

function GroupCard({
  group,
  onChange,
  onRemove,
}: {
  group: RoleGroup;
  onChange: (next: RoleGroup) => void;
  onRemove: () => void;
}) {
  const [revealed, setRevealed] = useState(false);

  return (
    <div className="border-t border-ft-edge py-3 first:border-t-0 first:pt-1">
      <div className="flex flex-wrap items-center gap-2">
        <input
          value={group.name}
          onChange={(e) => onChange({ ...group, name: e.target.value })}
          placeholder="Group name"
          aria-label="Group name"
          spellCheck={false}
          className="w-40 rounded-sm border border-ft-edge bg-ft-void px-2 py-1 text-sm font-bold text-ft-bone outline-none focus:border-ft-flamedim"
        />
        <div className="flex min-w-0 flex-1 items-center gap-1.5">
          <input
            type={revealed ? "text" : "password"}
            value={group.password}
            onChange={(e) => onChange({ ...group, password: e.target.value })}
            placeholder="Join password"
            aria-label={`Join password for ${group.name || "this group"}`}
            spellCheck={false}
            className="min-w-0 flex-1 rounded-sm border border-ft-edge bg-ft-void px-2 py-1 font-mono text-xs text-ft-bone outline-none focus:border-ft-flamedim"
          />
          <button
            type="button"
            onClick={() => setRevealed((v) => !v)}
            title={revealed ? "Hide password" : "Reveal password"}
            aria-label={revealed ? "Hide password" : "Reveal password"}
            className="text-ft-lichen transition hover:text-ft-bone"
          >
            {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </button>
        </div>
        <label className="flex items-center gap-1.5 text-[11px] uppercase tracking-[0.1em] text-ft-lichen">
          Reserved
          <input
            type="number"
            min={0}
            max={16}
            value={group.reservedSlots}
            onChange={(e) => onChange({ ...group, reservedSlots: Number(e.target.value) })}
            aria-label={`Reserved slots for ${group.name || "this group"}`}
            className="w-14 rounded-sm border border-ft-edge bg-ft-void px-1.5 py-1 font-mono text-xs text-ft-bone outline-none focus:border-ft-flamedim"
          />
        </label>
        <button
          type="button"
          onClick={onRemove}
          title={`Remove ${group.name || "this group"}`}
          aria-label={`Remove ${group.name || "this group"}`}
          className="text-ft-lichen transition hover:text-ft-spore"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {CAPABILITIES.map((c) => (
          <CapabilityChip
            key={c.key}
            label={c.label}
            on={group[c.key]}
            admin={c.key === "canKickBan"}
            onToggle={() => onChange({ ...group, [c.key]: !group[c.key] })}
          />
        ))}
      </div>
    </div>
  );
}

export function FtRoles({ serverId }: { serverId: number }) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<RoleGroup[] | null>(null);

  const rolesQuery = useQuery({
    queryKey: ["server-roles", serverId],
    queryFn: () => api.serverRoles(serverId),
    retry: false,
  });

  const save = useMutation({
    mutationFn: (groups: RoleGroup[]) => api.updateServerRoles(serverId, groups),
    onSuccess: (res) => {
      toast.success("Roles saved — restart to apply");
      setDraft(null);
      queryClient.setQueryData(["server-roles", serverId], res);
      // The flat settings view reads the same file.
      queryClient.invalidateQueries({ queryKey: ["server-config", serverId] });
    },
    onError: (e: Error) => toast.error("Save failed", { description: e.message }),
  });

  const saved = rolesQuery.data?.groups;
  const groups = draft ?? saved ?? [];
  const dirty = useMemo(
    () => draft !== null && JSON.stringify(draft) !== JSON.stringify(saved ?? []),
    [draft, saved],
  );

  const edit = (next: RoleGroup[]) => setDraft(next);

  const noPath = rolesQuery.error instanceof ApiError && rolesQuery.error.status === 400;

  return (
    <FtPanel title="Role groups" meta={rolesQuery.data ? "restart to apply" : undefined}>
      {rolesQuery.isLoading && <p className="text-sm text-ft-lichen">Reading the file…</p>}
      {/* The no-config-path case is already explained by the settings panel
          on this page; repeating it here would be noise. */}
      {rolesQuery.isError && !noPath && (
        <p className="text-sm text-ft-spore">{(rolesQuery.error as Error).message}</p>
      )}
      {rolesQuery.data && (
        <>
          {!rolesQuery.data.writable && (
            <p className="mb-2 text-xs text-ft-spore">
              The file is on a read-only mount — saving will fail until it's mounted read-write.
            </p>
          )}
          <div>
            {groups.map((g, i) => (
              <GroupCard
                key={`${g.index}-${i}`}
                group={g}
                onChange={(next) => edit(groups.map((x, j) => (j === i ? next : x)))}
                onRemove={() => edit(groups.filter((_, j) => j !== i))}
              />
            ))}
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-ft-edge pt-3">
            <button
              onClick={() => save.mutate(groups)}
              disabled={!dirty || save.isPending}
              className="rounded border border-ft-stone bg-gradient-to-b from-[#2b2f26] to-[#1d211a] px-4 py-1.5 text-sm font-bold tracking-[0.05em] text-ft-stonehi transition hover:brightness-125 disabled:opacity-40"
            >
              {save.isPending ? "Saving…" : "Save roles"}
            </button>
            {dirty && (
              <button
                onClick={() => setDraft(null)}
                className="rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:text-ft-bone"
              >
                Discard
              </button>
            )}
            <button
              onClick={() => edit([...groups, newGroup()])}
              className="ml-auto flex items-center gap-1.5 rounded border border-ft-edge px-3 py-1.5 text-sm text-ft-lichen transition hover:border-ft-stone hover:text-ft-stonehi"
            >
              <Plus className="h-3.5 w-3.5" /> Add group
            </button>
          </div>
          <FtNote>
            A joining player types one of these passwords and holds that group's rights for the session, so keep them
            different from each other. One group must keep <b className="not-italic">Kick/ban</b> and a password —
            that role is the only moderation this server has, and it's exercised from the in-game player menu. Changes
            take effect on the next restart; players already in the world keep what they joined with.
          </FtNote>
        </>
      )}
    </FtPanel>
  );
}
