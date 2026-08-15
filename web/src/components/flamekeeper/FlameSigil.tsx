/**
 * The Flamekeeper signature: the server as a flame, its player slots as a
 * ring of pips around it — the Flameborn seated around the fire. Lit pips
 * are players online; the flame breathes while the server is up (stilled
 * under prefers-reduced-motion via the .fk-flame-lit rule in index.css)
 * and gutters to an unlit wisp when it is down. Colors are the fk.*
 * literals from docs/design.md — flame azure is live-state's color and
 * nobody else's.
 */
export function FlameSigil({
  lit,
  total = 16,
  online,
  size = 132,
}: {
  lit: number;
  total?: number;
  online: boolean;
  size?: number;
}) {
  const radius = 57;
  const occupied = Math.max(0, Math.min(lit, total));
  const pips = Array.from({ length: total }, (_, i) => {
    // Start at the top and walk clockwise, like seats filling around a fire.
    const angle = (i / total) * 2 * Math.PI - Math.PI / 2;
    return {
      x: 66 + radius * Math.cos(angle),
      y: 66 + radius * Math.sin(angle),
      occupied: i < occupied,
    };
  });

  return (
    <svg
      viewBox="0 0 132 132"
      width={size}
      height={size}
      role="img"
      aria-label={
        online
          ? `${occupied} of ${total} player slots occupied, server online`
          : "Server offline"
      }
    >
      {pips.map((p, i) => (
        <circle
          key={i}
          cx={p.x}
          cy={p.y}
          r={p.occupied ? 3.4 : 2.4}
          fill={p.occupied ? "#7fc3f0" : "#2d3b32"}
          style={
            p.occupied
              ? { filter: "drop-shadow(0 0 3px rgba(127,195,240,.6))" }
              : undefined
          }
        />
      ))}

      {/* The hearth: a stone circle holding the fire. */}
      <circle cx="66" cy="66" r="42" fill="#131a15" stroke="#98917c" strokeWidth="1.4" />
      <path d="M50 88h32" stroke="#2d3b32" strokeWidth="3" strokeLinecap="round" />

      {online ? (
        <g className="fk-flame-lit">
          <path
            d="M66 42c3 9 12 13 12 24a12 12 0 0 1-24 0c0-6 3-9 5-13 2 3 3 5 5 6-1-6 0-11 2-17z"
            fill="#7fc3f0"
          />
          <path
            d="M66 60c2 4 6 6 6 11a6 6 0 0 1-12 0c0-5 4-7 6-11z"
            fill="#d3ecff"
          />
        </g>
      ) : (
        /* Unlit: a wisp of smoke off cold coals. */
        <g opacity="0.5">
          <path
            d="M63 82c-1-6 5-8 4-13-1-4-4-6-3-11 4 3 7 7 6 12-1 4-4 6-3 12z"
            fill="none"
            stroke="#87947e"
            strokeWidth="1.6"
            strokeLinecap="round"
          />
          <path d="M56 84q10 5 20 0" stroke="#87947e" strokeWidth="2" fill="none" strokeLinecap="round" />
        </g>
      )}
    </svg>
  );
}
