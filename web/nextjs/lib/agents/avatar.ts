// Agent avatar colours: Daybook's muted inks (navy, slate, gold, violet,
// green, oxblood, teal) instead of a rainbow. Each one carries white
// initials at 4.5:1 or better.
export const AVATAR_COLOURS = [
  "bg-[#1D3A5F]", // navy
  "bg-[#5847C2]", // violet
  "bg-[#7A5C1E]", // gold, deepened for white text
  "bg-[#1F6B4F]", // green
  "bg-[#8A3A3E]", // oxblood
  "bg-[#2F6170]", // teal
  "bg-[#46526A]", // slate
  "bg-[#274B78]", // blue
] as const;

export function avatarColour(name: string): string {
  const ch = name.trim().charCodeAt(0);
  if (!Number.isFinite(ch)) return AVATAR_COLOURS[0];
  return AVATAR_COLOURS[ch % AVATAR_COLOURS.length] ?? AVATAR_COLOURS[0];
}
