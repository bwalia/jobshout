import JobShoutAuth
import JobShoutCore
import JobShoutFeatures
import SwiftUI
import WatchKit

@main
struct JobShoutWatchApp: App {
    @State private var app: AppModel

    init() {
        let environment = AppEnvironment.fromBundle()
        let name = WKInterfaceDevice.current().name
        _app = State(initialValue: AppModel(
            environment: environment,
            store: KeychainTokenStore(account: environment.rawValue),
            device: .current(name: name)))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                .task { await app.start() }
        }
    }
}
