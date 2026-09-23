// Shown on every page until launch. JobShout is in beta: still being tested
// and not yet for sale. Delete this component and its call site in
// app/layout.tsx when sales open.
export function BetaBanner() {
  return (
    <aside
      aria-label="Beta notice"
      className="border-b border-line bg-shout/10 px-5 py-2 text-center text-sm font-medium text-ink"
    >
      <span className="mr-2 rounded-pill bg-shout px-2.5 py-0.5 text-xs font-bold uppercase tracking-wider text-[rgb(var(--on-brand))]">
        Beta
      </span>
      JobShout is still being tested. It will be available to buy soon.
    </aside>
  );
}
