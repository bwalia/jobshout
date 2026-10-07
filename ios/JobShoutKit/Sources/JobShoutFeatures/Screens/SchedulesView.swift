import JobShoutAPI
import SwiftUI

struct SchedulesView: View {
    @Environment(AppModel.self) private var app
    @State private var schedules = Resource<[Schedule]>()
    @State private var actionError: String?

    var body: some View {
        ResourceView(resource: schedules, retry: load) { list in
            List {
                if let actionError {
                    Section {
                        Label(actionError, systemImage: "exclamationmark.triangle").foregroundStyle(.red)
                    }
                }
                Section {
                    ForEach(list, id: \.id) { schedule in
                        ScheduleRow(schedule: schedule) { status in
                            await setStatus(schedule.id, status: status)
                        }
                    }
                } footer: {
                    SyncFooter(loadedAt: schedules.loadedAt)
                }
                if list.isEmpty {
                    ContentUnavailableView(
                        "No schedules",
                        systemImage: "clock",
                        description: Text("Create schedules on the web; pause and resume them here."))
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Schedules")
        .task { await load() }
    }

    private func load() async {
        await schedules.load {
            try await app.api.schedules().sorted {
                ($0.nextRunAt ?? .distantPast) > ($1.nextRunAt ?? .distantPast)
            }
        }
    }

    private func setStatus(_ id: String, status: String) async {
        actionError = nil
        do {
            _ = try await app.api.setScheduleStatus(id, status: status)
            await load()
        } catch {
            actionError = APIError.from(error).message
        }
    }
}

private struct ScheduleRow: View {
    let schedule: Schedule
    let onStatus: (String) async -> Void
    @State private var busy = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(schedule.name).font(.headline)
                    Text(subtitle).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                StatusPill(status: schedule.status)
            }
            HStack {
                if schedule.status == "active" {
                    Button("Pause") { Task { await act("paused") } }
                        .disabled(busy)
                } else if schedule.status == "paused" {
                    Button("Resume") { Task { await act("active") } }
                        .disabled(busy)
                }
                Spacer()
                if let next = schedule.nextRunAt {
                    Text("Next \(next.formatted(.relative(presentation: .named)))")
                        .font(.caption2).foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private var subtitle: String {
        var parts = [schedule.scheduleType]
        if let cron = schedule.cronExpression, !cron.isEmpty { parts.append(cron) }
        if let preset = schedule.schedulePreset, !preset.isEmpty { parts.append(preset) }
        return parts.joined(separator: " · ")
    }

    private func act(_ status: String) async {
        busy = true
        defer { busy = false }
        await onStatus(status)
    }
}
