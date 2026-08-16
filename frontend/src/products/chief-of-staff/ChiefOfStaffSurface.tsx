// Stub for the product-registry wiring pass. Replaced by the real
// ChiefOfStaffSurface (singleton chat, ChatArea-direct like Video Studio) in
// a follow-up pass -- this exists only to prove the switcher/lazy-import/
// routing wiring works end to end before the real chat UI lands.
export function ChiefOfStaffSurface() {
  return (
    <div className="flex h-full w-full items-center justify-center bg-slate-950 text-slate-400">
      <p className="text-sm">Chief of Staff surface — coming soon.</p>
    </div>
  )
}
