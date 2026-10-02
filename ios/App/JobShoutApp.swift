import JobShoutAuth
import JobShoutCore
import JobShoutFeatures
import SwiftUI
import UIKit
import UserNotifications

@main
struct JobShoutApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var app: AppModel

    init() {
        var environment = AppEnvironment.fromBundle()
        #if DEBUG
        // Point a debug build elsewhere without rebuilding, e.g.
        // SIMCTL_CHILD_JOBSHOUT_ENVIRONMENT=local xcrun simctl launch …
        if let raw = ProcessInfo.processInfo.environment["JOBSHOUT_ENVIRONMENT"],
           let override = AppEnvironment(rawValue: raw) {
            environment = override
        }
        #endif
        _app = State(initialValue: AppModel(
            environment: environment,
            store: KeychainTokenStore(account: environment.rawValue),
            device: .current(name: UIDevice.current.name)))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                .task {
                    delegate.app = app
                    await app.start()
                }
                .onChange(of: app.phase, initial: true) { _, phase in
                    if phase == .signedIn { Push.enable() }
                }
        }
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate {
    weak var app: AppModel?

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        #if DEBUG
        let sandbox = true
        #else
        let sandbox = false
        #endif
        Task { @MainActor in app?.didRegisterForPush(token: deviceToken, sandbox: sandbox) }
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        Log.push.notice("APNs registration failed: \(error.localizedDescription, privacy: .public)")
    }
}

enum Push {
    /// Provisional authorisation: notifications arrive quietly in Notification
    /// Centre without a permission prompt, and the user can promote them to
    /// alerts from there. Nobody is asked before they've seen what they get.
    @MainActor static func enable() {
        Task {
            _ = try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound, .provisional])
            UIApplication.shared.registerForRemoteNotifications()
        }
    }
}
