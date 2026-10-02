import SwiftUI

public struct RootView: View {
    @Environment(AppModel.self) private var app

    public init() {}

    public var body: some View {
        Group {
            switch app.phase {
            case .launching:
                ProgressView()
            case .signedOut:
                SignInView()
            case .signedIn:
                MainTabView()
            }
        }
        .animation(.default, value: app.phase)
    }
}

struct MainTabView: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        TabView {
            Tab("Home", systemImage: "house") {
                NavigationStack { HomeView().detailDestinations() }
            }
            Tab("Agents", systemImage: "sparkles") {
                NavigationStack { AgentsView().detailDestinations() }
            }
            Tab("Work", systemImage: "square.stack.3d.up") {
                NavigationStack { WorkView().detailDestinations() }
            }
            Tab("Approvals", systemImage: "checkmark.shield") {
                NavigationStack { ApprovalsView().detailDestinations() }
            }
            .badge(app.pendingApprovals)
            Tab("Me", systemImage: "person.crop.circle") {
                NavigationStack { MeView().detailDestinations() }
            }
        }
    }
}
