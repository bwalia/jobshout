import Link from "next/link";
import { Logo } from "@/components/Logo";

const COLUMNS = [
  {
    title: "Candidates",
    links: [
      { href: "/jobs", label: "Browse jobs" },
      { href: "/profile", label: "Matching profile" },
      { href: "/profile/matches", label: "Ranked matches" },
      { href: "/applications", label: "My applications" },
    ],
  },
  {
    title: "Employers",
    links: [
      { href: "/post-job", label: "Post a job" },
      { href: "/jobs", label: "See the board" },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="mt-24 border-t border-line bg-subtle">
      <div className="mx-auto grid max-w-board gap-10 px-5 py-14 sm:px-8 md:grid-cols-[1.4fr_1fr_1fr]">
        <div className="max-w-sm">
          <Logo />
          <p className="mt-4 text-sm leading-relaxed text-mute">
            The AI-native employment marketplace. Career Agents find roles, Hiring Agents find
            people, and a human approves every move.
          </p>
        </div>

        {COLUMNS.map((col) => (
          <div key={col.title}>
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">
              {col.title}
            </p>
            <ul className="mt-3 space-y-0.5">
              {col.links.map((link) => (
                <li key={link.href}>
                  <Link
                    href={link.href}
                    className="-mx-2 inline-flex min-h-[44px] items-center px-2 text-sm text-body transition-colors duration-200 hover:text-shout sm:min-h-[36px]"
                  >
                    {link.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>

      <div className="border-t border-line">
        <div className="mx-auto flex max-w-board flex-col gap-2 px-5 py-6 text-xs text-mute sm:flex-row sm:items-center sm:justify-between sm:px-8">
          <p>© {new Date().getFullYear()} JobShout.com</p>
          <p>Agents propose. Humans decide.</p>
        </div>
      </div>
    </footer>
  );
}
