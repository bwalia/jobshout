import Foundation
import JobShoutAPI
import OpenAPIRuntime

public typealias Agent = Components.Schemas.Agent
public typealias AgentSchema = Components.Schemas.AgentSchema
public typealias AgentSchemaField = Components.Schemas.AgentSchemaField
public typealias WorkTask = Components.Schemas.Task
public typealias TaskRun = Components.Schemas.TaskRun
public typealias Approval = Components.Schemas.Approval
public typealias Project = Components.Schemas.Project
public typealias Device = Components.Schemas.Device
public typealias User = Components.Schemas.User
public typealias LaunchResult = Components.Schemas.LaunchResult
public typealias Schedule = Components.Schemas.Schedule

/// The calls the screens make, each mapped to a value or an `APIError`.
/// Screens never see generated output enums.
public struct PlatformAPI: Sendable {
    let client: Client

    public init(client: Client) {
        self.client = client
    }

    public func me() async throws -> User {
        try await map {
            switch try await client.getMe() {
            case .ok(let r): return try r.body.json
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func agents(search: String? = nil) async throws -> [Agent] {
        try await map {
            switch try await client.listAgents(query: .init(page: 1, perPage: 100, search: search?.nilIfBlank)) {
            case .ok(let r): return try r.body.json.data
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func agent(_ id: String) async throws -> Agent {
        try await map {
            switch try await client.getAgent(path: .init(agentID: id)) {
            case .ok(let r): return try r.body.json
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    /// The launch form for an agent: its builtin's schema, else the generic
    /// one for custom agents.
    public func launchSchema(for agent: Agent) async throws -> AgentSchema {
        try await map {
            if let builtin = agent.builtin {
                if case .ok(let r) = try await client.listAgentSchemas(),
                   let match = try r.body.json.first(where: { $0.builtin == builtin }) {
                    return match
                }
            }
            switch try await client.getGenericAgentSchema() {
            case .ok(let r): return try r.body.json
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func projects() async throws -> [Project] {
        try await map {
            switch try await client.listProjects(query: .init(page: 1, perPage: 100)) {
            case .ok(let r): return try r.body.json.data
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func createProject(name: String) async throws -> Project {
        try await map {
            switch try await client.createProject(body: .json(.init(name: name))) {
            case .created(let r): return try r.body.json
            case .badRequest(let r): throw APIError.server(400, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func launch(agentID: String, projectID: String?, values: [String: String]) async throws -> LaunchResult {
        try await map {
            let body = Components.Schemas.LaunchRequest(
                agentId: agentID, projectId: projectID,
                values: .init(additionalProperties: values))
            switch try await client.launchTask(body: .json(body)) {
            case .ok(let r): return try r.body.json
            case .badRequest(let r): throw APIError.server(400, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func tasks(status: String? = nil, perPage: Int = 50) async throws -> [WorkTask] {
        try await map {
            let status = status.flatMap(Components.Schemas.TaskStatus.init(rawValue:))
            switch try await client.listTasks(query: .init(page: 1, perPage: perPage, status: status)) {
            case .ok(let r): return try r.body.json.data
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func task(_ id: String) async throws -> WorkTask {
        try await map {
            switch try await client.getTask(path: .init(taskID: id)) {
            case .ok(let r): return try r.body.json
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func runs(taskID: String) async throws -> [TaskRun] {
        try await map {
            switch try await client.listTaskRuns(path: .init(taskID: taskID), query: .init(page: 1, perPage: 20)) {
            case .ok(let r): return try r.body.json.data
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func transitionTask(_ id: String, status: String) async throws -> WorkTask {
        try await map {
            guard let status = Components.Schemas.TaskStatus(rawValue: status) else {
                throw APIError.server(400, nil)
            }
            switch try await client.transitionTask(
                path: .init(taskID: id),
                body: .json(.init(status: status))
            ) {
            case .ok(let r): return try r.body.json
            case .badRequest(let r): throw APIError.server(400, try? r.body.json)
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func schedules(perPage: Int = 100) async throws -> [Schedule] {
        try await map {
            switch try await client.listSchedules(query: .init(page: 1, perPage: perPage)) {
            case .ok(let r): return try r.body.json.data
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func setScheduleStatus(_ id: String, status: String) async throws -> Schedule {
        try await map {
            switch try await client.updateSchedule(
                path: .init(taskID: id),
                body: .json(.init(status: Components.Schemas.ScheduleUpdate.StatusPayload(rawValue: status)))
            ) {
            case .ok(let r): return try r.body.json
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func approvals(status: String? = "pending") async throws -> [Approval] {
        try await map {
            let status = status.flatMap(Operations.ListApprovals.Input.Query.StatusPayload.init(rawValue:))
            switch try await client.listApprovals(query: .init(status: status)) {
            case .ok(let r): return try r.body.json
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func getApproval(_ id: String) async throws -> Approval {
        try await map {
            switch try await client.getApproval(path: .init(approvalID: id)) {
            case .ok(let r): return try r.body.json
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func decide(approvalID: String, approve: Bool, reason: String?) async throws -> Approval {
        try await map {
            let body = Operations.DecideApproval.Input.Body.JsonPayload(
                decision: approve ? .approve : .reject, reason: reason?.nilIfBlank)
            switch try await client.decideApproval(path: .init(approvalID: approvalID), body: .json(body)) {
            case .ok(let r): return try r.body.json
            case .badRequest(let r): throw APIError.server(400, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func devices() async throws -> [Device] {
        try await map {
            switch try await client.listDevices() {
            case .ok(let r): return try r.body.json.devices
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func revokeDevice(_ id: String) async throws {
        try await map {
            switch try await client.revokeDevice(path: .init(deviceID: id)) {
            case .noContent: return
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func setPushToken(deviceID: String, token: String, sandbox: Bool) async throws {
        try await map {
            let body = Operations.SetDevicePush.Input.Body.JsonPayload(
                token: token, environment: sandbox ? .sandbox : .production)
            switch try await client.setDevicePush(path: .init(deviceID: deviceID), body: .json(body)) {
            case .noContent: return
            case .notFound(let r): throw APIError.server(404, try? r.body.json)
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    public func logoutEverywhere() async throws {
        try await map {
            switch try await client.logoutAll() {
            case .noContent: return
            case .undocumented(let s, _): throw APIError.undocumented(s)
            }
        }
    }

    private func map<T>(_ body: () async throws -> T) async throws -> T {
        do { return try await body() } catch { throw APIError.from(error) }
    }
}

extension Agent {
    /// The specialist marker the platform seeds on builtin agents.
    public var builtin: String? {
        guard let raw = metadata?.additionalProperties.value["builtin"], let value = raw as? String else { return nil }
        return value.nilIfBlank
    }
}

extension String {
    var nilIfBlank: String? {
        let t = trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }
}
