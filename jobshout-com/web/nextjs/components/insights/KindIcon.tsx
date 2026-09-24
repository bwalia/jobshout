import type { SVGProps } from "react";
import { BookIcon, MessageIcon, MicIcon, NewspaperIcon, PlayIcon } from "@/components/icons";
import type { InsightKind } from "@/lib/insights";

const ICONS = {
  post: MessageIcon,
  article: NewspaperIcon,
  blog: BookIcon,
  podcast: MicIcon,
  video: PlayIcon,
} as const;

export function KindIcon({ kind, ...props }: { kind: InsightKind } & SVGProps<SVGSVGElement>) {
  const Icon = ICONS[kind];
  return <Icon {...props} />;
}
