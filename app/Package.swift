// swift-tools-version:5.10
import PackageDescription

let package = Package(
    name: "AmbientLight",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(name: "AmbientLight", path: "Sources/AmbientLight"),
        .testTarget(name: "AmbientLightTests", dependencies: ["AmbientLight"], path: "Tests/AmbientLightTests"),
    ]
)
