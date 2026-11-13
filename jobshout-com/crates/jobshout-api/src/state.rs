use std::sync::Arc;

use jobshout_applications::ApplicationService;
use jobshout_auth::AccountService;
use jobshout_billing::BillingService;
use jobshout_candidates::CandidateService;
use jobshout_content::InsightService;
use jobshout_events::EventService;
use jobshout_freelance::FreelanceService;
use jobshout_groups::GroupsService;
use jobshout_hire::HireService;
use jobshout_integrations::PlatformClient;
use jobshout_jobs::JobService;
use jobshout_messaging::MessagingService;
use jobshout_notifications::NotificationService;
use jobshout_showcase::ShowcaseService;
use jobshout_users::MemberService;
use sqlx::PgPool;

#[derive(Clone)]
pub struct AppState {
    pub jobs: JobService,
    pub candidates: CandidateService,
    pub applications: ApplicationService,
    pub insights: InsightService,
    pub showcase: ShowcaseService,
    pub members: MemberService,
    pub accounts: AccountService,
    pub events: EventService,
    pub notifications: NotificationService,
    pub messaging: MessagingService,
    pub freelance: FreelanceService,
    pub hire: HireService,
    pub groups: GroupsService,
    pub billing: BillingService,
    /// Shared pool for AI draft persistence (Phase 5a).
    pub ai_pool: Option<PgPool>,
    /// Go platform client for `POST /tasks/launch`.
    pub platform: Option<PlatformClient>,
    /// Public origin used in feed links and newsletter confirmation links.
    pub site_url: Arc<str>,
    /// Identity headers are only trusted alongside this token. `None` means
    /// the operator opted out (local dev only) via JOBSHOUT_ALLOW_UNSIGNED_IDENTITY.
    pub internal_token: Option<Arc<str>>,
    /// When true, newsletter confirmations are enqueued to the email outbox.
    pub email_outbox_enabled: bool,
    /// Phase 1 feature flags (dark on int by default).
    pub members_rich_enabled: bool,
    pub dashboard_enabled: bool,
    pub messaging_enabled: bool,
    /// Phase 2a: live projects marketplace.
    pub projects_live: bool,
    /// Phase 2b: free monthly credits + bid spend.
    pub credits_enabled: bool,
    /// Phase 2b: Stripe packs/subscriptions (stays off until volume).
    pub purchases_enabled: bool,
    /// Phase 5a: marketplace AI assistants (drafts only; human confirms).
    pub ai_features: bool,
    /// Phase 3: Linked Hire.
    pub hire_enabled: bool,
    /// Phase 4: Linked Groups.
    pub groups_enabled: bool,
}
