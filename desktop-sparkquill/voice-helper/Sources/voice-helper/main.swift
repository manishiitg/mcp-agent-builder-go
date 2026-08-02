import AVFoundation
import FluidAudio
import Foundation

// Persistent native STT worker. Deliberately speaks the SAME shape of protocol
// as the Python worker it replaces (voice_worker.py): one JSON object per line
// on stdin, one JSON object per line on stdout, and the literal line
// "WORKER_READY" on stderr once setup is done. voice_worker.go's handshake,
// warm-idle timeout, and teardown logic therefore carry over unchanged.
//
// Commands:
//   {"cmd":"load"}                  -> {"status":"ok"}   (downloads/loads weights)
//   {"cmd":"start"}                 -> {"status":"ok"}   (begin a new utterance)
//   {"cmd":"audio","pcm":"<b64>"}   -> {"partial":"..."} (b64 little-endian Float32, 16kHz mono)
//   {"cmd":"finish"}                -> {"text":"...","final":true}
//
// WHY ONE MODEL, NOT TWO
//
// This originally ran FluidAudio's StreamingEouAsrManager for the live preview
// and used the batch model only at the end. Two problems killed that, both
// found against a real microphone rather than synthetic audio:
//
//  1. It is built for voice-assistant TURN-TAKING. After sustained silence it
//     declares End-of-Utterance and expects a reset, so a mid-sentence pause
//     stopped transcription dead — observed live as a partial frozen on one
//     word for 100+ chunks of loud speech.
//  2. It needs ~2s of audio before emitting anything, so dictating a single
//     word produced no preview at all, which simply reads as broken.
//
// Both are inherent to that model, not tuning. The batch model runs ~120x
// realtime, so re-transcribing everything said so far costs ~60ms at five
// seconds of speech — cheap enough to drive the preview directly. That gives
// text immediately even for one word, punctuated, and IDENTICAL to the final
// text (no jarring rewrite when the user stops), from one model rather than
// two.
//
// The tradeoff is honest: cost grows with recording length, where a true
// streaming decoder's would not. previewInterval below keeps that bounded.

let sampleRate = 16000.0

func emit(_ object: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: object),
        let json = String(data: data, encoding: .utf8)
    else { return }
    FileHandle.standardOutput.write(Data((json + "\n").utf8))
}

let engine = UnifiedAsrManager()
var modelsLoaded = false

/// Every sample since `start` — both the preview and the final read this.
var utterance: [Float] = []
var lastPreview = ""
var lastPreviewAt = Date.distantPast
var lastPreviewSamples = 0

/// How long to wait between preview passes.
///
/// A pass costs roughly duration/120, so a fixed interval would eventually let
/// passes outrun it on a long dictation and back the request queue up. Scaling
/// the gap with length keeps each pass a small fraction of the interval: ~0.4s
/// apart for short speech, stretching out as the recording grows.
func previewInterval(forSamples count: Int) -> TimeInterval {
    max(0.4, (Double(count) / sampleRate) / 20.0)
}

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
            if !modelsLoaded {
                try await engine.loadModels()
                modelsLoaded = true
            }
            emit(["status": "ok"])

        case "start":
            utterance.removeAll(keepingCapacity: true)
            lastPreview = ""
            lastPreviewAt = .distantPast
            lastPreviewSamples = 0
            emit(["status": "ok"])

        case "audio":
            guard let encoded = request["pcm"] as? String,
                let raw = Data(base64Encoded: encoded)
            else {
                emit(["error": "missing or invalid pcm"])
                continue
            }
            utterance.append(contentsOf: raw.withUnsafeBytes { Array($0.bindMemory(to: Float.self)) })

            // Re-read the whole utterance, but only when due and only if new
            // audio actually arrived since the last pass.
            let due =
                Date().timeIntervalSince(lastPreviewAt) >= previewInterval(forSamples: utterance.count)
            if due, utterance.count > lastPreviewSamples, !utterance.isEmpty {
                lastPreview = try await engine.transcribe(utterance)
                lastPreviewAt = Date()
                lastPreviewSamples = utterance.count
            }
            emit(["partial": lastPreview])

        case "finish":
            // Always a fresh pass: the last preview can predate the final
            // moments of speech, and this is the text the user actually sends.
            let text = utterance.isEmpty ? "" : try await engine.transcribe(utterance)
            utterance.removeAll(keepingCapacity: true)
            lastPreview = ""
            lastPreviewSamples = 0
            emit(["text": text, "streamed": text, "final": true])

        default:
            emit(["error": "unknown cmd \(cmd)"])
        }
    } catch {
        emit(["error": "\(type(of: error)): \(error)"])
    }
}
