import Foundation

/// A JobShout ring. The app talks to exactly one: every request, the
/// WebSocket and Sign in with Apple go to the same origin.
public enum AppEnvironment: String, CaseIterable, Sendable, Codable {
    case int, test, acc, prod
    /// A server on the Mac running the simulator (`start-local.sh`, port
    /// 8190 unless JOBSHOUT_LOCAL_PORT says otherwise). Debug builds only.
    case local

    public var host: String {
        switch self {
        case .local: "localhost:\(ProcessInfo.processInfo.environment["JOBSHOUT_LOCAL_PORT"] ?? "8190")"
        case .int: "int.jobshout.co.uk"
        case .test: "test.jobshout.co.uk"
        case .acc: "acc.jobshout.co.uk"
        case .prod: "jobshout.co.uk"
        }
    }

    public var apiBaseURL: URL { URL(string: "\(self == .local ? "http" : "https")://\(host)/api/v1")! }
    public var liveEventsURL: URL { URL(string: "\(self == .local ? "ws" : "wss")://\(host)/api/v1/ws")! }

    public var displayName: String {
        switch self {
        case .int: "Integration"
        case .test: "Test"
        case .acc: "Acceptance"
        case .prod: "Production"
        case .local: "This Mac"
        }
    }

    /// Info.plist key the build configuration sets (Debug → int,
    /// Staging → acc, Release → prod).
    public static let infoPlistKey = "JobShoutEnvironment"

    public static func fromBundle(_ bundle: Bundle = .main) -> AppEnvironment {
        let raw = bundle.object(forInfoDictionaryKey: infoPlistKey) as? String
        return raw.flatMap(AppEnvironment.init(rawValue:)) ?? .prod
    }
}
