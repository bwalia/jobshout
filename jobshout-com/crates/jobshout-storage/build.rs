// `sqlx::migrate!` embeds ../../migrations at compile time. Without this,
// adding a migration file does not rebuild the crate, and a local binary
// silently runs without the new migration.
fn main() {
    println!("cargo:rerun-if-changed=../../migrations");
}
