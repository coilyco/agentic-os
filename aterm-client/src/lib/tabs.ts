/** Roving focus for a tablist: arrows either way, Home, and End move between tabs. */
export function tablistKeys(event: KeyboardEvent): void {
  const list = (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role="tab"]');
  const tabs = Array.from(list);
  const current = tabs.indexOf(document.activeElement as HTMLElement);
  if (current === -1) return;
  const next = { ArrowDown: current + 1, ArrowRight: current + 1, ArrowUp: current - 1, ArrowLeft: current - 1, Home: 0, End: tabs.length - 1 }[event.key];
  if (next === undefined) return;
  event.preventDefault();
  tabs[(next + tabs.length) % tabs.length]?.focus();
}
