use jobshout_candidates::CandidateService;
use jobshout_jobs::JobService;

#[derive(Clone)]
pub struct AppState {
    pub jobs: JobService,
    pub candidates: CandidateService,
}
