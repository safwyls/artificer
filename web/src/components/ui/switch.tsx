import * as React from "react";
import * as SwitchPrimitives from "@radix-ui/react-switch";
import { cn } from "../../lib/utils";

const Switch = React.forwardRef<
  React.ElementRef<typeof SwitchPrimitives.Root>,
  React.ComponentPropsWithoutRef<typeof SwitchPrimitives.Root>
>(({ className, ...props }, ref) => (
  <SwitchPrimitives.Root
    className={cn(
      // The app's cut-corner language: top-left/bottom-right rounded like
      // the buttons, top-right/bottom-left cut 45° by (border
      // radius still paints inside the clip on the kept corners).
      "peer inline-flex h-[34px] w-16 shrink-0 cursor-pointer items-center rounded-br-[10px] rounded-tl-[10px] border-2 border-transparent shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-fk-spore data-[state=unchecked]:bg-fk-edge",
      className,
    )}
    {...props}
    ref={ref}
  >
    <SwitchPrimitives.Thumb
      className={cn(
        "pointer-events-none ml-0.5 flex h-6 w-6 items-center justify-center rounded-full border-2 border-fk-edge bg-fk-void shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-8 data-[state=unchecked]:translate-x-0",
      )}
    >
      <span className="h-1.5 w-1.5 rounded-full bg-fk-void" />
    </SwitchPrimitives.Thumb>
  </SwitchPrimitives.Root>
));
Switch.displayName = SwitchPrimitives.Root.displayName;

export { Switch };
