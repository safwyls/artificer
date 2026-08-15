/**
 * The Flamekeeper signature: a ring of six rune segments, one per player
 * slot (Dragonwilds' hard cap), lit for each adventurer online, around a
 * dragon-eye that pulses while the server is up. Geometry and colors come
 * from the mock; the pulse is stilled under prefers-reduced-motion via the
 * .wk-eye rule in index.css.
 */
export function RuneSigil({
  lit,
  total = 6,
  online,
  size = 132,
}: {
  lit: number;
  total?: number;
  online: boolean;
  size?: number;
}) {
  const radius = 56;
  const circumference = 2 * Math.PI * radius;
  const segment = circumference / total;
  const arc = segment - 8.6; // the mock's gap between segments
  const occupied = Math.max(0, Math.min(lit, total));

  return (
    <svg
      viewBox="0 0 132 132"
      width={size}
      height={size}
      role="img"
      aria-label={
        online
          ? `${occupied} of ${total} adventurer slots occupied, server online`
          : "Server offline"
      }
    >
      <g transform="rotate(-90 66 66)">
        {Array.from({ length: total }, (_, i) => (
          <circle
            key={i}
            cx="66"
            cy="66"
            r={radius}
            fill="none"
            strokeWidth="7"
            strokeLinecap="round"
            strokeDasharray={`${arc} ${circumference - arc}`}
            strokeDashoffset={-i * segment}
            stroke={i < occupied ? "#52d8d0" : "#232d3d"}
            style={
              i < occupied
                ? { filter: "drop-shadow(0 0 4px rgba(82,216,208,.55))" }
                : undefined
            }
          />
        ))}
      </g>
      <circle cx="66" cy="66" r="40" fill="#0d1218" stroke="#8a6f3a" strokeWidth="1.5" />
      <path
        className={online ? "wk-eye" : undefined}
        d="M46 66q20-16 40 0q-20 16-40 0z"
        fill={online ? "#52d8d0" : "#2b6f6c"}
        opacity={online ? undefined : 0.45}
      />
      <ellipse cx="66" cy="66" rx="4" ry="9" fill="#0b0e12" />
      <path
        d="M66 30v8M66 94v8M30 66h8M94 66h8M41 41l5 5M86 86l5 5M91 41l-5 5M46 86l-5 5"
        stroke="#c9a24b"
        strokeWidth="1.3"
        fill="none"
        opacity="0.85"
      />
    </svg>
  );
}
