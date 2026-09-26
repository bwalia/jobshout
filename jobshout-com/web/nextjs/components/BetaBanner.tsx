// Shown on every page until launch. JobShout is in beta: still being tested
// and not yet for sale. Delete this component and its call site in
// app/layout.tsx when sales open.
//
// The hard hat is decoration, so it is hidden from screen readers — the
// sentence already carries the whole message without it.
export function BetaBanner() {
  return (
    <aside
      aria-label="Beta notice"
      className="border-b border-line bg-shout/10 px-5 py-2 text-center text-sm font-medium text-ink"
    >
      <span className="mr-2 rounded-pill bg-shout px-2.5 py-0.5 text-xs font-bold uppercase tracking-wider text-[rgb(var(--on-brand))]">
        Beta
      </span>
      <span aria-hidden="true">🏗️</span> Mind the scaffolding — JobShout is still being
      tested. Launching soon, please visit again.
    </aside>
  );
}
