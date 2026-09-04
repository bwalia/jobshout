use jobshout_jobs::JobService;

#[derive(Clone)]
pub struct AppState {
    pub jobs: JobService,
}
