import { forwardRef } from "react";
import { cn } from "../../lib/utils";

export const Input = forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        "w-full rounded border border-edge bg-ink px-2.5 py-2 font-serif text-[14px] text-parchment placeholder:text-mist/60",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";
