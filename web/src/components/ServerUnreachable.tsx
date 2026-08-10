export function ServerUnreachable() {
  return (
    <div className="flex flex-col items-center gap-4 px-6 py-16 text-center">
      <svg viewBox="0 0 132 132" width="120" height="120" role="img" aria-label="Server unreachable">
        <circle cx="66" cy="66" r="56" fill="none" stroke="#232d3d" strokeWidth="7" />
        <circle cx="66" cy="66" r="40" fill="#0d1218" stroke="#8a6f3a" strokeWidth="1.5" />
        <path d="M46 66q20-16 40 0q-20 16-40 0z" fill="#2b6f6c" opacity="0.4" />
        <ellipse cx="66" cy="66" rx="4" ry="9" fill="#0b0e12" />
        <path
          d="M66 30v8M66 94v8M30 66h8M94 66h8M41 41l5 5M86 86l5 5M91 41l-5 5M46 86l-5 5"
          stroke="#c9a24b"
          strokeWidth="1.3"
          fill="none"
          opacity="0.5"
        />
      </svg>
      <div>
        <p className="font-wkdisplay text-lg font-semibold tracking-[0.05em] text-wk-parchment">The eye is dark</p>
        <p className="mt-1 max-w-md text-sm text-wk-mist">
          Couldn't reach the server — check that it's running and that its wkagent is reachable from Wildskeeper.
        </p>
      </div>
    </div>
  );
}
