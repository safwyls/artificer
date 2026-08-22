export function RelicMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} role="img" aria-label="Reliquary">
      <path
        d="M8 8.3 9.7 11.1 6.3 12.6zM24 8.3 25.7 12.6 22.3 11.1zM16 4.4 17.9 9.1 16 8.2 14.1 9.1z"
        fill="currentColor"
      />
      <circle cx="8" cy="6.4" r="1.7" fill="currentColor" />
      <circle cx="24" cy="6.4" r="1.7" fill="currentColor" />
      <circle cx="16" cy="2.6" r="1.9" fill="currentColor" />
      <path
        d="M4 13.7 16 8.2l12 5.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      <path d="M6.5 15.6h19" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      <path d="M8 15.8v8.6M24 15.8v8.6" fill="none" stroke="currentColor" strokeWidth="2" />
      <rect x="11" y="17.8" width="10" height="5.6" rx="2.8" fill="currentColor" />
      <rect x="4.5" y="24.4" width="23" height="2.4" rx="1.2" fill="currentColor" />
      <rect x="7" y="27" width="3.4" height="1.9" rx="0.7" fill="currentColor" />
      <rect x="21.6" y="27" width="3.4" height="1.9" rx="0.7" fill="currentColor" />
    </svg>
  );
}
