import { Toaster as Sonner } from "sonner";

/** The old page's one-line status readout, as toasts. Every action says
 * what happened — "hold extended", "asked — their companion answers within
 * a minute" — or why it didn't, in the server's own words. */
export function Toaster() {
  return (
    <Sonner
      position="bottom-right"
      toastOptions={{
        classNames: {
          toast:
            "!bg-panel !border !border-edge !text-parchment !font-serif !rounded !text-[13px]",
          description: "!text-mist",
          success: "!border-ok/50",
          error: "!border-ember/60",
        },
      }}
    />
  );
}
