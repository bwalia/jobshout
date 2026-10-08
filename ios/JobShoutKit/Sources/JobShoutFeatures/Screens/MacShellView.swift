import SwiftUI

#if os(macOS)

/// Mac signed-in chrome: sidebar groups match `web/nextjs/lib/nav.ts`.
struct MacShellView: View {
    @Environment(AppModel.self) private var app
    @State private var selection: MacDestination? = .home

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                Section {
                    Label("Home", systemImage: "house")
                        .tag(MacDestination.home)
                        .accessibilityIdentifier("sidebar-home")
                }
                Section("Work") {
                    Label("Tasks", systemImage: "square.stack.3d.up")
                        .tag(MacDestination.tasks)
                        .accessibilityIdentifier("sidebar-tasks")
                    Label("Board", systemImage: "rectangle.split.3x1")
                        .tag(MacDestination.board)
                        .accessibilityIdentifier("sidebar-board")
                }
                Section("Agents") {
                    Label("All agents", systemImage: "sparkles")
                        .tag(MacDestination.agents)
                        .accessibilityIdentifier("sidebar-agents")
                }
                Section("Automation") {
                    Label("Schedules", systemImage: "clock")
                        .tag(MacDestination.schedules)
                        .accessibilityIdentifier("sidebar-schedules")
                }
                Section {
                    Label("Approvals", systemImage: "checkmark.shield")
                        .badge(app.pendingApprovals)
                        .tag(MacDestination.approvals)
                        .accessibilityIdentifier("sidebar-approvals")
                    Label("Settings", systemImage: "gearshape")
                        .tag(MacDestination.settings)
                        .accessibilityIdentifier("sidebar-settings")
                }
            }
            .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 280)
            .listStyle(.sidebar)
            .accessibilityIdentifier("mac-sidebar")
        } detail: {
            NavigationStack {
                Group {
                    switch selection ?? .home {
                    case .home: HomeView()
                    case .tasks: WorkView()
                    case .board: TaskBoardView()
                    case .agents: AgentsView()
                    case .schedules: SchedulesView()
                    case .approvals: ApprovalsView()
                    case .settings: MeView()
                    }
                }
                .detailDestinations()
            }
        }
        .navigationSplitViewStyle(.balanced)
    }
}

enum MacDestination: Hashable {
    case home, tasks, board, agents, schedules, approvals, settings
}

#endif
