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
  title,
  body,
  confirmLabel,
  danger,
  onConfirm,
}: {
  trigger: ReactNode;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {/* The trigger is whatever the caller drew — a quiet button, a menu
          item — so the dialog never dictates how the verb looks. */}
      <span onClick={() => setOpen(true)}>{trigger}</span>
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
