import AVFoundation
import FluidAudio
import Foundation

// Persistent native STT worker. Deliberately speaks the SAME shape of protocol
// as the Python worker it replaces (voice_worker.py): one JSON object per line
// on stdin, one JSON object per line on stdout, and the literal line
// "WORKER_READY" on stderr once imports/setup are done. voice_worker.go's
// handshake, warm-idle timeout, and teardown logic therefore carry over
// unchanged rather than being rewritten against a new transport.
//
// The difference that matters is statefulness: `audio` appends only the NEW
// samples to a live decoder and reads the running transcript back, instead of
// re-transcribing the whole recording from the start every time. Cost per
// refresh is flat in recording length rather than growing with it — the whole
// point of this helper (see docs/refactor/native_streaming_stt.md).
//
// Commands:
//   {"cmd":"load"}                  -> {"status":"ok"}   (downloads/loads weights)
//   {"cmd":"start"}                 -> {"status":"ok"}   (begin a new utterance)
//   {"cmd":"audio","pcm":"<b64>"}   -> {"partial":"..."} (b64 of little-endian Float32, 16kHz mono)
//   {"cmd":"finish"}                -> {"text":"...","final":true}
//
// Audio is base64 rather than a binary side-channel on purpose: it keeps the
// one-JSON-object-per-line contract debuggable by hand (the existing worker's
// stated reason for that shape), and at 16kHz mono a 160ms chunk is ~10KB raw
// — the encoding overhead is not a real cost at this rate.

let sampleRate = 16000.0

func emit(_ object: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: object),
        let json = String(data: data, encoding: .utf8)
    else { return }
    FileHandle.standardOutput.write(Data((json + "\n").utf8))
}

/// FluidAudio takes AVAudioPCMBuffer; the wire carries raw Float32 samples.
func makeBuffer(_ samples: [Float]) -> AVAudioPCMBuffer? {
    guard
        let format = AVAudioFormat(
            commonFormat: .pcmFormatFloat32,
            sampleRate: sampleRate,
            channels: 1,
            interleaved: false
        ),
        let buffer = AVAudioPCMBuffer(
            pcmFormat: format,
            frameCapacity: AVAudioFrameCount(samples.count)
        )
    else { return nil }

    buffer.frameLength = AVAudioFrameCount(samples.count)
    if let channel = buffer.floatChannelData {
        samples.withUnsafeBufferPointer { source in
            guard let base = source.baseAddress else { return }
            channel[0].update(from: base, count: samples.count)
        }
    }
    return buffer
}

// Chunk size selects a genuinely DIFFERENT model (parakeetEou160/320/1280),
// not just a buffering parameter — so transcript quality, not only latency,
// varies with it. Configurable here so the variants can be compared on the
// same audio; see docs/refactor/native_streaming_stt.md.
let chunkSize: StreamingChunkSize = {
    switch ProcessInfo.processInfo.environment["VOICE_HELPER_CHUNK_MS"] {
    case "1280": return .ms1280
    case "320": return .ms320
    default: return .ms160
    }
}()

let engine = StreamingEouAsrManager(chunkSize: chunkSize)
var modelsLoaded = false

// Go waits for this exact line on stderr before sending any request — the
// signal that setup finished, kept distinct from stdout (which carries only
// JSON responses, and would be corrupted by anything else landing on it).
FileHandle.standardError.write(Data("WORKER_READY\n".utf8))

while let line = readLine(strippingNewline: true) {
    let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
    if trimmed.isEmpty { continue }

    guard let data = trimmed.data(using: .utf8),
        let request = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
        let cmd = request["cmd"] as? String
    else {
        emit(["error": "malformed request"])
        continue
    }

    do {
        switch cmd {
        case "load":
            // Downloads the CoreML weights on first run, then loads them.
            if !modelsLoaded {
                try await engine.loadModels()
                modelsLoaded = true
            }
            emit(["status": "ok"])

        case "start":
            await engine.reset()
            emit(["status": "ok"])

        case "audio":
            guard let encoded = request["pcm"] as? String,
                let raw = Data(base64Encoded: encoded)
            else {
                emit(["error": "missing or invalid pcm"])
                continue
            }
            let samples = raw.withUnsafeBytes { Array($0.bindMemory(to: Float.self)) }
            if !samples.isEmpty, let buffer = makeBuffer(samples) {
                try await engine.appendAudio(buffer)
                try await engine.processBufferedAudio()
            }
            emit(["partial": await engine.getPartialTranscript()])

        case "finish":
            let text = try await engine.finish()
            await engine.reset()
            emit(["text": text, "final": true])

        default:
            emit(["error": "unknown cmd \(cmd)"])
        }
    } catch {
        // Reported to the caller rather than swallowed, matching the Python
        // worker's error contract.
        emit(["error": "\(type(of: error)): \(error)"])
    }
}
