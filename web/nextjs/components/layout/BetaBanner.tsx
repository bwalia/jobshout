// Shown on every page until launch. JobShout is in beta: still being tested
// and not yet for sale. Delete this component and its two call sites
// (AppShell, the auth layout) when sales open.
export function BetaBanner() {
  return (
    <aside
      aria-label="Beta notice"
      className="shrink-0 border-b border-border bg-accent px-4 py-1.5 text-center text-xs font-medium text-accent-foreground"
    >
      <span className="mr-2 rounded-full bg-primary px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-primary-foreground">
        Beta
      </span>
      JobShout is still being tested. It will be available to buy soon.
    </aside>
  );
}
