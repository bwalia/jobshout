import JobShoutAPI
import SwiftUI

struct AgentsView: View {
    @Environment(AppModel.self) private var app
    @State private var agents = Resource<[Agent]>()
    @State private var search = ""

    var body: some View {
        ResourceView(resource: agents, retry: load) { list in
            let shown = filtered(list)
            List {
                ForEach(shown, id: \.id) { agent in
                    NavigationLink(value: AgentRoute(id: agent.id)) { AgentRow(agent: agent) }
                        .accessibilityIdentifier("agent-row")
                }
                if shown.isEmpty {
                    ContentUnavailableView.search(text: search)
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Agents")
        .searchable(text: $search, prompt: "Name, role or skill")
        .task(id: app.revision(.agents)) { await load() }
    }

    private func load() async {
        await agents.load { try await app.api.agents() }
    }

    private func filtered(_ list: [Agent]) -> [Agent] {
        let q = search.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty else { return list }
        return list.filter {
            $0.name.localizedCaseInsensitiveContains(q)
                || $0.role.localizedCaseInsensitiveContains(q)
                || ($0.description ?? "").localizedCaseInsensitiveContains(q)
        }
    }
}

struct AgentRow: View {
    let agent: Agent

    var body: some View {
        HStack(spacing: 12) {
            AgentAvatar(name: agent.name)
            VStack(alignment: .leading, spacing: 2) {
                Text(agent.name).font(.headline)
                Text(agent.role).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 8)
            StatusPill(status: agent.status)
        }
        .padding(.vertical, 2)
    }
}

struct AgentAvatar: View {
    let name: String
    var size: CGFloat = 40

    var body: some View {
        Text(String(name.prefix(1)).uppercased())
            .font(.system(size: size * 0.45, weight: .semibold, design: .rounded))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(Color.accentColor.gradient, in: .rect(cornerRadius: size * 0.28))
            .accessibilityHidden(true)
    }
}

struct AgentDetailView: View {
    @Environment(AppModel.self) private var app
    let agentID: String
    @State private var agent = Resource<Agent>()
    @State private var calling = false
    @State private var launchedTask: TaskRoute?

    var body: some View {
        ResourceView(resource: agent, retry: load) { agent in
            List {
                Section {
                    HStack(spacing: 14) {
                        AgentAvatar(name: agent.name, size: 56)
                        VStack(alignment: .leading, spacing: 4) {
                            Text(agent.name).font(.title2.bold())
                            Text(agent.role).foregroundStyle(.secondary)
                            StatusPill(status: agent.status)
                        }
                    }
                    .padding(.vertical, 4)
                    if let description = agent.description, !description.isEmpty {
                        Text(description)
                    }
                }
                Section {
                    Button {
                        calling = true
                    } label: {
                        Label("Call agent", systemImage: "bolt.fill")
                            .font(.headline)
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                } footer: {
                    Text("The agent runs on JobShout, not on this phone. Anything that needs approval comes back to you first.")
                }
                Section("Details") {
                    LabeledContent("Engine", value: agent.engineType)
                    if let model = agent.modelName { LabeledContent("Model", value: model) }
                    if let kind = agent.builtin { LabeledContent("Specialist", value: kind) }
                }
            }
            .sheet(isPresented: $calling) {
                CallAgentView(agent: agent) { taskID in
                    calling = false
                    launchedTask = TaskRoute(id: taskID)
                }
            }
        }
        .navigationTitle("Agent")
        .inlineTitle()
        .navigationDestination(item: $launchedTask) { TaskDetailView(taskID: $0.id) }
        .task { await load() }
    }

    private func load() async {
        await agent.load { try await app.api.agent(agentID) }
    }
}
