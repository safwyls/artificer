import { useRef } from "react";
import { MoreHorizontal } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "./ui/button";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "./ui/menu";
import { ConfirmDialog } from "./ConfirmDialog";
import type { WorldAction } from "../lib/worldActions";

export type UploadHandler = (kind: "checkin" | "import", file: File) => void;

/** The .tar picker behind "Check in…" and "Import…". Save bundles are raw
 * tars posted as the request body; nothing here reads them. */
function useTarPicker(onPick: (file: File) => void) {
  const ref = useRef<HTMLInputElement | null>(null);
  const input = (
    <input
      ref={ref}
      type="file"
      accept=".tar,application/x-tar"
      className="hidden"
      onChange={(e) => {
        const file = e.target.files?.[0];
        // Clear first: picking the same file twice in a row must fire again.
        e.target.value = "";
        if (file) onPick(file);
      }}
    />
  );
  return { input, open: () => ref.current?.click() };
}

function variantFor(action: WorldAction, placement: "primary" | "quiet") {
  if (action.danger) return "danger" as const;
  return placement === "primary" ? ("primary" as const) : ("quiet" as const);
}

/** One action as a button (or a link, for downloads and navigation), with
 * its confirm dialog and file picker wired if it wants one. */
export function ActionButton({
  action,
  placement,
  onUpload,
}: {
  action: WorldAction;
  placement: "primary" | "quiet";
  onUpload: UploadHandler;
}) {
  const variant = variantFor(action, placement);
  const picker = useTarPicker((file) => action.upload && onUpload(action.upload, file));

  if (action.href) {
    // A download must be a real link so the browser streams it; an in-app
    // destination must be a router link so it doesn't reload the bundle.
    const internal = action.href.startsWith("/") && !action.href.startsWith("/api");
    return (
      <Button asChild variant={variant}>
        {internal ? (
          <Link to={action.href}>{action.label}</Link>
        ) : (
          <a href={action.href}>{action.label}</a>
        )}
      </Button>
    );
  }
  if (action.upload) {
    return (
      <>
        {picker.input}
        <Button variant={variant} onClick={picker.open}>
          {action.label}
        </Button>
      </>
    );
  }
  if (action.confirm) {
    return (
      <ConfirmDialog
        trigger={<Button variant={variant}>{action.label}</Button>}
        title={action.confirm.title}
        body={action.confirm.body}
        confirmLabel={action.confirm.confirmLabel}
        danger={action.danger}
        onConfirm={() => action.run?.()}
      />
    );
  }
  return (
    <Button variant={variant} onClick={() => action.run?.()}>
      {action.label}
    </Button>
  );
}

/** The overflow menu: the admin and rare verbs, out of the way of the one
 * action the world's custody state actually calls for. */
export function OverflowMenu({
  actions,
  onUpload,
  label = "More actions",
}: {
  actions: WorldAction[];
  onUpload: UploadHandler;
  label?: string;
}) {
  const picker = useTarPicker(() => {});
  if (!actions.length) return null;
  return (
    <Menu>
      <MenuTrigger asChild>
        <Button variant="quiet" size="icon" aria-label={label}>
          <MoreHorizontal className="h-3.5 w-3.5" aria-hidden />
        </Button>
      </MenuTrigger>
      <MenuContent>
        {picker.input}
        {actions.map((action, i) => (
          <div key={action.id}>
            {/* The destructive verbs sit below a rule, so a mis-aimed click
                lands on nothing rather than on Delete. */}
            {action.danger && !actions[i - 1]?.danger ? <MenuSeparator /> : null}
            <OverflowItem action={action} onUpload={onUpload} />
          </div>
        ))}
      </MenuContent>
    </Menu>
  );
}

function OverflowItem({ action, onUpload }: { action: WorldAction; onUpload: UploadHandler }) {
  const picker = useTarPicker((file) => action.upload && onUpload(action.upload, file));
  if (action.href) {
    const internal = action.href.startsWith("/") && !action.href.startsWith("/api");
    return (
      <MenuItem asChild danger={action.danger}>
        {internal ? (
          <Link to={action.href}>{action.label}</Link>
        ) : (
          <a href={action.href}>{action.label}</a>
        )}
      </MenuItem>
    );
  }
  if (action.upload) {
    return (
      <>
        {picker.input}
        <MenuItem danger={action.danger} onSelect={() => picker.open()}>
          {action.label}
        </MenuItem>
      </>
    );
  }
  if (action.confirm) {
    return (
      <ConfirmDialog
        trigger={
          // The menu closes on select; the dialog opens from the same click.
          <MenuItem danger={action.danger} onSelect={(e) => e.preventDefault()}>
            {action.label}
          </MenuItem>
        }
        title={action.confirm.title}
        body={action.confirm.body}
        confirmLabel={action.confirm.confirmLabel}
        danger={action.danger}
        onConfirm={() => action.run?.()}
      />
    );
  }
  return (
    <MenuItem danger={action.danger} onSelect={() => action.run?.()}>
      {action.label}
    </MenuItem>
  );
}
