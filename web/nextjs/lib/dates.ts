/** Parse a date-only string (YYYY-MM-DD) as local midnight, not UTC. */
export function parseDateOnly(iso: string): Date {
  if (/^\d{4}-\d{2}-\d{2}$/.test(iso)) {
    return new Date(`${iso}T00:00:00`);
  }
  return new Date(iso);
}

/** Format a date-only API value in the user's locale without an off-by-one. */
export function formatDateOnly(
  iso: string,
  opts: Intl.DateTimeFormatOptions = { day: "numeric", month: "short" }
): string {
  return parseDateOnly(iso).toLocaleDateString(undefined, opts);
}

/** True when a due date has passed. Date-only values compare against end of that local day. */
export function isDueOverdue(iso: string | null | undefined): boolean {
  if (!iso) return false;
  if (/^\d{4}-\d{2}-\d{2}$/.test(iso)) {
    const end = parseDateOnly(iso);
    end.setHours(23, 59, 59, 999);
    return Date.now() > end.getTime();
  }
  const d = new Date(iso);
  return !Number.isNaN(d.getTime()) && d.getTime() < Date.now();
}
