import Link from "next/link";
import { buttonClass } from "@/components/ui";
import { SearchIcon, SlidersIcon, StarIcon, CheckCircleIcon, GlobeIcon, WalletIcon } from "@/components/icons";

export default function FreelancersPage() {
  const freelancers = [
    {
      id: "1",
      name: "Alex Chen",
      title: "Full Stack Developer",
      rate: { min: 60, max: 90 },
      score: 4.9,
      projects: 42,
      skills: ["React", "Node.js", "TypeScript", "AWS", "PostgreSQL"],
      verified: true,
      availability: "full_time",
    },
    {
      id: "2",
      name: "Maria Rodriguez",
      title: "UI/UX Designer",
      rate: { min: 50, max: 75 },
      score: 4.8,
      projects: 28,
      skills: ["Figma", "UI Design", "UX Research", "Prototyping"],
      verified: true,
      availability: "full_time",
    },
    {
      id: "3",
      name: "James Wilson",
      title: "DevOps Engineer",
      rate: { min: 80, max: 120 },
      score: 4.7,
      projects: 35,
      skills: ["Kubernetes", "AWS", "Docker", "Terraform", "CI/CD"],
      verified: true,
      availability: "part_time",
    },
    {
      id: "4",
      name: "Sarah Johnson",
      title: "Content Writer",
      rate: { min: 40, max: 65 },
      score: 4.9,
      projects: 56,
      skills: ["Content Writing", "SEO", "Blogging", "Copywriting"],
      verified: false,
      availability: "full_time",
    },
    {
      id: "5",
      name: "David Kim",
      title: "Mobile Developer",
      rate: { min: 70, max: 100 },
      score: 4.6,
      projects: 23,
      skills: ["React Native", "iOS", "Android", "Firebase"],
      verified: true,
      availability: "full_time",
    },
    {
      id: "6",
      name: "Emma Thompson",
      title: "Data Scientist",
      rate: { min: 90, max: 130 },
      score: 4.8,
      projects: 19,
      skills: ["Python", "Machine Learning", "SQL", "Data Analysis"],
      verified: true,
      availability: "part_time",
    },
  ];

  const skills = [
    "React", "Node.js", "TypeScript", "Python", "UI/UX", "DevOps",
    "Mobile", "Content Writing", "Marketing", "Data Science",
  ];

  return (
    <div className="mx-auto max-w-board px-5 py-8 sm:px-8">
      {/* Hero Section */}
      <div className="mb-10 text-center">
        <h1 className="font-display text-[clamp(2.5rem,6vw,3.5rem)] font-semibold leading-[1.02] tracking-[-0.035em] text-ink">
          Find talented freelancers
          <br className="hidden sm:block" />{" "}
          <span className="relative inline-block">
            for your projects
            <span
              aria-hidden
              className="absolute -bottom-1 left-0 h-[0.18em] w-full rounded-full bg-shout/70"
            />
          </span>
        </h1>
        <p className="mx-auto mt-5 max-w-xl text-base leading-relaxed text-mute sm:text-lg">
          Browse verified freelancers with proven track records. Filter by skills, rates, and availability.
        </p>
      </div>

      {/* Search & Filters */}
      <div className="mb-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <div className="flex-1">
            <div className="relative">
              <SearchIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
              <input
                type="search"
                placeholder="Search freelancers (skills, titles, etc.)"
                className="w-full rounded-pill border border-line bg-bg py-2.5 pl-10 pr-4 text-ink placeholder:text-mute"
              />
            </div>
          </div>
          <button className={buttonClass("secondary", "md")}>
            <SlidersIcon className="h-4 w-4" />
            Filters
          </button>
          <Link href="/freelancers/create" className={buttonClass("ghost", "md")}>
            Join as Freelancer
          </Link>
        </div>

        {/* Quick Skills Filter */}
        <div className="mt-4">
          <div className="flex flex-wrap gap-2">
            {skills.map((skill) => (
              <button
                key={skill}
                className="rounded-pill border border-line bg-surface px-3 py-1.5 text-sm font-medium text-body transition-colors hover:border-shout hover:text-shout"
              >
                {skill}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
        {/* Main Content */}
        <div className="lg:col-span-2">
          <div className="space-y-6">
            {freelancers.map((freelancer) => (
              <div key={freelancer.id} className="rounded-card border border-line bg-surface p-6">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-start gap-4">
                      <div className="h-16 w-16 rounded-full bg-line" />
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <h3 className="font-display text-xl font-semibold text-ink">
                            <Link href={`/freelancers/${freelancer.id}`} className="hover:text-shout">
                              {freelancer.name}
                            </Link>
                          </h3>
                          {freelancer.verified && (
                            <CheckCircleIcon className="h-4 w-4 text-green-500" />
                          )}
                        </div>
                        <p className="mt-1 text-body">{freelancer.title}</p>

                        <div className="mt-3 flex flex-wrap items-center gap-4 text-sm">
                          <span className="inline-flex items-center gap-1.5 font-medium text-ink">
                            <WalletIcon className="h-3 w-3" />
                            ${freelancer.rate.min}-${freelancer.rate.max}/hr
                          </span>
                          <span className="inline-flex items-center gap-1.5 text-mute">
                            <StarIcon className="h-3 w-3 text-yellow-500" />
                            {freelancer.score} ({freelancer.projects} projects)
                          </span>
                          <span className="inline-flex items-center gap-1.5 rounded-pill bg-line/50 px-2.5 py-1 text-xs text-mute">
                            <GlobeIcon className="h-3 w-3" />
                            {freelancer.availability.replace('_', ' ')}
                          </span>
                        </div>

                        <div className="mt-4">
                          <div className="flex flex-wrap gap-1.5">
                            {freelancer.skills.map((skill) => (
                              <span
                                key={skill}
                                className="rounded-pill border border-line px-2.5 py-1 text-xs font-medium"
                              >
                                {skill}
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="ml-4">
                    <Link
                      href={`/freelancers/${freelancer.id}/invite`}
                      className={buttonClass("primary", "sm")}
                    >
                      Invite to Project
                    </Link>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-8 text-center">
            <button className={buttonClass("secondary", "md")}>
              Load More Freelancers
            </button>
          </div>
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          {/* Top Skills */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Top Skills</h3>
            <div className="mt-4 space-y-3">
              {[
                { skill: "React", count: 156 },
                { skill: "Node.js", count: 128 },
                { skill: "TypeScript", count: 112 },
                { skill: "UI/UX Design", count: 98 },
                { skill: "Python", count: 87 },
                { skill: "AWS", count: 76 },
              ].map((item) => (
                <div key={item.skill} className="flex items-center justify-between">
                  <span className="text-sm text-body">{item.skill}</span>
                  <span className="text-xs text-mute">{item.count} freelancers</span>
                </div>
              ))}
            </div>
          </div>

          {/* Rate Distribution */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Hourly Rate Ranges</h3>
            <div className="mt-4 space-y-4">
              {[
                { range: "$20-40", percent: 15 },
                { range: "$40-60", percent: 35 },
                { range: "$60-80", percent: 28 },
                { range: "$80+", percent: 22 },
              ].map((item) => (
                <div key={item.range}>
                  <div className="flex justify-between text-sm">
                    <span className="text-mute">{item.range}/hr</span>
                    <span className="font-medium">{item.percent}%</span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-line">
                    <div className="h-full rounded-full bg-brand" style={{ width: `${item.percent}%` }} />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* CTA for Clients */}
          <div className="rounded-card border border-shout/30 bg-shout/5 p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Need a freelancer?</h3>
            <p className="mt-2 text-sm text-body">
              Post your project and receive bids from qualified freelancers within hours.
            </p>
            <Link href="/projects/post" className={buttonClass("primary", "sm", "mt-4 w-full")}>
              Post a Project
            </Link>
          </div>

          {/* Verification Info */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">
              <CheckCircleIcon className="mr-2 inline h-4 w-4 text-green-500" />
              Verified Freelancers
            </h3>
            <p className="mt-2 text-sm text-body">
              Verified freelancers have completed identity verification and have a proven track record of successful projects.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}