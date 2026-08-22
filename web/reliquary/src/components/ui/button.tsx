import { forwardRef } from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

// Three buttons, and only three. Primary is gold outline on a dark gold
// gradient and there is *one per custody state* — never several competing
// for the same decision. Quiet is mist on an edge border, for everything
// else. Danger is quiet that turns ember when you reach for it.
const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded whitespace-nowrap font-serif transition-colors disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        primary:
          "border border-gold bg-gradient-to-b from-[#2a2416] to-[#1e1a10] font-bold tracking-[0.04em] text-gold hover:text-goldhi hover:border-goldhi",
        quiet: "border border-edge text-mist hover:border-gold/60 hover:text-parchment",
        danger: "border border-ember/40 text-mist hover:border-ember hover:text-ember",
      },
      size: {
        default: "px-4 py-1.5 text-[13px]",
        lg: "px-[18px] py-2 text-[14px]",
        sm: "px-3 py-1 text-[12px]",
        icon: "px-2.5 py-1.5 text-[13px]",
      },
    },
    defaultVariants: { variant: "quiet", size: "default" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp className={cn(buttonVariants({ variant, size }), className)} ref={ref} {...props} />
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
