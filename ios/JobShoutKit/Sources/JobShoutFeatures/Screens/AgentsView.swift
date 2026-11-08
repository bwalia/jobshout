import JobShoutAPI
import JobShoutCore
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
    @Environment(\.openURL) private var openURL
    let agentID: String
    @State private var agent = Resource<Agent>()
    @State private var knowledge: [KnowledgeFile] = []
    @State private var skills: [Skill] = []
    @State private var extrasError: String?
    @State private var running = false
    @State private var creatingTask = false
    @State private var askInChat: ChatRoute?
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
                        running = true
                    } label: {
                        Label("Run", systemImage: "play.fill")
                            .font(.headline)
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                    .accessibilityIdentifier("run-agent")

                    Button {
                        creatingTask = true
                    } label: {
                        Label("New task", systemImage: "plus.square.on.square")
                    }

                    Button {
                        Task { await startChat(for: agent) }
                    } label: {
                        Label("Ask in chat", systemImage: "bubble.left.and.bubble.right")
                    }

                    if AgentWebWorkspace.hasWebClient(agent.builtin) {
                        Button {
                            openURL(app.environment.agentWorkspaceURL(agentID: agent.id))
                        } label: {
                            Label("Open workspace on web", systemImage: "safari")
                        }
                        .accessibilityIdentifier("open-workspace-web")
                    }
                } footer: {
                    Text(footerCopy(for: agent))
                }

                Section("Model") {
                    LabeledContent("Engine", value: agent.engineType)
                    LabeledContent("Model", value: agent.modelName ?? "Default")
                    if let kind = agent.builtin {
                        LabeledContent("Specialist", value: kind)
                    }
                    if let score = agent.performanceScore, score != 0 {
                        LabeledContent("Score", value: String(format: "%.0f", score))
                    }
                }

                if !knowledge.isEmpty || !skills.isEmpty || extrasError != nil {
                    Section("Knowledge & skills") {
                        if let extrasError {
                            Text(extrasError).foregroundStyle(.secondary)
                        }
                        ForEach(knowledge, id: \.id) { file in
                            Label(file.filename, systemImage: "doc.text")
                        }
                        ForEach(skills, id: \.id) { skill in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(skill.name)
                                Text(skill.slug).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        if knowledge.isEmpty && skills.isEmpty && extrasError == nil {
                            Text("None enabled yet.").foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .sheet(isPresented: $running) {
                CallAgentView(agent: agent) { taskID in
                    running = false
                    launchedTask = TaskRoute(id: taskID)
                }
            }
            .sheet(isPresented: $creatingTask) {
                NewTaskView(agent: agent) { taskID in
                    creatingTask = false
                    launchedTask = TaskRoute(id: taskID)
                }
            }
        }
        .navigationTitle("Agent")
        .inlineTitle()
        .navigationDestination(item: $launchedTask) { TaskDetailView(taskID: $0.id) }
        .navigationDestination(item: $askInChat) { ChatSessionView(sessionID: $0.id, title: $0.title) }
        .task { await load() }
    }

    private func footerCopy(for agent: Agent) -> String {
        if AgentWebWorkspace.hasWebClient(agent.builtin) {
            return "Run starts work on JobShout. Specialist results live in the web Workspace; open it from here when you need the full console."
        }
        return "The agent runs on JobShout, not on this phone. Anything that needs approval comes back to you first."
    }

    private func load() async {
        await agent.load { try await app.api.agent(agentID) }
        extrasError = nil
        do {
            async let k = app.api.knowledge(agentID: agentID)
            async let s = app.api.skills(agentID: agentID)
            let (files, skillList) = try await (k, s)
            knowledge = files
            skills = skillList
        } catch {
            extrasError = APIError.from(error).message
            knowledge = []
            skills = []
        }
    }

    private func startChat(for agent: Agent) async {
        do {
            let session = try await app.api.startChatSession(agentID: agent.id)
            askInChat = ChatRoute(id: session.id, title: agent.name)
        } catch {
            extrasError = APIError.from(error).message
        }
    }
}
