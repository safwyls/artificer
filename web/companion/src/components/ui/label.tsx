import { forwardRef } from "react";
import * as LabelPrimitive from "@radix-ui/react-label";
import { cn } from "../../lib/utils";

/** The one field label: small, letterspaced, mist — uppercased by the
 * caller's own text, not by CSS, so a label reads the same in the DOM as
 * on screen (and a test can find it by the words it says). */
export const Label = forwardRef<
  React.ElementRef<typeof LabelPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof LabelPrimitive.Root>
>(({ className, ...props }, ref) => (
  <LabelPrimitive.Root
    ref={ref}
    className={cn("block text-[11px] uppercase tracking-[0.1em] text-mist", className)}
    {...props}
  />
));
Label.displayName = "Label";
