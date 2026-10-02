import os

/// Loggers by area. Tokens, passwords, identity tokens and push tokens are
/// never logged; interpolate user content with `privacy: .private`.
public enum Log {
    public static let auth = Logger(subsystem: "com.jobshout.app", category: "auth")
    public static let api = Logger(subsystem: "com.jobshout.app", category: "api")
    public static let live = Logger(subsystem: "com.jobshout.app", category: "live")
    public static let push = Logger(subsystem: "com.jobshout.app", category: "push")
}
