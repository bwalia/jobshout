import JobShoutAPI
import SwiftUI

struct MeView: View {
    @Environment(AppModel.self) private var app
    @State private var me = Resource<User>()
    @State private var devices = Resource<[Device]>()
    @State private var confirmEverywhere = false
    @State private var error: String?

    var body: some View {
        List {
            Section {
                if let user = me.value {
                    HStack(spacing: 14) {
                        AgentAvatar(name: user.fullName, size: 52)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(user.fullName).font(.headline)
                            Text(user.email).foregroundStyle(.secondary)
                        }
                    }
                } else {
                    ProgressView()
                }
            }

            Section {
                ForEach(devices.value ?? [], id: \.id) { device in
                    DeviceRow(device: device, isThisDevice: device.id == app.deviceID)
                        .swipeActions {
                            if device.id != app.deviceID {
                                Button("Sign out", role: .destructive) { Task { await revoke(device) } }
                            }
                        }
                }
            } header: {
                Text("Signed-in devices")
            } footer: {
                Text("Swipe a device to sign it out. Signing out removes its notifications too.")
            }

            if let error {
                Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
            }

            Section {
                LabeledContent("Environment", value: app.environment.displayName)
                LabeledContent("Server", value: app.environment.host)
            }

            Section {
                Button("Sign out") { Task { await app.signOut() } }
                Button("Sign out on all devices", role: .destructive) { confirmEverywhere = true }
            }
        }
        .navigationTitle("Me")
        .refreshable { await load() }
        .task { await load() }
        .confirmationDialog("Sign out everywhere?", isPresented: $confirmEverywhere, titleVisibility: .visible) {
            Button("Sign out on all devices", role: .destructive) {
                Task {
                    do { try await app.signOutEverywhere() } catch { self.error = APIError.from(error).message }
                }
            }
        } message: {
            Text("Every phone and browser signed in to your account will need to sign in again.")
        }
    }

    private func load() async {
        async let m: Void = me.load { try await app.api.me() }
        async let d: Void = devices.load { try await app.api.devices() }
        _ = await (m, d)
    }

    private func revoke(_ device: Device) async {
        do {
            try await app.api.revokeDevice(device.id)
            await devices.load { try await app.api.devices() }
        } catch {
            self.error = APIError.from(error).message
        }
    }
}

struct DeviceRow: View {
    let device: Device
    let isThisDevice: Bool

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: device.platform == "ios" ? "iphone" : "desktopcomputer")
                .font(.title3)
                .foregroundStyle(.secondary)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(device.name.isEmpty ? "iPhone" : device.name).font(.body)
                    if isThisDevice {
                        Text("This device").font(.caption.bold()).foregroundStyle(.tint)
                    }
                }
                Text("Version \(device.appVersion) · active \(device.lastSeenAt.formatted(.relative(presentation: .named)))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if device.hasPush {
                Image(systemName: "bell.fill").foregroundStyle(.secondary).accessibilityLabel("Notifications on")
            }
        }
    }
}
