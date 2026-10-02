import AuthenticationServices
import JobShoutAPI
import JobShoutAuth
import SwiftUI

struct SignInView: View {
    @Environment(AppModel.self) private var app
    @State private var mode: Mode = .signIn
    @State private var email = ""
    @State private var password = ""
    @State private var fullName = ""
    @State private var orgName = ""
    @State private var busy = false
    @State private var error: String?
    @State private var appleEnabled = false
    @State private var appleNonce: String?

    enum Mode: String, CaseIterable, Identifiable {
        case signIn = "Sign in", register = "Create account"
        var id: String { rawValue }
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("JobShout")
                            .font(.largeTitle.bold())
                        Text("Call agents, watch the work, approve what matters.")
                            .foregroundStyle(.secondary)
                    }
                    .listRowBackground(Color.clear)
                }

                if appleEnabled {
                    Section {
                        SignInWithAppleButton(mode == .signIn ? .signIn : .signUp) { request in
                            request.requestedScopes = [.fullName, .email]
                            if let appleNonce { request.nonce = AppleNonce.hash(appleNonce) }
                        } onCompletion: { result in
                            Task { await completeApple(result) }
                        }
                        .frame(height: 48)
                        .disabled(appleNonce == nil || busy)
                        .listRowInsets(EdgeInsets())
                    } footer: {
                        Text("Or use your email below.")
                    }
                }

                Section {
                    Picker("Mode", selection: $mode) {
                        ForEach(Mode.allCases) { Text($0.rawValue).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                Section {
                    TextField("Email", text: $email)
                        .emailField()
                        .textContentType(.username)
                        .accessibilityIdentifier("email")
                    SecureField("Password", text: $password)
                        .accessibilityIdentifier("password")
                        .textContentType(mode == .signIn ? .password : .newPassword)
                    if mode == .register {
                        TextField("Your name", text: $fullName)
                            .textContentType(.name)
                        TextField("Organisation", text: $orgName)
                            .textContentType(.organizationName)
                    }
                }

                if let error {
                    Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }

                Section {
                    Button {
                        Task { await submit() }
                    } label: {
                        HStack {
                            Spacer()
                            if busy { ProgressView() } else { Text(mode.rawValue).bold() }
                            Spacer()
                        }
                    }
                    .disabled(!canSubmit || busy)
                    .accessibilityIdentifier("submit")
                } footer: {
                    Text("Connected to \(app.environment.displayName).")
                }
            }
            .task { await prepareApple() }
        }
    }

    private var canSubmit: Bool {
        let base = email.contains("@") && password.count >= (mode == .register ? 8 : 1)
        return mode == .signIn ? base : base && fullName.count >= 2 && orgName.count >= 2
    }

    private func submit() async {
        busy = true
        error = nil
        defer { busy = false }
        do {
            switch mode {
            case .signIn: try await app.signIn(email: email, password: password)
            case .register: try await app.register(email: email, password: password, fullName: fullName, orgName: orgName)
            }
        } catch {
            self.error = APIError.from(error).message
        }
    }

    /// The nonce must exist before the button is tapped, because the Apple
    /// request is built synchronously. It is valid for ten minutes and
    /// single-use, so fetch a fresh one after every attempt.
    private func prepareApple() async {
        appleEnabled = await app.appleEnabled()
        guard appleEnabled else { return }
        appleNonce = try? await app.appleNonce()
    }

    private func completeApple(_ result: Result<ASAuthorization, Error>) async {
        defer { Task { await prepareApple() } }
        switch result {
        case .failure(let err):
            if (err as? ASAuthorizationError)?.code != .canceled {
                error = err.localizedDescription
            }
        case .success(let auth):
            guard let credential = auth.credential as? ASAuthorizationAppleIDCredential,
                  let tokenData = credential.identityToken,
                  let token = String(data: tokenData, encoding: .utf8),
                  let nonce = appleNonce else {
                error = "Apple didn't return a sign-in token."
                return
            }
            let name = credential.fullName.map { PersonNameComponentsFormatter().string(from: $0) }
            busy = true
            defer { busy = false }
            do {
                try await app.signInWithApple(identityToken: token, rawNonce: nonce, fullName: name?.isEmpty == false ? name : nil)
            } catch {
                self.error = APIError.from(error).message
            }
        }
    }
}
