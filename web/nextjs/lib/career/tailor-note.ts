/** The "Tailored for …" line lives in artifact markdown for the web, not the PDF. */
export function splitCareerTailorNote(body: string): { cv: string; note: string } {
  const lines = (body ?? "").replace(/\s+$/, "").split("\n");
  const last = (lines[lines.length - 1] ?? "").trim();
  if (last.startsWith("Tailored for ")) {
    return { cv: lines.slice(0, -1).join("\n").replace(/\s+$/, ""), note: last };
  }
  return { cv: body ?? "", note: "" };
}
