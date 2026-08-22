import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

// The version-row badges: HEAD, CONFLICT, CHECKPOINT — a monospace stamp
// on the page ground, colored by what it means rather than by rank.
const badgeVariants = cva(
  "inline-block rounded-[3px] bg-ink px-2 py-0.5 font-mono text-[10px] tracking-[0.06em]",
  {
    variants: {
      tone: {
        head: "text-goldhi",
        conflict: "text-ember",
        checkpoint: "text-rune",
        muted: "text-mist",
      },
    },
    defaultVariants: { tone: "muted" },
  },
);

export function Badge({
  className,
  tone,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />;
}
