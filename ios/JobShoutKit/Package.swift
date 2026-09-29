// swift-tools-version: 6.0
import PackageDescription

// JobShoutKit holds everything but the app shell, so it builds and tests with
// `swift test` on macOS as well as in the iOS app. Layering (no upward imports):
//
//   JobShoutCore      environment, logging
//   JobShoutAPI       client generated from server/api/openapi.yaml + auth middleware
//   JobShoutAuth      Keychain session, sign-in flows
//   JobShoutLive      WebSocket live events
//   JobShoutFeatures  SwiftUI screens and their @Observable models
let package = Package(
    name: "JobShoutKit",
    platforms: [.iOS(.v18), .macOS(.v15)],
    products: [
        .library(name: "JobShoutFeatures", targets: ["JobShoutFeatures"]),
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-openapi-generator", from: "1.13.0"),
        .package(url: "https://github.com/apple/swift-openapi-runtime", from: "1.12.0"),
        .package(url: "https://github.com/apple/swift-openapi-urlsession", from: "1.3.0"),
        .package(url: "https://github.com/apple/swift-http-types", from: "1.0.0"),
    ],
    targets: [
        .target(name: "JobShoutCore"),
        .target(
            name: "JobShoutAPI",
            dependencies: [
                "JobShoutCore",
                .product(name: "OpenAPIRuntime", package: "swift-openapi-runtime"),
                .product(name: "OpenAPIURLSession", package: "swift-openapi-urlsession"),
                .product(name: "HTTPTypes", package: "swift-http-types"),
            ],
            plugins: [.plugin(name: "OpenAPIGenerator", package: "swift-openapi-generator")]
        ),
        .target(name: "JobShoutAuth", dependencies: ["JobShoutCore", "JobShoutAPI"]),
        .target(name: "JobShoutLive", dependencies: ["JobShoutCore", "JobShoutAPI"]),
        .target(name: "JobShoutFeatures", dependencies: ["JobShoutCore", "JobShoutAPI", "JobShoutAuth", "JobShoutLive"]),
        .testTarget(
            name: "JobShoutKitTests",
            dependencies: ["JobShoutCore", "JobShoutAPI", "JobShoutAuth", "JobShoutLive", "JobShoutFeatures"]
        ),
    ]
)
