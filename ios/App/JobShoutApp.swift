import JobShoutAuth
import JobShoutCore
import JobShoutFeatures
import SwiftUI
import UIKit
import UserNotifications

@main
struct JobShoutApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @Environment(\.scenePhase) private var scenePhase
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
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active { app.noteBecameActive() }
                }
        }
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    weak var app: AppModel?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

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

    // MARK: UNUserNotificationCenterDelegate
    // Delegate methods are nonisolated (Swift 6); hop to MainActor for AppModel.

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let type = response.notification.request.content.userInfo["type"] as? String
        Task { @MainActor in self.app?.openApprovalsFromNotification(type: type) }
        completionHandler()
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        // Foreground: still show banner; Work/Approvals also refetch via WS.
        completionHandler([.banner, .badge, .sound])
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
