import Foundation
import JobShoutAPI
import Observation
import SwiftUI

/// A value loaded from the API, with when it was last loaded so screens can
/// say how fresh it is instead of implying it's live.
@MainActor @Observable
public final class Resource<Value> {
    public private(set) var value: Value?
    public private(set) var error: APIError?
    public private(set) var loadedAt: Date?
    public private(set) var isLoading = false

    public init(_ value: Value? = nil) {
        self.value = value
    }

    public func load(_ fetch: @MainActor () async throws -> Value) async {
        isLoading = true
        defer { isLoading = false }
        do {
            value = try await fetch()
            loadedAt = .now
            error = nil
        } catch is CancellationError {
            // A newer load replaced this one.
        } catch {
            let api = APIError.from(error)
            // URLSession reports a cancelled task as URLError.cancelled.
            if (error as? URLError)?.code == .cancelled { return }
            self.error = api
        }
    }
}

/// Shows loading, the value, or a retryable error; keeps showing a stale
/// value with a banner when a refresh fails.
struct ResourceView<Value, Content: View>: View {
    let resource: Resource<Value>
    let retry: () async -> Void
    @ViewBuilder let content: (Value) -> Content

    var body: some View {
        if let value = resource.value {
            content(value)
                .safeAreaInset(edge: .top) {
                    if let error = resource.error {
                        ErrorBanner(message: error.message)
                    }
                }
        } else if let error = resource.error {
            ContentUnavailableView {
                Label("Couldn't load", systemImage: "exclamationmark.triangle")
            } description: {
                Text(error.message)
            } actions: {
                Button("Try again") { Task { await retry() } }
                    .buttonStyle(.borderedProminent)
            }
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

struct ErrorBanner: View {
    let message: String

    var body: some View {
        Label(message, systemImage: "exclamationmark.triangle.fill")
            .font(.footnote)
            .foregroundStyle(.white)
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.red.gradient, in: .rect(cornerRadius: 10))
            .padding(.horizontal)
            .accessibilityElement(children: .combine)
    }
}
