import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Laptop } from "lucide-react";
import { toast } from "sonner";
import { api, errorDetail } from "../lib/api";
import { useArtwork } from "../lib/art";
import { useAuth } from "../lib/auth";
import { POLL_MS } from "../lib/live";
import { PageHeader } from "../components/AppShell";
import { WorldCard } from "../components/WorldCard";
import { Button } from "../components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "../components/ui/dialog";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";

/** An account without world custody sees the shelf and can download a
 * version — that is the whole of what it may do, said out loud once rather
 * than discovered by finding every button missing. */
function ReadOnlyBanner() {
  return (
    <div className="rounded-panel border border-dashed border-edge bg-well px-5 py-3.5 text-[13px] text-mist">
      Your account can see these worlds and download a version, but not hold one. World custody is a grant an admin
      gives — ask one, and your companion can check worlds out and back in.
    </div>
  );
}

function NewWorldDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: (n: string) => api.createWorld(n),
    onSuccess: () => {
      toast.success("world created");
      setName("");
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (err) => toast.error(errorDetail(err)),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (trimmed) create.mutate(trimmed);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button variant="primary" size="lg" onClick={() => setOpen(true)}>
        + New world
      </Button>
      <DialogContent>
        <DialogTitle>New shared world</DialogTitle>
        <form onSubmit={submit} className="mt-4 flex flex-col gap-1.5">
          <Label htmlFor="new-world">Name</Label>
          <Input
            id="new-world"
            autoFocus
            placeholder="the world's name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <p className="mt-2 text-[12px] italic text-mist">
            Linking an installed game from the Artificer Companion also creates a world, with the game&apos;s details
            filled in — this form is the by-hand path.
          </p>
          <div className="mt-4 flex justify-end">
            <Button type="submit" variant="primary" disabled={create.isPending}>
              Create
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** A one-line pointer, not a panel: the token lives on the Companion page
 * now, and the shelf's job is worlds. */
function CompanionStrip({ minted }: { minted: boolean }) {
  return (
    <div className="flex items-center gap-3.5 rounded-panel border border-dashed border-edge bg-well px-5 py-3.5">
      <Laptop className="h-5 w-5 flex-none text-gold" strokeWidth={1.3} aria-hidden />
      <div className="flex-1 text-[13px] text-mist">
        <span className="text-[14px] text-parchment">
          {minted ? "Your companion token is minted." : "You have no companion token yet."}
        </span>{" "}
        The Artificer Companion on your gaming machine moves the saves for you — manage the token on the Companion
        page.
      </div>
      <Button asChild variant="quiet">
        <Link to="/companion">Open Companion</Link>
      </Button>
    </div>
  );
}

export function Worlds() {
  const { canSync } = useAuth();
  const worlds = useQuery({
    queryKey: ["worlds"],
    queryFn: api.worlds,
    // The slow poll under the custody stream: a proxy that eats event
    // streams shouldn't leave the shelf frozen.
    refetchInterval: POLL_MS,
  });
  const list = worlds.data?.worlds ?? [];
  // Covers are fetched when the *set* of worlds changes, never on the poll.
  useArtwork(list);
  const token = useQuery({
    queryKey: ["sync-token"],
    queryFn: api.syncToken,
    enabled: canSync,
    retry: false,
  });

  return (
    <>
      <PageHeader title="Shared worlds" subtitle="One holder at a time; every check-in a kept version.">
        {canSync ? <NewWorldDialog /> : null}
      </PageHeader>
      <div className="flex flex-col gap-3.5 px-8 py-6">
        {!canSync ? <ReadOnlyBanner /> : null}
        {worlds.isLoading ? <p className="text-mist">Reading the ledger…</p> : null}
        {worlds.isError ? (
          <p className="font-mono text-[13px] text-ember">{errorDetail(worlds.error)}</p>
        ) : null}
        {worlds.isSuccess && !list.length ? (
          <div className="rounded-panel border border-edge bg-panel px-5 py-4 text-[13px] text-mist">
            No worlds yet. Create one, or link an installed game from the Artificer Companion.
          </div>
        ) : null}
        {list.map((status) => (
          <WorldCard key={status.world.id} status={status} />
        ))}
        {canSync ? <CompanionStrip minted={Boolean(token.data?.token)} /> : null}
      </div>
    </>
  );
}
