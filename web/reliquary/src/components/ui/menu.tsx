import { forwardRef } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { cn } from "../../lib/utils";

// The overflow menu behind a world card's rare and admin verbs. They stay
// reachable and stop competing with the one action the custody state calls
// for.
export const Menu = DropdownMenu.Root;
export const MenuTrigger = DropdownMenu.Trigger;

export const MenuContent = forwardRef<
  React.ElementRef<typeof DropdownMenu.Content>,
  React.ComponentPropsWithoutRef<typeof DropdownMenu.Content>
>(({ className, align = "end", sideOffset = 6, ...props }, ref) => (
  <DropdownMenu.Portal>
    <DropdownMenu.Content
      ref={ref}
      align={align}
      sideOffset={sideOffset}
      className={cn(
        "z-50 min-w-[13rem] rounded border border-edge bg-panel py-1 text-[13px] shadow-xl",
        className,
      )}
      {...props}
    />
  </DropdownMenu.Portal>
));
MenuContent.displayName = "MenuContent";

export const MenuItem = forwardRef<
  React.ElementRef<typeof DropdownMenu.Item>,
  React.ComponentPropsWithoutRef<typeof DropdownMenu.Item> & { danger?: boolean }
>(({ className, danger, ...props }, ref) => (
  <DropdownMenu.Item
    ref={ref}
    className={cn(
      "cursor-pointer px-3 py-1.5 outline-none",
      danger
        ? "text-mist data-[highlighted]:bg-ember/10 data-[highlighted]:text-ember"
        : "text-mist data-[highlighted]:bg-ink data-[highlighted]:text-parchment",
      className,
    )}
    {...props}
  />
));
MenuItem.displayName = "MenuItem";

export const MenuSeparator = () => <DropdownMenu.Separator className="my-1 h-px bg-edge" />;
