import { downloadURL } from "../lib/api";
import { fmtBytes, fmtTime } from "../lib/format";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { ConfirmDialog } from "./ConfirmDialog";
import type { Version } from "../lib/types";

/**
 * One kept version. The badges are the whole point of the row: HEAD is what
 * the next player gets, CONFLICT is a check-in from a hold that could no
 * longer move the head — accepted and flagged rather than lost, and waiting
 * for an admin to pick a head — and CHECKPOINT is a mid-session snapshot,
 * which never moved the head in the first place.
 */
export function VersionRow({
  version,
  worldID,
  isHead,
  uploader,
  canSetHead,
  onSetHead,
}: {
  version: Version;
  worldID: number;
  isHead: boolean;
  uploader: string;
  canSetHead: boolean;
  onSetHead: (versionID: number) => void;
}) {
  const conflictNote = version.conflict
    ? " — from a hold that could no longer move the head"
    : "";
  return (
    <div className="flex flex-wrap items-center gap-3.5 border-b border-edge px-[18px] py-3 last:border-b-0">
      <span className="w-[42px] font-mono text-[13px] text-parchment">v{version.id}</span>
      <span className="flex gap-1.5">
        {isHead ? <Badge tone="head">HEAD</Badge> : null}
        {version.conflict ? (
          <Badge tone="conflict" title="Checked in from a hold that could no longer move the head — pick a head to resolve">
            CONFLICT
          </Badge>
        ) : null}
        {version.kind === "checkpoint" ? <Badge tone="checkpoint">CHECKPOINT</Badge> : null}
      </span>
      <span className="text-[13px] text-mist">
        {version.kind} by <span className="text-parchment">{uploader}</span> · {fmtBytes(version.bytes)} ·{" "}
        {fmtTime(version.createdAt)}
        {conflictNote}
      </span>
      <div className="ml-auto flex gap-2">
        <Button asChild variant="quiet" size="sm">
          <a href={downloadURL(worldID, version.id)}>Download</a>
        </Button>
        {canSetHead && !isHead ? (
          <ConfirmDialog
            trigger={
              <Button variant="quiet" size="sm">
                Make head
              </Button>
            }
            title={`Make v${version.id} the canonical head?`}
            body="The next player to check this world out gets this version."
            confirmLabel="Make head"
            onConfirm={() => onSetHead(version.id)}
          />
        ) : null}
      </div>
    </div>
  );
}
