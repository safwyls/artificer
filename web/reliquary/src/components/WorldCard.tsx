import { Link } from "react-router-dom";
import { CoverArt } from "./CoverArt";
import { CustodyChip, holderLine, requestLine } from "./CustodyChip";
import { ActionButton, OverflowMenu } from "./WorldActions";
import { useAuth } from "../lib/auth";
import { useWorldMutations } from "../lib/mutations";
import { fmtBytes, fmtTime } from "../lib/format";
import { at, worldActions } from "../lib/worldActions";
import type { WorldStatus } from "../lib/types";

/**
 * One world on the shelf: its cover, its custody chip, and the single
 * action that chip calls for. Everything rarer is a click away — the world's
 * own page for history and settings, the overflow menu for the admin verbs.
 */
export function WorldCard({ status }: { status: WorldStatus }) {
  const { username, isAdmin, canSync } = useAuth();
  const w = status.world;
  const { handlers, upload } = useWorldMutations(w.id);
  const actions = worldActions(status, { username, isAdmin, canSync }, handlers);
  const onUpload = (kind: "checkin" | "import", file: File) =>
    upload(kind, file, status.holder?.sessionId);
  const asked = requestLine(status);

  return (
    <div className="flex items-start gap-[18px] rounded-panel border border-edge bg-panel px-5 py-[18px]">
      <Link to={`/worlds/${w.id}`} aria-label={w.name}>
        <CoverArt world={w} />
      </Link>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex flex-wrap items-baseline gap-2.5">
          <Link to={`/worlds/${w.id}`} className="text-[18px] font-bold text-parchment no-underline hover:text-goldhi">
            {w.name}
          </Link>
          {w.gameTitle ? <span className="text-[12px] text-rune">{w.gameTitle}</span> : null}
          <span className="ml-auto font-mono text-[12px] text-mist">
            {status.head
              ? `head v${status.head.id} · ${fmtBytes(status.head.bytes)} · ${fmtTime(status.head.createdAt)}`
              : "no versions yet"}
          </span>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          <CustodyChip status={status} />
          <span className="text-[13px] text-mist">{holderLine(status, username)}</span>
        </div>
        {asked ? <div className="font-mono text-[12px] text-rune">{asked}</div> : null}

        <div className="flex flex-wrap items-center gap-2">
          {at(actions, "card", "primary").map((a) => (
            <ActionButton key={a.id} action={a} placement="primary" onUpload={onUpload} />
          ))}
          {at(actions, "card", "quiet").map((a) => (
            <ActionButton key={a.id} action={a} placement="quiet" onUpload={onUpload} />
          ))}
          <OverflowMenu
            actions={at(actions, "card", "overflow")}
            onUpload={onUpload}
            label={`More actions for ${w.name}`}
          />
        </div>
      </div>
    </div>
  );
}
