import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "../lib/api";
import { hasSync, saveUser, type UserChanges } from "../lib/users";
import { PageHeader } from "../components/AppShell";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { TableHead, TableRow, TableShell } from "../components/ui/table";
import type { AppUser } from "../lib/types";

// min-w keeps the four columns legible on a phone: the shell scrolls
// sideways rather than crushing the action buttons into a single column.
const COLS = "grid min-w-[560px] grid-cols-[1.2fr_0.6fr_0.8fr_2fr] gap-2";

export function AdminUsers() {
  const queryClient = useQueryClient();
  const users = useQuery({ queryKey: ["users"], queryFn: api.users });
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [admin, setAdmin] = useState(false);

  const done = (msg: string) => {
    toast.success(msg);
    queryClient.invalidateQueries({ queryKey: ["users"] });
  };
  const onError = (err: unknown) => toast.error(errorDetail(err));

  // Every change goes through saveUser, which sends the whole record —
  // the API replaces role, permissions and disabled together.
  const update = useMutation({
    mutationFn: ({ user, changes }: { user: AppUser; changes: UserChanges; msg: string }) =>
      saveUser(user, changes),
    onSuccess: (_data, vars) => done(vars.msg),
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: number) => api.deleteUser(id),
    onSuccess: () => done("deleted"),
    onError,
  });
  const create = useMutation({
    mutationFn: () => api.createUser(username.trim(), password, admin ? "admin" : ""),
    onSuccess: () => {
      setUsername("");
      setPassword("");
      setAdmin(false);
      done("user added");
    },
    onError,
  });

  const rows = users.data ?? [];

  return (
    <>
      <PageHeader
        title="Users"
        subtitle="World custody is the grant that lets an account check worlds out and hold a companion token."
      />
      <div className="flex flex-col gap-[18px] px-4 py-5 md:px-8 md:py-6">
        <TableShell className="overflow-x-auto">
          <TableHead className={COLS}>
            <span>User</span>
            <span>Role</span>
            <span>World custody</span>
            <span className="text-right">Actions</span>
          </TableHead>
          {rows.map((u) => (
            <TableRow key={u.id} className={COLS}>
              <span className="truncate">
                {u.username}
                {u.disabled ? (
                  <Badge tone="conflict" className="ml-2">
                    disabled
                  </Badge>
                ) : null}
              </span>
              <span className={u.role === "admin" ? "font-mono text-[12px] text-goldhi" : "font-mono text-[12px] text-mist"}>
                {u.role === "admin" ? "admin" : "user"}
              </span>
              <span className="text-[12px]">
                {u.role === "admin" ? (
                  <span className="text-mist">via admin</span>
                ) : hasSync(u) ? (
                  <span className="text-ok">yes</span>
                ) : (
                  <span className="text-mist">no</span>
                )}
              </span>
              <div className="flex flex-wrap justify-end gap-2">
                {u.role !== "admin" ? (
                  <Button
                    variant="quiet"
                    size="sm"
                    onClick={() =>
                      update.mutate({
                        user: u,
                        changes: { sync: !hasSync(u) },
                        msg: hasSync(u)
                          ? `${u.username} can no longer hold worlds`
                          : `${u.username} can now hold worlds`,
                      })
                    }
                  >
                    {hasSync(u) ? "Revoke custody" : "Grant custody"}
                  </Button>
                ) : null}
                {u.role === "admin" ? (
                  <ConfirmDialog
                    trigger={
                      <Button variant="quiet" size="sm">
                        Make ordinary user
                      </Button>
                    }
                    title={`Make ${u.username} an ordinary user?`}
                    body="They keep world custody — a demotion shouldn't stop them doing what they were just doing."
                    confirmLabel="Demote"
                    onConfirm={() =>
                      // Demotion carries the custody grant with it, for the
                      // reason the dialog gives.
                      update.mutate({
                        user: u,
                        changes: { role: "user", sync: true },
                        msg: `${u.username} is now an ordinary user`,
                      })
                    }
                  />
                ) : (
                  <Button
                    variant="quiet"
                    size="sm"
                    onClick={() =>
                      update.mutate({
                        user: u,
                        changes: { role: "admin", sync: true },
                        msg: `${u.username} is now an admin`,
                      })
                    }
                  >
                    Make admin
                  </Button>
                )}
                <Button
                  variant="quiet"
                  size="sm"
                  onClick={() =>
                    update.mutate({
                      user: u,
                      changes: { disabled: !u.disabled },
                      msg: u.disabled ? "enabled" : "signed out and blocked",
                    })
                  }
                >
                  {u.disabled ? "Enable" : "Disable"}
                </Button>
                <ConfirmDialog
                  trigger={
                    <Button variant="danger" size="sm">
                      Delete
                    </Button>
                  }
                  title={`Delete ${u.username}?`}
                  body="Their worlds and versions stay."
                  confirmLabel="Delete"
                  danger
                  onConfirm={() => remove.mutate(u.id)}
                />
              </div>
            </TableRow>
          ))}
        </TableShell>

        <form
          className="flex flex-wrap items-end gap-3 rounded-panel border border-dashed border-edge bg-well px-5 py-4"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            if (username.trim()) create.mutate();
          }}
        >
          <div className="flex min-w-[12rem] flex-1 flex-col gap-1">
            <Label htmlFor="new-user">Username</Label>
            <Input id="new-user" value={username} onChange={(e) => setUsername(e.target.value)} />
          </div>
          <div className="flex min-w-[12rem] flex-1 flex-col gap-1">
            <Label htmlFor="new-pass">Password</Label>
            <Input
              id="new-pass"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 pb-2 text-[13px]">
            <input type="checkbox" checked={admin} onChange={(e) => setAdmin(e.target.checked)} /> admin
          </label>
          <Button type="submit" variant="primary" size="lg" disabled={create.isPending}>
            Add user
          </Button>
        </form>

        <p className="max-w-3xl text-[12px] italic text-mist">
          New accounts made here get world custody, and so do accounts created by signing in through Cloudflare Access
          — that policy already names your friend group (set{" "}
          <span className="font-mono not-italic">ACCESS_GRANT_CUSTODY=0</span> to grant each one by hand instead).
          Anyone who signed in before that was the default arrives without it; grant it above. Admins have it
          implicitly, and additionally manage worlds, users and rollbacks.
        </p>
      </div>
    </>
  );
}
