#if !os(watchOS)
import AuthenticationServices
import Foundation

#if canImport(AppKit)
import AppKit
#endif
#if canImport(UIKit)
import UIKit
#endif

/// Opens Google OAuth in a system browser sheet and returns the one-time ticket
/// from `jobshout://auth/google?ticket=…` (server must be called with `native=1`).
public enum GoogleSignIn {
    public static let callbackScheme = "jobshout"

    public enum Outcome: Sendable {
        case ticket(String)
        case cancelled
        case failed(String)
    }

    /// ASWebAuthenticationSession delivers its completion off the main actor.
    /// Keep that callback nonisolated and only hop to MainActor for presentation.
    public static func authorize(startURL: URL) async -> Outcome {
        await withCheckedContinuation { continuation in
            let holder = SessionHolder()
            holder.finish = { outcome in
                holder.finish = nil
                holder.session = nil
                continuation.resume(returning: outcome)
            }

            let session = ASWebAuthenticationSession(
                url: startURL,
                callbackURLScheme: callbackScheme
            ) { callbackURL, error in
                holder.finish?(outcome(from: callbackURL, error: error))
            }
            session.prefersEphemeralWebBrowserSession = false
            holder.session = session

            Task { @MainActor in
                session.presentationContextProvider = Presenter.shared
                if !session.start() {
                    holder.finish?(.failed("Couldn't open Google sign-in."))
                }
            }
        }
    }

    private static func outcome(from callbackURL: URL?, error: Error?) -> Outcome {
        if let error {
            if let auth = error as? ASWebAuthenticationSessionError,
               auth.code == .canceledLogin {
                return .cancelled
            }
            return .failed(error.localizedDescription)
        }
        guard let callbackURL,
              let comps = URLComponents(url: callbackURL, resolvingAgainstBaseURL: false) else {
            return .failed("Google didn't return a sign-in ticket.")
        }
        if let err = comps.queryItems?.first(where: { $0.name == "error" })?.value, !err.isEmpty {
            return .failed(googleErrorMessage(err))
        }
        guard let ticket = comps.queryItems?.first(where: { $0.name == "ticket" })?.value,
              !ticket.isEmpty else {
            return .failed("Google didn't return a sign-in ticket.")
        }
        return .ticket(ticket)
    }

    private static func googleErrorMessage(_ code: String) -> String {
        switch code {
        case "denied": "Google sign-in was cancelled."
        case "missing_code": "Google didn't finish authorizing."
        default: "Google sign-in failed (\(code))."
        }
    }
}

/// Keeps the auth session alive until the callback fires, and makes resume once-only.
private final class SessionHolder: @unchecked Sendable {
    var session: ASWebAuthenticationSession?
    var finish: ((GoogleSignIn.Outcome) -> Void)?
}

@MainActor
private final class Presenter: NSObject, ASWebAuthenticationPresentationContextProviding {
    static let shared = Presenter()

    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        #if os(macOS)
        NSApplication.shared.keyWindow
            ?? NSApplication.shared.windows.first
            ?? ASPresentationAnchor()
        #else
        UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first { $0.isKeyWindow }
            ?? ASPresentationAnchor()
        #endif
    }
}
#endif
