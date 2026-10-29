export type ProjectBudget =
  | { kind: "fixed"; min: number; max: number }
  | { kind: "hourly"; rate: number };

export type MockProject = {
  id: string;
  title: string;
  summary: string;
  description: string;
  budget: ProjectBudget;
  type: "fixed_price" | "hourly" | "retainer" | "milestone";
  posted: string;
  bids: number;
  skills: string[];
  duration: string;
  deliverables: string[];
  client: {
    name: string;
    location: string;
    projectsPosted: number;
    hireRate: string;
  };
};

export const MOCK_PROJECTS: MockProject[] = [
  {
    id: "1",
    title: "React Developer for E-commerce Platform",
    summary: "Build a modern e-commerce frontend with React and TypeScript",
    description:
      "We're rebuilding the storefront for a mid-size online retailer. You will own the product listing, cart, and checkout flows in Next.js App Router, wire them to our existing GraphQL catalog API, and ship a responsive design that matches our Figma kit. Expect weekly demos and a clear acceptance checklist per milestone.",
    budget: { kind: "fixed", min: 2000, max: 5000 },
    type: "fixed_price",
    posted: "2 hours ago",
    bids: 12,
    skills: ["React", "TypeScript", "Next.js", "Tailwind"],
    duration: "3–4 weeks",
    deliverables: [
      "Product listing + detail pages",
      "Cart and checkout UI",
      "Responsive layout matching Figma",
      "Basic Playwright smoke tests",
    ],
    client: {
      name: "Northline Retail",
      location: "Remote · UK",
      projectsPosted: 8,
      hireRate: "75%",
    },
  },
  {
    id: "2",
    title: "Mobile App UI/UX Design",
    summary: "Design a fitness tracking app for iOS and Android",
    description:
      "Early-stage fitness startup needs a complete UI/UX pass for a cross-platform tracking app. Deliver wireframes, high-fidelity screens, and a clickable prototype covering onboarding, workout logging, and weekly progress.",
    budget: { kind: "fixed", min: 1500, max: 3000 },
    type: "fixed_price",
    posted: "5 hours ago",
    bids: 8,
    skills: ["UI/UX", "Figma", "Mobile Design", "Prototyping"],
    duration: "2–3 weeks",
    deliverables: [
      "User flows and wireframes",
      "Hi-fi Figma screens (light + dark)",
      "Interactive prototype",
      "Design handoff notes",
    ],
    client: {
      name: "Pulse Labs",
      location: "Remote · EU",
      projectsPosted: 3,
      hireRate: "100%",
    },
  },
  {
    id: "3",
    title: "Backend API Development",
    summary: "Build REST API for inventory management system",
    description:
      "Need a production-ready REST API for inventory, SKUs, and warehouse transfers. Prefer Node.js + PostgreSQL. Auth via JWT, OpenAPI docs, and Docker Compose for local runs.",
    budget: { kind: "hourly", rate: 50 },
    type: "hourly",
    posted: "1 day ago",
    bids: 15,
    skills: ["Node.js", "PostgreSQL", "Docker", "AWS"],
    duration: "4–6 weeks",
    deliverables: [
      "CRUD inventory API",
      "Auth + role middleware",
      "OpenAPI specification",
      "Docker Compose setup",
    ],
    client: {
      name: "ShelfWise",
      location: "Remote · US",
      projectsPosted: 12,
      hireRate: "68%",
    },
  },
  {
    id: "4",
    title: "Content Writing - Tech Blog",
    summary: "Write 10 articles about AI and machine learning",
    description:
      "Produce ten long-form articles (1,200–1,800 words) on practical AI/ML topics for an engineering audience. SEO briefs provided; you own research, drafts, and one revision round.",
    budget: { kind: "fixed", min: 800, max: 1500 },
    type: "fixed_price",
    posted: "2 days ago",
    bids: 5,
    skills: ["Content Writing", "Tech Writing", "SEO", "AI"],
    duration: "3 weeks",
    deliverables: [
      "10 published-ready articles",
      "Meta titles and descriptions",
      "Internal linking suggestions",
    ],
    client: {
      name: "Signal Desk",
      location: "Remote · Worldwide",
      projectsPosted: 21,
      hireRate: "82%",
    },
  },
  {
    id: "5",
    title: "DevOps Setup & CI/CD Pipeline",
    summary: "Set up CI/CD pipeline for web application",
    description:
      "Greenfield web app needs GitHub Actions CI, staging/prod deploys to AWS EKS, and Terraform for the base cluster. Document runbooks for the in-house team.",
    budget: { kind: "hourly", rate: 75 },
    type: "hourly",
    posted: "3 days ago",
    bids: 9,
    skills: ["Kubernetes", "GitHub Actions", "AWS", "Terraform"],
    duration: "2–3 weeks",
    deliverables: [
      "CI workflows (lint, test, build)",
      "Staging + prod deploy pipelines",
      "Terraform modules for EKS",
      "Operator runbook",
    ],
    client: {
      name: "Orbit Systems",
      location: "Remote · UK",
      projectsPosted: 5,
      hireRate: "90%",
    },
  },
  {
    id: "6",
    title: "Social Media Marketing Strategy",
    summary: "Develop 3-month social media marketing plan",
    description:
      "B2B SaaS company wants a three-month LinkedIn and X plan: content pillars, posting calendar, paid boost recommendations, and KPI dashboard.",
    budget: { kind: "fixed", min: 1200, max: 2500 },
    type: "fixed_price",
    posted: "4 days ago",
    bids: 7,
    skills: ["Social Media", "Marketing", "Content Strategy", "Analytics"],
    duration: "2 weeks",
    deliverables: [
      "Strategy document",
      "90-day content calendar",
      "Paid boost plan",
      "KPI tracking sheet",
    ],
    client: {
      name: "Clearpath CRM",
      location: "Remote · US",
      projectsPosted: 6,
      hireRate: "71%",
    },
  },
];

export function getMockProject(id: string): MockProject | undefined {
  return MOCK_PROJECTS.find((p) => p.id === id);
}

export function formatProjectBudget(budget: ProjectBudget): string {
  if (budget.kind === "hourly") return `$${budget.rate}/hr`;
  return `$${budget.min} - $${budget.max}`;
}

export function projectTypeLabel(type: MockProject["type"]): string {
  return type.replace(/_/g, " ");
}
