export function EmptyState() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center">
      <div className="clip-notch h-14 w-14 rounded-full bg-gradient-to-br from-wk-ember to-wk-brasshi opacity-60" />
      <div>
        <p className="font-display text-lg font-bold text-wk-parchment">No server selected</p>
        <p className="mt-1 text-sm text-wk-parchment/50">Pick a server from the rail, or add one with the + button.</p>
      </div>
    </div>
  );
}
