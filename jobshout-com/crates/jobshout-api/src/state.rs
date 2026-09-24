use std::sync::Arc;

use jobshout_applications::ApplicationService;
use jobshout_candidates::CandidateService;
use jobshout_content::InsightService;
use jobshout_jobs::JobService;

#[derive(Clone)]
pub struct AppState {
    pub jobs: JobService,
    pub candidates: CandidateService,
    pub applications: ApplicationService,
    pub insights: InsightService,
    /// Public origin used in feed links and newsletter confirmation links.
    pub site_url: Arc<str>,
    /// Identity headers are only trusted alongside this token. `None` means
    /// the operator opted out (local dev only) via JOBSHOUT_ALLOW_UNSIGNED_IDENTITY.
    pub internal_token: Option<Arc<str>>,
}
