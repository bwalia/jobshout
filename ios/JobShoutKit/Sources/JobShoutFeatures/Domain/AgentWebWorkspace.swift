import Foundation

/// Builtins that register a specialist Workspace tab on web (`AGENT_CLIENTS` in
/// `web/nextjs/lib/agents/tab-clients.tsx`). iOS never clones those UIs — it
/// deep-links to `/panel/agents/{id}?tab=workspace` instead. Keep this set in
/// sync with the web registry; do not add per-builtin Swift product screens.
public enum AgentWebWorkspace {
    public static let builtins: Set<String> = [
        "career_ops",
        "mail",
        "pentester",
        "pr_reviewer",
        "credit_controller",
        "simpro_payments",
        "waf_lab",
        "seo_analyst",
        "secrets_rotation",
        "linux_patch",
        "ab_testing",
        "article_writer",
        "images",
        "course_generator",
        "jobshout_com_writer",
        "linkedin_poster",
        "video_producer",
        "marketing_email_writer",
        "meta_lead_analyst",
    ]

    public static func hasWebClient(_ builtin: String?) -> Bool {
        guard let builtin, !builtin.isEmpty else { return false }
        return builtins.contains(builtin)
    }
}
