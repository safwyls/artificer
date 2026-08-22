import { useEffect, useState } from "react";
import { api, errorText } from "../lib/api";
import { Button } from "./ui/button";
import { cn } from "../lib/utils";
import type { Browse } from "../lib/types";

/**
 * Typing a Windows path into a text box is where the last mile went wrong
 * most often: the wrong slash, a quoted "Copy as path", a folder one
 * level off. Browsing removes the transcription. It lives inside the link
 * form rather than over it, so the folder you pick and the world you are
 * linking it to stay on screen together.
 */
export function FolderBrowser({
  start,
  onUse,
}: {
  start: string;
  onUse: (path: string) => void;
}) {
  const [at, setAt] = useState(start);
  const [browse, setBrowse] = useState<Browse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    api
      .browse(at)
      .then((out) => {
        if (!cancelled) {
          setBrowse(out.browse);
          setError("");
        }
      })
      .catch((err) => {
        if (!cancelled) setError(errorText(err));
      });
    return () => {
      cancelled = true;
    };
  }, [at]);

  const row = "cursor-pointer border-t border-edge/50 px-2.5 py-1 text-[13px] hover:bg-gold/10";

  return (
    <div className="rounded border border-edge bg-ink">
      <div className="break-all border-b border-edge px-2.5 py-1.5 font-mono text-[11px] text-mist">
        {browse?.path ?? at}
        {browse?.error ? <span className="text-ember"> — {browse.error}</span> : null}
        {error ? <span className="text-ember"> — {error}</span> : null}
      </div>
      <div className="max-h-[210px] overflow-y-auto">
        {browse?.parent ? (
          <div className={cn(row, "text-mist")} onClick={() => setAt(browse.parent!)}>
            ↑ up one folder
          </div>
        ) : null}
        {(browse?.entries ?? []).map((e) => (
          <div
            key={e.path}
            // A folder whose name games use for saves is worth pointing
            // at: it is usually the answer.
            className={cn(row, e.saveish && "text-goldhi")}
            onClick={() => setAt(e.path)}
          >
            {e.name}
          </div>
        ))}
        {browse && !browse.entries?.length && !browse.error ? (
          <div className="border-t border-edge/50 px-2.5 py-1 text-[13px] text-mist">
            no folders here
          </div>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-1.5 border-t border-edge px-2.5 py-1.5">
        <Button
          type="button"
          variant="primary"
          size="sm"
          onClick={() => onUse(browse?.path ?? at)}
        >
          Use this folder
        </Button>
        {(browse?.roots ?? []).map((r) => (
          <Button key={r.path} type="button" size="sm" onClick={() => setAt(r.path)}>
            {r.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
