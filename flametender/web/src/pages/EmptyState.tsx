export function EmptyState() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center">
      <div className="h-14 w-14 rounded-full bg-gradient-to-br from-ft-spore to-ft-stonehi opacity-60" />
      <div>
        <p className="font-display text-lg font-bold text-ft-bone">No server selected</p>
        <p className="mt-1 text-sm text-ft-bone/50">Pick a server from the rail, or add one with the + button.</p>
      </div>
    </div>
  );
}
