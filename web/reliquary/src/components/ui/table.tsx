import { cn } from "../../lib/utils";

// A table drawn as a panel: a header strip of letterspaced mist, then rows
// separated by the panel's own edge. Not <table>, because every column here
// is either a word or a row of buttons, and grid keeps them aligned across
// rows without colspan arithmetic.

export function TableShell({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("overflow-hidden rounded-panel border border-edge bg-panel", className)}
      {...props}
    />
  );
}

export function TableHead({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      role="row"
      className={cn(
        "border-b border-edge px-[18px] py-2.5 text-[10px] uppercase tracking-[0.12em] text-mist",
        className,
      )}
      {...props}
    />
  );
}

export function TableRow({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      role="row"
      className={cn("items-center px-[18px] py-3 border-b border-edge last:border-b-0", className)}
      {...props}
    />
  );
}
