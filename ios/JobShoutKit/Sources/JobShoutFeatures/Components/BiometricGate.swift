import LocalAuthentication

/// Asks for Face ID / Touch ID / passcode before a consequential action.
enum BiometricGate {
    /// True when the owner confirmed, or when the device has no passcode set
    /// (nothing to ask; the server still enforces who may act).
    static func confirm(reason: String) async -> Bool {
        let context = LAContext()
        var error: NSError?
        guard context.canEvaluatePolicy(.deviceOwnerAuthentication, error: &error) else {
            return (error as? LAError)?.code == .passcodeNotSet
        }
        return (try? await context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason)) ?? false
    }
}
