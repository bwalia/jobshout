import Link from "next/link";
import { Logo } from "@/components/Logo";
import {
  formatDeployIdentity,
  readDeployIdentity,
} from "@/lib/deploy-identity";

const COLUMNS = [
  {
    title: "Find",
    links: [
      { href: "/jobs", label: "Jobs" },
      { href: "/members", label: "Talents" },
      { href: "/agents", label: "Agents" },
      { href: "/groups", label: "Groups" },
      { href: "/insights", label: "Insights" },
      { href: "/showcase", label: "AI Showcase" },
    ],
  },
  {
    title: "For you",
    links: [
      { href: "/profile", label: "Matching profile" },
      { href: "/applications", label: "My applications" },
      { href: "/jobs/saved", label: "Saved jobs" },
      { href: "/messages", label: "Messages" },
      { href: "/following", label: "Following" },
    ],
  },
  {
    title: "Hiring",
    links: [
      { href: "/post-job", label: "Post a job" },
      { href: "/jobs/mine", label: "My posted jobs" },
      { href: "/hire", label: "Linked Hire" },
      { href: "/projects", label: "Projects" },
    ],
  },
  {
    title: "Insights",
    links: [
      { href: "/insights", label: "Latest on AI & work" },
      { href: "/newsletter", label: "Weekly newsletter" },
      { href: "/insights/new", label: "Write for Insights" },
      { href: "/showcase/new", label: "Showcase your app" },
    ],
  },
];

export function SiteFooter() {
  const deployLine = formatDeployIdentity(readDeployIdentity());

  return (
    <footer className="mt-24 border-t border-line bg-subtle">
      <div className="mx-auto grid max-w-board gap-10 px-5 py-14 sm:px-8 md:grid-cols-2 lg:grid-cols-[1.4fr_1fr_1fr_1fr_1fr]">
        <div className="max-w-sm">
          <Logo />
          <p className="mt-4 text-sm leading-relaxed text-mute">
            A simple search for jobs, AI agents, talents, groups, and trends — with humans in the
            loop on every move that matters.
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
          <div className="flex flex-col gap-1 sm:items-end">
            <p>Agents propose. Humans decide.</p>
            {deployLine ? (
              <p className="font-mono text-[11px] leading-tight text-mute/80" title={deployLine}>
                {deployLine}
              </p>
            ) : null}
          </div>
        </div>
      </div>
    </footer>
  );
}
