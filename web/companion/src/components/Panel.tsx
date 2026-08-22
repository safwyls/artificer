import { Component, type ErrorInfo, type ReactNode } from "react";

/**
 * Per-panel failure isolation. One section throwing used to take the
 * whole page with it — the shelf, the scan trail and the version line all
 * sat downstream of a single unguarded read, so one null turned into
 * three bugs that looked unrelated. A panel that fails now fails alone,
 * and names itself while doing it.
 */
export class PanelBoundary extends Component<
  { name: string; children: ReactNode },
  { message: string | null }
> {
  state = { message: null as string | null };

  static getDerivedStateFromError(err: unknown) {
    return { message: err instanceof Error ? err.message : String(err) };
  }

  componentDidCatch(err: Error, info: ErrorInfo) {
    console.error(`${this.props.name} panel:`, err, info.componentStack);
  }

  render() {
    if (this.state.message !== null) {
      return (
        <div className="rounded-panel border border-ember/50 bg-panel px-4 py-3 text-[13px] text-ember">
          The {this.props.name} panel failed: {this.state.message}
          <div className="mt-1 text-[12px] text-mist">
            The rest of this page still works. Reopening the companion usually clears it.
          </div>
        </div>
      );
    }
    return <>{this.props.children}</>;
  }
}

/** A section heading: the letterspaced gold label the whole app uses,
 * with room for the section's own controls on the right. */
export function SectionHeader({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-2.5">
      <h2 className="text-[12px] uppercase tracking-[0.12em] text-gold">{title}</h2>
      {hint ? <span className="text-[12px] italic text-mist">{hint}</span> : null}
      {children ? <div className="ml-auto flex gap-2">{children}</div> : null}
    </div>
  );
}
