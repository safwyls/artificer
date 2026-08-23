import { useState, type ReactNode } from "react";
import { Button } from "./ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from "./ui/dialog";

/**
 * Replaces window.confirm for the verbs that used to sit behind one. The
 * confirm texts are the page's, kept word for word: each says what the
 * action costs, not just that it is irreversible ("The old holder's late
 * check-in is kept and flagged, not lost", "Nothing is deleted").
 */
export function ConfirmDialog({
  trigger,
  open: openProp,
  onOpenChange,
  title,
  body,
  confirmLabel,
  danger,
  onConfirm,
}: {
  /** Omitted when the caller drives `open` itself — a verb chosen from an
   * overflow menu has no trigger left on screen by the time the dialog
   * opens. */
  trigger?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => void;
}) {
  const [ownOpen, setOwnOpen] = useState(false);
  const controlled = openProp !== undefined;
  const open = controlled ? openProp : ownOpen;
  const setOpen = (next: boolean) => {
    if (!controlled) setOwnOpen(next);
    onOpenChange?.(next);
  };
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {/* The trigger is whatever the caller drew — a quiet button, a menu
          item — so the dialog never dictates how the verb looks. */}
      {trigger ? <span onClick={() => setOpen(true)}>{trigger}</span> : null}
      <DialogContent>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{body}</DialogDescription>
        <div className="mt-6 flex justify-end gap-2">
          <DialogClose asChild>
            <Button variant="quiet">Cancel</Button>
          </DialogClose>
          <Button
            variant={danger ? "danger" : "primary"}
            onClick={() => {
              setOpen(false);
              onConfirm();
            }}
          >
            {confirmLabel}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
