import { ChevronDown } from "lucide-react";
import { cn } from "../../lib/utils";

/**
 * Native `<select>` with the app's styling and a custom chevron. The browser's
 * default arrow crowds the right edge; this replaces it with a lucide chevron
 * inset from the border (with matching right padding) so it reads balanced.
 */
export function Select({ className, children, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <div className="relative inline-flex">
      <select
        {...props}
        className={cn(
          "w-full appearance-none rounded-lg border border-wk-edge bg-wk-panel py-1.5 pl-2.5 pr-8 text-sm text-wk-parchment transition-colors focus:border-wk-ember/50 focus:outline-none",
          className,
        )}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-wk-parchment/40" />
    </div>
  );
}
