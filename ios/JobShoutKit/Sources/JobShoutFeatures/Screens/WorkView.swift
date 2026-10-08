import JobShoutAPI
import SwiftUI

struct WorkView: View {
    @Environment(AppModel.self) private var app
    @State private var filter: Filter = .active
    @State private var tasks = Resource<[WorkTask]>()

    enum Filter: String, CaseIterable, Identifiable {
        case active = "Running", review = "Review", done = "Done", all = "All"
        var id: String { rawValue }
        var status: String? {
            switch self {
            case .active: "in_progress"
            case .review: "review"
            case .done: "done"
            case .all: nil
            }
        }
    }

    var body: some View {
        ResourceView(resource: tasks, retry: load) { list in
            List {
                Section {
                    Picker("Filter", selection: $filter) {
                        ForEach(Filter.allCases) { Text($0.rawValue).tag($0) }
                    }
                    .segmentedPickerStyle()
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }
                Section {
                    ForEach(list, id: \.id) { task in
                        NavigationLink(value: TaskRoute(id: task.id)) { TaskRow(task: task) }
                            .accessibilityIdentifier("task-row")
                    }
                } footer: {
                    SyncFooter(loadedAt: tasks.loadedAt)
                }
                if list.isEmpty {
                    ContentUnavailableView(emptyTitle, systemImage: "tray", description: Text("Call an agent from the Agents tab."))
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Work")
        .task(id: TaskKey(filter: filter, revision: app.revision(.work))) { await load() }
    }

    private struct TaskKey: Hashable { let filter: Filter; let revision: Int }

    private var emptyTitle: String {
        switch filter {
        case .active: "Nothing running"
        case .review: "Nothing to review"
        case .done: "Nothing finished yet"
        case .all: "No work yet"
        }
    }

    private func load() async {
        let status = filter.status
        await tasks.load {
            try await app.api.tasks(status: status).sorted { $0.updatedAt > $1.updatedAt }
        }
    }
}

struct TaskRow: View {
    let task: WorkTask

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(task.title).font(.headline).lineLimit(2)
            HStack(spacing: 8) {
                StatusPill(status: task.lastRunStatus ?? task.status)
                Spacer()
                Text(task.updatedAt, format: .relative(presentation: .named))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}

struct TaskDetailView: View {
    @Environment(AppModel.self) private var app
    let taskID: String
    @State private var task = Resource<WorkTask>()
    @State private var runs = Resource<[TaskRun]>()

    var body: some View {
        ResourceView(resource: task, retry: load) { task in
            List {
                Section {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(task.title).font(.title3.bold())
                        StatusPill(status: task.status)
                    }
                    if let description = task.description, !description.isEmpty {
                        Text(description).foregroundStyle(.secondary)
                    }
                } footer: {
                    SyncFooter(loadedAt: self.task.loadedAt)
                }
                Section("Runs") {
                    if let list = runs.value, !list.isEmpty {
                        ForEach(list, id: \.id) { RunCard(run: $0) }
                    } else if runs.value != nil {
                        Text("No runs yet.")
                            .foregroundStyle(.secondary)
                    } else {
                        ProgressView()
                    }
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Task")
        .inlineTitle()
        .task(id: app.revision(.work)) { await load() }
        .task { await poll() }
    }

    private var inFlight: Bool {
        (task.value?.status.isInFlight ?? false) || (runs.value ?? []).contains { $0.status.isInFlight }
    }

    private func load() async {
        async let t: Void = task.load { try await app.api.task(taskID) }
        async let r: Void = runs.load { try await app.api.runs(taskID: taskID) }
        _ = await (t, r)
    }

    /// Live events trigger refetches, but they are hints; while the work is
    /// moving, poll too so progress never depends on the socket.
    private func poll() async {
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(inFlight ? 4 : 30))
            if Task.isCancelled { return }
            await load()
        }
    }
}

struct RunCard: View {
    let run: TaskRun
    @State private var expanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                StatusPill(status: run.status)
                Spacer()
                if let ms = run.latencyMs, ms > 0 {
                    Text(Duration.milliseconds(ms), format: .units(allowed: [.minutes, .seconds], width: .abbreviated))
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            if let error = run.errorMessage, !error.isEmpty {
                Text(error).font(.callout).foregroundStyle(.red)
            }
            if let output = run.output, !output.isEmpty {
                Text(output)
                    .font(.callout)
                    .lineLimit(expanded ? nil : 6)
                    .selectableText()
                Button(expanded ? "Show less" : "Show all") { expanded.toggle() }
                    .font(.caption.bold())
            }
            if let tokens = run.totalTokens, tokens > 0 {
                HStack(spacing: 12) {
                    Label("\(tokens.formatted()) tokens", systemImage: "number")
                    if let cost = run.costUsd, cost > 0 {
                        Label(cost.formatted(.currency(code: "USD").precision(.fractionLength(2...4))), systemImage: "dollarsign.circle")
                    }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }
}
