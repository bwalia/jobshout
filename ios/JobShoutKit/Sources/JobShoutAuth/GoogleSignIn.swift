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

    @MainActor
    public static func authorize(startURL: URL) async -> Outcome {
        await withCheckedContinuation { continuation in
            let session = ASWebAuthenticationSession(
                url: startURL,
                callbackURLScheme: callbackScheme
            ) { callbackURL, error in
                if let error {
                    if let auth = error as? ASWebAuthenticationSessionError,
                       auth.code == .canceledLogin {
                        continuation.resume(returning: .cancelled)
                    } else {
                        continuation.resume(returning: .failed(error.localizedDescription))
                    }
                    return
                }
                guard let callbackURL,
                      let comps = URLComponents(url: callbackURL, resolvingAgainstBaseURL: false) else {
                    continuation.resume(returning: .failed("Google didn't return a sign-in ticket."))
                    return
                }
                if let err = comps.queryItems?.first(where: { $0.name == "error" })?.value, !err.isEmpty {
                    continuation.resume(returning: .failed(googleErrorMessage(err)))
                    return
                }
                guard let ticket = comps.queryItems?.first(where: { $0.name == "ticket" })?.value,
                      !ticket.isEmpty else {
                    continuation.resume(returning: .failed("Google didn't return a sign-in ticket."))
                    return
                }
                continuation.resume(returning: .ticket(ticket))
            }
            session.prefersEphemeralWebBrowserSession = false
            let presenter = Presenter.shared
            session.presentationContextProvider = presenter
            if !session.start() {
                continuation.resume(returning: .failed("Couldn't open Google sign-in."))
            }
        }
    }

    private static func googleErrorMessage(_ code: String) -> String {
        switch code {
        case "denied": "Google sign-in was cancelled."
        case "missing_code": "Google didn't finish authorizing."
        default: "Google sign-in failed (\(code))."
        }
    }
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
