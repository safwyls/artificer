/**
 * The vault mark: a gold diamond.
 *
 * It used to be the diamond sitting on a wider kite. That composition
 * does not survive being small — at icon sizes it stops reading as a
 * vault and starts reading as a small person, the diamond a head and the
 * kite a body — and the app wears this at 13px more often than anywhere
 * else. The diamond alone is unambiguous at every size the app and the
 * OS ask for.
 *
 * Drawn once here because it is worn at three sizes and in three places:
 * the titlebar, the empty state, and the app/tray icon that
 * companion-desktop/scripts/make-icon.js rasterises from this same
 * shape.
 *
 * The default stroke is the icon's own ratio, not a taste: the icon
 * strokes 8.5% of the tile on a diamond 32% of it, so stroke over radius
 * is 0.266 — here that is 2.4 against a radius of 9. Picked by number
 * because eyeballing it left the strip's mark visibly lighter than the
 * tray's, and one mark drawn two weights is two marks.
 */
export function VaultMark({
  className,
  strokeWidth = 2.4,
}: {
  className?: string;
  strokeWidth?: number;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinejoin="round"
      className={className}
      aria-hidden
    >
      <path d="M12 3l9 9-9 9-9-9z" />
    </svg>
  );
}
