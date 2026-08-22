import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "../lib/api";
import { fmtTime } from "../lib/format";
import { PageHeader } from "../components/AppShell";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import type { ArtworkTest } from "../lib/types";

/**
 * The panel exists to answer one question the shelf cannot: when a game has
 * no cover, is that IGDB not knowing it, or this deployment's credentials
 * not working? lastError is the difference.
 */
export function AdminArtwork() {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ["artwork-settings"], queryFn: api.artworkSettings });
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [test, setTest] = useState<ArtworkTest | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["artwork-settings"] });
  const onError = (err: unknown) => {
    toast.error(errorDetail(err));
    refresh();
  };

  const save = useMutation({
    mutationFn: () => api.saveArtworkSettings(clientId.trim(), clientSecret.trim()),
    onSuccess: (out) => {
      setTest(out.test ?? null);
      setClientSecret("");
      if (out.test?.ok) toast.success("credentials saved; IGDB answered");
      else toast.error("credentials saved, but IGDB did not answer");
      refresh();
    },
    onError,
  });
  const runTest = useMutation({
    mutationFn: api.testArtwork,
    onSuccess: (out) => {
      setTest(out.test ?? null);
      if (out.test?.ok) toast.success("IGDB answered");
      else toast.error(out.test?.error ?? "IGDB did not answer");
      refresh();
    },
    onError: (err) => {
      setTest({ ok: false, error: errorDetail(err) });
      onError(err);
    },
  });
  const clear = useMutation({
    mutationFn: api.clearArtworkSettings,
    onSuccess: () => {
      setTest(null);
      toast.success("saved credentials removed");
      refresh();
    },
    onError,
  });

  const a = settings.data;
  const st = a?.status ?? {};
  const source = st.configured ? (a?.stored ? "saved here" : "from the environment") : "not configured";
  const rows: [string, React.ReactNode][] = [
    [
      "Credentials",
      <>
        {source}
        {st.clientId ? (
          <>
            {" · client id "}
            <span className="font-mono">{st.clientId}</span>
          </>
        ) : null}
      </>,
    ],
    [
      "Fallback",
      a?.envConfigured ? "IGDB_CLIENT_ID/SECRET are set in the environment" : "nothing in the environment",
    ],
  ];
  if (st.lastOkAt) rows.push(["Last success", fmtTime(st.lastOkAt)]);
  if (st.lastError) rows.push(["Last error", <span className="text-ember">{st.lastError}</span>]);
  if (st.configured) {
    rows.push([
      "Lookups",
      `${st.lookups ?? 0} asked · ${st.hits ?? 0} matched · ${st.misses ?? 0} unknown · ${st.cached ?? 0} cached`,
    ]);
  }
  if (st.filter) rows.push(["Steam filter", <span className="font-mono">{st.filter}</span>]);
  if (test) {
    rows.push([
      "Test",
      test.ok ? <span className="text-ok">IGDB answered</span> : <span className="text-ember">{test.error}</span>,
    ]);
  }

  return (
    <>
      <PageHeader
        title="Cover art"
        subtitle="IGDB authenticates through a Twitch application, so the pair is a Twitch client id and secret."
      />
      <div className="flex max-w-3xl flex-col gap-4 px-8 py-6">
        <div className="rounded-panel border border-edge bg-panel px-5 py-4">
          <h2 className="mb-3 text-[12px] uppercase tracking-[0.12em] text-gold">Status</h2>
          <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-2 text-[13px]">
            {rows.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-mist">{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        </div>

        <form
          className="flex flex-col gap-3 rounded-panel border border-dashed border-edge bg-well px-5 py-4"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="flex flex-wrap gap-3">
            <div className="flex min-w-[14rem] flex-1 flex-col gap-1">
              <Label htmlFor="igdb-id">Twitch client id</Label>
              <Input
                id="igdb-id"
                placeholder="from dev.twitch.tv/console/apps"
                value={clientId}
                onChange={(e) => setClientId(e.target.value)}
              />
            </div>
            <div className="flex min-w-[14rem] flex-1 flex-col gap-1">
              <Label htmlFor="igdb-secret">Twitch client secret</Label>
              <Input
                id="igdb-secret"
                type="password"
                placeholder="never shown again once saved"
                value={clientSecret}
                onChange={(e) => setClientSecret(e.target.value)}
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="primary" disabled={save.isPending}>
              Save &amp; test
            </Button>
            <Button type="button" variant="quiet" onClick={() => runTest.mutate()} disabled={runTest.isPending}>
              Test
            </Button>
            <ConfirmDialog
              trigger={
                <Button type="button" variant="danger">
                  Remove saved
                </Button>
              }
              title="Remove the saved IGDB credentials?"
              body="The environment's, if any, take over."
              confirmLabel="Remove"
              danger
              onConfirm={() => clear.mutate()}
            />
          </div>
        </form>

        <p className="text-[12px] italic text-mist">
          Saved here the pair is encrypted at rest and wins over IGDB_CLIENT_ID/IGDB_CLIENT_SECRET in the environment;
          removing it falls back to those. Companions never hold it — they ask this service for covers.
        </p>
      </div>
    </>
  );
}
