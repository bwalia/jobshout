import JobShoutAuth
import JobShoutCore
import JobShoutFeatures
import SwiftUI

@main
struct JobShoutMacApp: App {
    @State private var app: AppModel

    init() {
        var environment = AppEnvironment.fromBundle()
        #if DEBUG
        if let raw = ProcessInfo.processInfo.environment["JOBSHOUT_ENVIRONMENT"],
           let override = AppEnvironment(rawValue: raw) {
            environment = override
        }
        #endif
        let host = Host.current().localizedName ?? "Mac"
        _app = State(initialValue: AppModel(
            environment: environment,
            store: KeychainTokenStore(account: environment.rawValue),
            device: .current(name: host)))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                .frame(minWidth: 900, minHeight: 600)
                .task { await app.start() }
        }
        .commands {
            CommandGroup(replacing: .newItem) {}
        }
    }
}
