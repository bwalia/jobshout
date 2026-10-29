import Foundation

/// Upwork-style freelance marketplace listing (jobshout.com `/projects`).
/// Mirrors `jobshout-com/web/nextjs/lib/projects.ts` until the Rust freelance
/// API is auth-wired and proxied through Go.
public struct FreelanceProject: Identifiable, Hashable, Sendable {
    public enum Kind: String, Hashable, Sendable {
        case fixedPrice = "fixed_price"
        case hourly
        case retainer
        case milestone

        public var label: String { rawValue.replacingOccurrences(of: "_", with: " ") }
    }

    public enum Budget: Hashable, Sendable {
        case fixed(min: Double, max: Double)
        case hourly(rate: Double)

        public var display: String {
            switch self {
            case .fixed(let min, let max): return String(format: "$%.0f – $%.0f", min, max)
            case .hourly(let rate): return String(format: "$%.0f/hr", rate)
            }
        }
    }

    public struct Client: Hashable, Sendable {
        public var name: String
        public var location: String
        public var projectsPosted: Int
        public var hireRate: String
    }

    public var id: String
    public var title: String
    public var summary: String
    public var description: String
    public var budget: Budget
    public var kind: Kind
    public var posted: String
    public var bidCount: Int
    public var skills: [String]
    public var duration: String
    public var deliverables: [String]
    public var client: Client

    public var isHourly: Bool {
        kind == .hourly || {
            if case .hourly = budget { return true }
            return false
        }()
    }
}

public enum FreelanceCatalog {
    public static let projects: [FreelanceProject] = [
        .init(
            id: "1",
            title: "React Developer for E-commerce Platform",
            summary: "Build a modern e-commerce frontend with React and TypeScript",
            description: "We're rebuilding the storefront for a mid-size online retailer. You will own the product listing, cart, and checkout flows in Next.js App Router, wire them to our existing GraphQL catalog API, and ship a responsive design that matches our Figma kit. Expect weekly demos and a clear acceptance checklist per milestone.",
            budget: .fixed(min: 2000, max: 5000),
            kind: .fixedPrice,
            posted: "2 hours ago",
            bidCount: 12,
            skills: ["React", "TypeScript", "Next.js", "Tailwind"],
            duration: "3–4 weeks",
            deliverables: [
                "Product listing + detail pages",
                "Cart and checkout UI",
                "Responsive layout matching Figma",
                "Basic Playwright smoke tests",
            ],
            client: .init(name: "Northline Retail", location: "Remote · UK", projectsPosted: 8, hireRate: "75%")
        ),
        .init(
            id: "2",
            title: "Mobile App UI/UX Design",
            summary: "Design a fitness tracking app for iOS and Android",
            description: "Early-stage fitness startup needs a complete UI/UX pass for a cross-platform tracking app. Deliver wireframes, high-fidelity screens, and a clickable prototype covering onboarding, workout logging, and weekly progress.",
            budget: .fixed(min: 1500, max: 3000),
            kind: .fixedPrice,
            posted: "5 hours ago",
            bidCount: 8,
            skills: ["UI/UX", "Figma", "Mobile Design", "Prototyping"],
            duration: "2–3 weeks",
            deliverables: [
                "User flows and wireframes",
                "Hi-fi Figma screens (light + dark)",
                "Interactive prototype",
                "Design handoff notes",
            ],
            client: .init(name: "Pulse Labs", location: "Remote · EU", projectsPosted: 3, hireRate: "100%")
        ),
        .init(
            id: "3",
            title: "Backend API Development",
            summary: "Build REST API for inventory management system",
            description: "Need a production-ready REST API for inventory, SKUs, and warehouse transfers. Prefer Node.js + PostgreSQL. Auth via JWT, OpenAPI docs, and Docker Compose for local runs.",
            budget: .hourly(rate: 50),
            kind: .hourly,
            posted: "1 day ago",
            bidCount: 15,
            skills: ["Node.js", "PostgreSQL", "Docker", "AWS"],
            duration: "4–6 weeks",
            deliverables: [
                "CRUD inventory API",
                "Auth + role middleware",
                "OpenAPI specification",
                "Docker Compose setup",
            ],
            client: .init(name: "ShelfWise", location: "Remote · US", projectsPosted: 12, hireRate: "68%")
        ),
        .init(
            id: "4",
            title: "Content Writing - Tech Blog",
            summary: "Write 10 articles about AI and machine learning",
            description: "Produce ten long-form articles (1,200–1,800 words) on practical AI/ML topics for an engineering audience. SEO briefs provided; you own research, drafts, and one revision round.",
            budget: .fixed(min: 800, max: 1500),
            kind: .fixedPrice,
            posted: "2 days ago",
            bidCount: 5,
            skills: ["Content Writing", "Tech Writing", "SEO", "AI"],
            duration: "3 weeks",
            deliverables: [
                "10 published-ready articles",
                "Meta titles and descriptions",
                "Internal linking suggestions",
            ],
            client: .init(name: "Signal Desk", location: "Remote · Worldwide", projectsPosted: 21, hireRate: "82%")
        ),
        .init(
            id: "5",
            title: "DevOps Setup & CI/CD Pipeline",
            summary: "Set up CI/CD pipeline for web application",
            description: "Greenfield web app needs GitHub Actions CI, staging/prod deploys to AWS EKS, and Terraform for the base cluster. Document runbooks for the in-house team.",
            budget: .hourly(rate: 75),
            kind: .hourly,
            posted: "3 days ago",
            bidCount: 9,
            skills: ["Kubernetes", "GitHub Actions", "AWS", "Terraform"],
            duration: "2–3 weeks",
            deliverables: [
                "CI workflows (lint, test, build)",
                "Staging + prod deploy pipelines",
                "Terraform modules for EKS",
                "Operator runbook",
            ],
            client: .init(name: "Orbit Systems", location: "Remote · UK", projectsPosted: 5, hireRate: "90%")
        ),
        .init(
            id: "6",
            title: "Social Media Marketing Strategy",
            summary: "Develop 3-month social media marketing plan",
            description: "B2B SaaS company wants a three-month LinkedIn and X plan: content pillars, posting calendar, paid boost recommendations, and KPI dashboard.",
            budget: .fixed(min: 1200, max: 2500),
            kind: .fixedPrice,
            posted: "4 days ago",
            bidCount: 7,
            skills: ["Social Media", "Marketing", "Content Strategy", "Analytics"],
            duration: "2 weeks",
            deliverables: [
                "Strategy document",
                "90-day content calendar",
                "Paid boost plan",
                "KPI tracking sheet",
            ],
            client: .init(name: "Clearpath CRM", location: "Remote · US", projectsPosted: 6, hireRate: "71%")
        ),
    ]

    public static func project(id: String) -> FreelanceProject? {
        projects.first { $0.id == id }
    }
}
