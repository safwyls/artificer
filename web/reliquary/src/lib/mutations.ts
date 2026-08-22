import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, errorDetail } from "./api";
import type { WorldHandlers } from "./worldActions";

/**
 * Every custody verb, wired the same way: run it, say what happened in the
 * words the old page used, and re-read the truth. Failures show the
 * server's own message — a custody refusal explains itself ("held by mira
 * until …"), and paraphrasing it would lose that.
 */
export function useWorldMutations(worldID: number, onDone?: () => void) {
  const queryClient = useQueryClient();

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["worlds"] });
    queryClient.invalidateQueries({ queryKey: ["world", worldID] });
    onDone?.();
  };

  const run = async (fn: () => Promise<unknown>, okMsg?: string) => {
    try {
      await fn();
      if (okMsg) toast.success(okMsg);
    } catch (err) {
      toast.error(errorDetail(err));
    } finally {
      refresh();
    }
  };

  const handlers: WorldHandlers = {
    // Checkout from the browser exists for the take-over-and-inspect cases;
    // the companion is the usual driver.
    checkout: (takeover) =>
      run(
        () => api.checkout(worldID, takeover),
        "checked out — download the head or let your companion fetch it",
      ),
    renew: (sessionID) => run(() => api.renew(sessionID), "hold extended"),
    claim: () =>
      run(() => api.claim(worldID), "you're next — your companion fetches it automatically"),
    unclaim: () => run(() => api.unclaim(worldID), "claim withdrawn"),
    requestHandback: (kind) =>
      run(
        () => api.requestHandback(worldID, kind),
        kind ? "asked — their companion answers within a minute" : "withdrawn",
      ),
    release: () => run(() => api.release(worldID), "released"),
    serverGive: () =>
      run(() => api.serverGive(worldID), "the server holds the world — start it when ready"),
    serverTake: () =>
      run(() => api.serverTake(worldID), "taken back — the server save is the new head"),
    remove: () => run(() => api.deleteWorld(worldID), "deleted"),
  };

  /**
   * Uploads are raw .tar POSTs. A check-in that lands on a hold which can
   * no longer move the head is accepted and *flagged* rather than refused —
   * so the toast has to say so, or the uploader walks away believing their
   * save is the head.
   */
  const upload = async (kind: "checkin" | "import", file: File, sessionID?: number) => {
    const pending = toast.loading("uploading…");
    try {
      const out =
        kind === "checkin"
          ? await api.checkin(sessionID as number, file)
          : await api.importSave(worldID, file);
      if (out.version?.conflict) {
        toast.warning("checked in, but flagged as a conflict — pick a head in the history", {
          id: pending,
        });
      } else {
        toast.success("uploaded", { id: pending });
      }
    } catch (err) {
      toast.error(errorDetail(err), { id: pending });
    } finally {
      refresh();
    }
  };

  return { handlers, upload, refresh };
}
