// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "PhotoFrame",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "PhotoFrame", targets: ["PhotoFrame"])],
    targets: [
        .executableTarget(name: "PhotoFrame"),
        .testTarget(name: "PhotoFrameTests", dependencies: ["PhotoFrame"])
    ]
)
