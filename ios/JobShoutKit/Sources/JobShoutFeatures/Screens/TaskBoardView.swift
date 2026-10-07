import JobShoutAPI
import SwiftUI

#if os(watchOS)

/// Board kanban is Mac/iPhone only; watch uses the Runs list instead.
struct TaskBoardView: View {
    var body: some View {
        ContentUnavailableView("Board on Mac or iPhone", systemImage: "rectangle.split.3x1")
    }
}

#else

/// Kanban board: columns match `TaskStatus`. Drag a card onto another column
/// to call `PATCH /tasks/{id}/transition`.
struct TaskBoardView: View {
    @Environment(AppModel.self) private var app
    @State private var tasks = Resource<[WorkTask]>()
    @State private var dragging: WorkTask?
    @State private var error: String?

    private let columns: [(id: String, title: String)] = [
        ("backlog", "Backlog"),
        ("todo", "To do"),
        ("in_progress", "In progress"),
        ("review", "Review"),
        ("done", "Done"),
    ]

    var body: some View {
        ResourceView(resource: tasks, retry: load) { list in
            board(list)
        }
        .navigationTitle("Board")
        .toolbar {
            ToolbarItem {
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
            }
        }
        .task(id: app.revision(.work)) { await load() }
    }

    @ViewBuilder
    private func board(_ list: [WorkTask]) -> some View {
        ScrollView(.horizontal) {
            HStack(alignment: .top, spacing: 12) {
                ForEach(columns, id: \.id) { col in
                    BoardColumn(
                        title: col.title,
                        tasks: list.filter { $0.status == col.id },
                        dragging: $dragging,
                        onDrop: { task in await move(task, to: col.id) }
                    )
                }
            }
            .padding()
        }
        .overlay(alignment: .bottom) {
            if let error {
                Text(error).font(.caption).foregroundStyle(.red).padding()
            }
        }
    }

    private func load() async {
        await tasks.load {
            try await app.api.tasks(perPage: 100).sorted { $0.updatedAt > $1.updatedAt }
        }
    }

    private func move(_ task: WorkTask, to status: String) async {
        guard task.status != status else { return }
        error = nil
        do {
            _ = try await app.api.transitionTask(task.id, status: status)
            await load()
        } catch {
            self.error = APIError.from(error).message
        }
    }
}

private struct BoardColumn: View {
    let title: String
    let tasks: [WorkTask]
    @Binding var dragging: WorkTask?
    let onDrop: (WorkTask) async -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            header
            cardList
        }
        .frame(width: 260)
        .padding(10)
        .background(.quaternary.opacity(0.4), in: .rect(cornerRadius: 12))
        .dropDestination(for: String.self) { _, _ in
            guard let dragging else { return false }
            Task { await onDrop(dragging) }
            return true
        }
    }

    private var header: some View {
        HStack {
            Text(title).font(.headline)
            Spacer()
            Text("\(tasks.count)").font(.caption).foregroundStyle(.secondary)
        }
    }

    private var cardList: some View {
        ScrollView {
            LazyVStack(spacing: 8) {
                ForEach(tasks, id: \.id) { task in
                    NavigationLink(value: TaskRoute(id: task.id)) {
                        TaskRow(task: task)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(.background.secondary, in: .rect(cornerRadius: 10))
                    }
                    .buttonStyle(.plain)
                    .draggable(task.id) {
                        dragging = task
                        return Text(task.title).padding(8)
                    }
                }
            }
        }
    }
}

#endif
