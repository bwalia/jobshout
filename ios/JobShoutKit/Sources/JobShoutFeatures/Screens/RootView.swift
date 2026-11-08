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
                #if os(macOS)
                MacShellView()
                #else
                MainTabView()
                #endif
            }
        }
        .animation(.default, value: app.phase)
    }
}

enum MainTab: Hashable {
    case home, agents, chat, freelance, work, schedules, approvals, me
}

struct MainTabView: View {
    @Environment(AppModel.self) private var app
    @State private var selection: MainTab = .home

    var body: some View {
        TabView(selection: $selection) {
            Tab("Home", systemImage: "house", value: MainTab.home) {
                NavigationStack { HomeView().detailDestinations() }
            }
            Tab("Agents", systemImage: "sparkles", value: MainTab.agents) {
                NavigationStack { AgentsView().detailDestinations() }
            }
            Tab("Chat", systemImage: "bubble.left.and.bubble.right", value: MainTab.chat) {
                NavigationStack { ChatView().detailDestinations() }
            }
            #if os(iOS)
            // Not "Projects" — that collides with workspace/board projects on web.
            Tab("Freelance", systemImage: "briefcase", value: MainTab.freelance) {
                NavigationStack { FreelanceProjectsView().detailDestinations() }
            }
            #endif
            Tab("Work", systemImage: "square.stack.3d.up", value: MainTab.work) {
                NavigationStack { WorkView().detailDestinations() }
            }
            #if os(iOS)
            Tab("Schedules", systemImage: "clock", value: MainTab.schedules) {
                NavigationStack { SchedulesView().detailDestinations() }
            }
            #endif
            Tab("Approvals", systemImage: "checkmark.shield", value: MainTab.approvals) {
                NavigationStack { ApprovalsView().detailDestinations() }
            }
            #if !os(watchOS)
            .badge(app.pendingApprovals)
            #endif
            Tab("Me", systemImage: "person.crop.circle", value: MainTab.me) {
                NavigationStack { MeView().detailDestinations() }
            }
        }
        .onChange(of: app.approvalsFocus) { _, _ in
            selection = .approvals
        }
    }
}
