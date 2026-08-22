import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, companionURL, errorDetail } from "../lib/api";
import { copyText } from "../lib/format";
import { PageHeader } from "../components/AppShell";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { Button } from "../components/ui/button";

/**
 * The player-side half of save sync: one token, one download. The token is
 * the companion's whole credential, so minting a new one revokes the old
 * everywhere — said out loud, because it is the surprising part.
 */
export function Companion() {
  const queryClient = useQueryClient();
  const token = useQuery({ queryKey: ["sync-token"], queryFn: api.syncToken, retry: false });
  const version = useQuery({ queryKey: ["version"], queryFn: api.version, staleTime: Infinity });

  const after = (msg: string) => () => {
    toast.success(msg);
    queryClient.invalidateQueries({ queryKey: ["sync-token"] });
  };
  const onError = (err: unknown) => toast.error(errorDetail(err));
  const mint = useMutation({
    mutationFn: api.mintSyncToken,
    onSuccess: after("token minted — paste it into your companion"),
    onError,
  });
  const revoke = useMutation({
    mutationFn: api.revokeSyncToken,
    onSuccess: after("token revoked"),
    onError,
  });

  const value = token.data?.token ?? "";

  return (
    <>
      <PageHeader
        title="Your companion"
        subtitle="The Artificer Companion runs on your gaming machine and moves the saves for you."
      />
      <div className="flex max-w-3xl flex-col gap-4 px-4 py-5 md:px-8 md:py-6">
        <div className="rounded-panel border border-edge bg-panel px-5 py-4">
          <h2 className="text-[12px] uppercase tracking-[0.12em] text-gold">Token</h2>
          {token.isLoading ? <p className="mt-3 text-[13px] text-mist">…</p> : null}
          {!token.isLoading && !value ? (
            <div className="mt-3 flex items-center gap-3">
              <Button variant="primary" onClick={() => mint.mutate()} disabled={mint.isPending}>
                Mint a token
              </Button>
              <span className="text-[13px] text-mist">
                Nothing is minted yet — your companion has no way in until one is.
              </span>
            </div>
          ) : null}
          {value ? (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <code className="rounded bg-ink px-2 py-1 font-mono text-[13px] text-parchment">{value}</code>
              <Button
                variant="quiet"
                onClick={async () => {
                  // One attempt, then report it: copying twice to decide
                  // what to say would put the token on the clipboard twice.
                  const ok = await copyText(value);
                  if (ok) toast.success("copied");
                  else toast.error("could not copy — select it by hand");
                }}
              >
                Copy
              </Button>
              <Button asChild variant="quiet">
                <a href={companionURL(value)}>Download the companion</a>
              </Button>
              <ConfirmDialog
                trigger={<Button variant="danger">Revoke</Button>}
                title="Revoke this token?"
                body="Your companion stops being able to reach the vault until you mint a new one and paste it in."
                confirmLabel="Revoke"
                danger
                onConfirm={() => revoke.mutate()}
              />
              <ConfirmDialog
                trigger={<Button variant="quiet">Mint a new one</Button>}
                title="Mint a new token?"
                body="The old one stops working everywhere it is pasted — including on any other machine of yours."
                confirmLabel="Mint"
                onConfirm={() => mint.mutate()}
              />
            </div>
          ) : null}
          <p className="mt-4 text-[12px] italic text-mist">
            The token is yours alone — minting a new one revokes the old everywhere. The download hands out the build
            this service ships, <span className="font-mono not-italic">{version.data?.version ?? "…"}</span>; the
            companion shows its own version in its footer, so you can tell whether a download actually replaced the
            old one.
          </p>
        </div>

        <div className="rounded-panel border border-dashed border-edge bg-well px-5 py-4 text-[13px] text-mist">
          The companion finds installed games, links their save folders to worlds here, and moves the saves. It runs on
          your machine, not on the server — the vault never reaches into a player&apos;s computer, it only answers when
          the companion asks.
        </div>
      </div>
    </>
  );
}
