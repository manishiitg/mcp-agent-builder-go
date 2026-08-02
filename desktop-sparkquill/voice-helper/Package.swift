// swift-tools-version: 6.0
import PackageDescription

// Native streaming speech-to-text helper — see
// docs/refactor/native_streaming_stt.md.
//
// FluidAudio (Apache-2.0) is the only dependency. It is taken for
// StreamingEouAsrManager, which keeps encoder/decoder state across appends;
// the Python/MLX path this replaces has no stateful incremental API, so it has
// to re-transcribe the whole recording on every preview refresh.
let package = Package(
    name: "voice-helper",
    platforms: [.macOS(.v14)],
    dependencies: [
        .package(url: "https://github.com/FluidInference/FluidAudio.git", from: "0.15.5")
    ],
    targets: [
        .executableTarget(
            name: "voice-helper",
            dependencies: [.product(name: "FluidAudio", package: "FluidAudio")],
            path: "Sources/voice-helper"
        )
    ]
)
