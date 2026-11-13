//! External integrations for JobShout.com.
//!
//! Marketplace AI calls the Go platform via [`platform::PlatformClient`]
//! (`POST /api/v1/tasks/launch`) — never invent a second AI runtime.

#![forbid(unsafe_code)]

pub mod platform;

pub use platform::{LaunchResult, PlatformClient};
