// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "CoWorkLocationHelper",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "CoWorkLocationHelper", targets: ["CoWorkLocationHelper"]),
    ],
    targets: [
        .executableTarget(
            name: "CoWorkLocationHelper",
            path: "Sources/CoWorkLocationHelper",
            swiftSettings: [
                .unsafeFlags(["-parse-as-library"])
            ]
        ),
    ]
)
