import Link from "next/link";
import { buttonClass } from "@/components/ui";
import { SearchIcon, SlidersIcon, BriefcaseIcon, WalletIcon, ClockIcon } from "@/components/icons";

export default function ProjectsPage() {
  const projects = [
    {
      id: "1",
      title: "React Developer for E-commerce Platform",
      summary: "Build a modern e-commerce frontend with React and TypeScript",
      budget: { min: 2000, max: 5000 },
      type: "fixed_price",
      posted: "2 hours ago",
      bids: 12,
      skills: ["React", "TypeScript", "Next.js", "Tailwind"],
    },
    {
      id: "2",
      title: "Mobile App UI/UX Design",
      summary: "Design a fitness tracking app for iOS and Android",
      budget: { min: 1500, max: 3000 },
      type: "fixed_price",
      posted: "5 hours ago",
      bids: 8,
      skills: ["UI/UX", "Figma", "Mobile Design", "Prototyping"],
    },
    {
      id: "3",
      title: "Backend API Development",
      summary: "Build REST API for inventory management system",
      budget: { hourly: 50 },
      type: "hourly",
      posted: "1 day ago",
      bids: 15,
      skills: ["Node.js", "PostgreSQL", "Docker", "AWS"],
    },
    {
      id: "4",
      title: "Content Writing - Tech Blog",
      summary: "Write 10 articles about AI and machine learning",
      budget: { min: 800, max: 1500 },
      type: "fixed_price",
      posted: "2 days ago",
      bids: 5,
      skills: ["Content Writing", "Tech Writing", "SEO", "AI"],
    },
    {
      id: "5",
      title: "DevOps Setup & CI/CD Pipeline",
      summary: "Set up CI/CD pipeline for web application",
      budget: { hourly: 75 },
      type: "hourly",
      posted: "3 days ago",
      bids: 9,
      skills: ["Kubernetes", "GitHub Actions", "AWS", "Terraform"],
    },
    {
      id: "6",
      title: "Social Media Marketing Strategy",
      summary: "Develop 3-month social media marketing plan",
      budget: { min: 1200, max: 2500 },
      type: "fixed_price",
      posted: "4 days ago",
      bids: 7,
      skills: ["Social Media", "Marketing", "Content Strategy", "Analytics"],
    },
  ];

  const categories = [
    { name: "Web Development", count: 42 },
    { name: "Mobile Development", count: 28 },
    { name: "Design & Creative", count: 35 },
    { name: "Writing & Content", count: 19 },
    { name: "Marketing & Sales", count: 23 },
    { name: "DevOps & Cloud", count: 31 },
  ];

  return (
    <div className="mx-auto max-w-board px-5 py-8 sm:px-8">
      {/* Hero Section */}
      <div className="mb-10 text-center">
        <h1 className="font-display text-[clamp(2.5rem,6vw,3.5rem)] font-semibold leading-[1.02] tracking-[-0.035em] text-ink">
          Find freelance projects worth
          <br className="hidden sm:block" />{" "}
          <span className="relative inline-block">
            shouting about
            <span
              aria-hidden
              className="absolute -bottom-1 left-0 h-[0.18em] w-full rounded-full bg-shout/70"
            />
          </span>
        </h1>
        <p className="mx-auto mt-5 max-w-xl text-base leading-relaxed text-mute sm:text-lg">
          Browse short-term projects, submit your bids, and work with clients on flexible terms.
        </p>
      </div>

      {/* Stats */}
      <div className="mx-auto mb-10 grid max-w-2xl grid-cols-1 gap-px overflow-hidden rounded-card border border-line bg-line sm:grid-cols-4">
        <div className="bg-surface p-6 text-center">
          <div className="font-display text-2xl font-semibold text-ink">156</div>
          <div className="mt-1 text-sm text-mute">Active Projects</div>
        </div>
        <div className="bg-surface p-6 text-center">
          <div className="font-display text-2xl font-semibold text-ink">$42K</div>
          <div className="mt-1 text-sm text-mute">Average Budget</div>
        </div>
        <div className="bg-surface p-6 text-center">
          <div className="font-display text-2xl font-semibold text-ink">824</div>
          <div className="mt-1 text-sm text-mute">Freelancers</div>
        </div>
        <div className="bg-surface p-6 text-center">
          <div className="font-display text-2xl font-semibold text-ink">4.8</div>
          <div className="mt-1 text-sm text-mute">Avg. Rating</div>
        </div>
      </div>

      {/* Search & Filters */}
      <div className="mb-8">
        <div className="flex items-center gap-4">
          <div className="flex-1">
            <div className="relative">
              <SearchIcon className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-mute" />
              <input
                type="search"
                placeholder="Search projects (React, design, writing, etc.)"
                className="w-full rounded-pill border border-line bg-bg py-2.5 pl-10 pr-4 text-ink placeholder:text-mute"
              />
            </div>
          </div>
          <button className={buttonClass("secondary", "md")}>
            <SlidersIcon className="h-4 w-4" />
            Filters
          </button>
          <Link href="/projects/post" className={buttonClass("primary", "md")}>
            <BriefcaseIcon className="h-4 w-4" />
            Post Project
          </Link>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
        {/* Main Content */}
        <div className="lg:col-span-2">
          <div className="space-y-6">
            {projects.map((project) => (
              <div key={project.id} className="rounded-card border border-line bg-surface p-6">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-display text-xl font-semibold text-ink">
                      <Link href={`/projects/${project.id}`} className="hover:text-shout">
                        {project.title}
                      </Link>
                    </h3>
                    <p className="mt-2 text-body">{project.summary}</p>

                    <div className="mt-4 flex flex-wrap items-center gap-4 text-sm">
                      <span className="inline-flex items-center gap-1.5 rounded-pill bg-brand/10 px-2.5 py-1 font-medium text-brand">
                        <WalletIcon className="h-3 w-3" />
                        {project.budget.hourly
                          ? `$${project.budget.hourly}/hr`
                          : `$${project.budget.min} - $${project.budget.max}`}
                      </span>
                      <span className="inline-flex items-center gap-1.5 rounded-pill bg-line/50 px-2.5 py-1 text-mute">
                        <ClockIcon className="h-3 w-3" />
                        {project.type.replace('_', ' ')}
                      </span>
                      <span className="text-mute">{project.posted}</span>
                      <span className="text-mute">{project.bids} bids</span>
                    </div>

                    <div className="mt-4">
                      <div className="flex flex-wrap gap-1.5">
                        {project.skills.map((skill) => (
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

                  <div className="ml-4">
                    <Link
                      href={`/projects/${project.id}/bid`}
                      className={buttonClass("primary", "sm")}
                    >
                      Submit Bid
                    </Link>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-8 text-center">
            <button className={buttonClass("secondary", "md")}>
              Load More Projects
            </button>
          </div>
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          {/* Categories */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Categories</h3>
            <div className="mt-4 space-y-2">
              {categories.map((category) => (
                <a
                  key={category.name}
                  href="#"
                  className="flex items-center justify-between rounded-pill px-3 py-2 text-sm hover:bg-bg"
                >
                  <span className="text-body">{category.name}</span>
                  <span className="rounded-full bg-line px-2 py-0.5 text-xs">
                    {category.count}
                  </span>
                </a>
              ))}
            </div>
          </div>

          {/* Quick Stats */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Project Stats</h3>
            <div className="mt-4 space-y-4">
              <div>
                <div className="flex justify-between text-sm">
                  <span className="text-mute">Fixed Price</span>
                  <span className="font-medium">68%</span>
                </div>
                <div className="mt-1 h-1.5 w-full rounded-full bg-line">
                  <div className="h-full w-2/3 rounded-full bg-brand" />
                </div>
              </div>
              <div>
                <div className="flex justify-between text-sm">
                  <span className="text-mute">Hourly</span>
                  <span className="font-medium">32%</span>
                </div>
                <div className="mt-1 h-1.5 w-full rounded-full bg-line">
                  <div className="h-full w-1/3 rounded-full bg-green-500" />
                </div>
              </div>
            </div>
          </div>

          {/* CTA for Freelancers */}
          <div className="rounded-card border border-shout/30 bg-shout/5 p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Start Freelancing</h3>
            <p className="mt-2 text-sm text-body">
              Create your freelancer profile to start bidding on projects and build your portfolio.
            </p>
            <Link href="/freelancers/create" className={buttonClass("primary", "sm", "mt-4 w-full")}>
              Create Profile
            </Link>
          </div>

          {/* CTA for Clients */}
          <div className="rounded-card border border-line bg-surface p-6">
            <h3 className="font-display text-lg font-semibold text-ink">Need work done?</h3>
            <p className="mt-2 text-sm text-body">
              Post your project and get competitive bids from talented freelancers.
            </p>
            <Link href="/projects/post" className={buttonClass("secondary", "sm", "mt-4 w-full")}>
              Post a Project
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}