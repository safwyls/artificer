import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Ban, Megaphone, Power, UserX } from "lucide-react";
import { toast } from "sonner";
import { api, type Player } from "../lib/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { NumberField } from "./ui/number-field";
import { Label } from "./ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

export function BroadcastDialog({
  serverId,
  open,
  onOpenChange,
}: {
  serverId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [message, setMessage] = useState("");

  const broadcast = useMutation({
    mutationFn: (msg: string) => api.broadcast(serverId, msg),
    onSuccess: () => {
      toast.success("Broadcast sent");
      setMessage("");
      onOpenChange(false);
    },
    onError: () => toast.error("Failed to send broadcast"),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Broadcast a message</DialogTitle>
          <DialogDescription>Shown in-game to every player currently online.</DialogDescription>
        </DialogHeader>
        <Input
          placeholder="Server restarting in 10 minutes…"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && message) broadcast.mutate(message);
          }}
          autoFocus
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!message || broadcast.isPending} onClick={() => broadcast.mutate(message)}>
            <Megaphone className="h-4 w-4" />
            {broadcast.isPending ? "Sending..." : "Send"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export type ModerationTarget = { action: "kick" | "ban"; player: Player };

/** Confirm dialog for kick/ban. Ban especially is permanent and sits one
 * misclick from Kick in the player table, so neither fires without this
 * step — and the admin can say why instead of the old hardcoded reason. */
export function ModerationDialog({
  serverId,
  target,
  onClose,
  onDone,
}: {
  serverId: number;
  target: ModerationTarget | null;
  onClose: () => void;
  onDone?: () => void;
}) {
  const [reason, setReason] = useState("");

  useEffect(() => {
    if (target) setReason(target.action === "kick" ? "Kicked by admin" : "Banned by admin");
  }, [target]);

  const act = useMutation({
    mutationFn: ({ action, player }: ModerationTarget) =>
      action === "kick" ? api.kick(serverId, player.playerId, reason) : api.ban(serverId, player.playerId, reason),
    onSuccess: (_, { action, player }) => {
      toast.success(`${action === "kick" ? "Kicked" : "Banned"} ${player.name}`);
      onClose();
      onDone?.();
    },
    onError: (_, { action, player }) => toast.error(`Failed to ${action} ${player.name}`),
  });

  const isBan = target?.action === "ban";

  return (
    <Dialog open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isBan ? "Ban" : "Kick"} {target?.player.name}?
          </DialogTitle>
          <DialogDescription>
            {isBan
              ? "Bans are permanent until lifted with an unban — the player cannot rejoin."
              : "Disconnects the player; they can rejoin right away."}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1">
          <Label className="text-xs">Reason shown to the player</Label>
          <Input value={reason} onChange={(e) => setReason(e.target.value)} autoFocus />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant={isBan ? "destructive" : "default"}
            disabled={act.isPending || !target}
            onClick={() => target && act.mutate(target)}
          >
            {isBan ? <Ban className="h-4 w-4" /> : <UserX className="h-4 w-4" />}
            {act.isPending ? (isBan ? "Banning..." : "Kicking...") : isBan ? "Ban player" : "Kick player"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function ShutdownDialog({
  serverId,
  open,
  onOpenChange,
}: {
  serverId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [waitSeconds, setWaitSeconds] = useState(60);
  const [message, setMessage] = useState("Server restarting soon");

  const shutdown = useMutation({
    mutationFn: () => api.shutdown(serverId, waitSeconds, message),
    onSuccess: () => {
      toast.success("Shutdown initiated");
      onOpenChange(false);
    },
    onError: () => toast.error("Shutdown failed"),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Shut down server?</DialogTitle>
          <DialogDescription>
            Players get the message below, then the server stops after the wait period.
          </DialogDescription>
        </DialogHeader>
        <div className="flex gap-2">
          <div className="w-24 space-y-1">
            <Label className="text-xs">Wait (s)</Label>
            <NumberField value={waitSeconds} onChange={setWaitSeconds} min={0} />
          </div>
          <div className="flex-1 space-y-1">
            <Label className="text-xs">Message</Label>
            <Input value={message} onChange={(e) => setMessage(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={shutdown.isPending} onClick={() => shutdown.mutate()}>
            <Power className="h-4 w-4" />
            {shutdown.isPending ? "Shutting down..." : "Shut down"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
