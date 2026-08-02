import sys
import json
import os
import contextlib

# Persistent MLX voice worker. Started once by voice_worker.go and kept warm
# for as long as the family is actively using voice features — this exists
# because a fresh `python -c` process pays ~1.9s just re-importing mlx_audio
# EVERY call, before any real work happens (measured directly: ~1.9s import,
# ~0.8s actual model-load + inference). Loading each model ONCE here and
# reusing the in-memory object for every subsequent request eliminates both
# the repeated import cost and the repeated model reload.

_stt_models = {}


def get_stt_model(name):
    if name not in _stt_models:
        from mlx_audio.stt.utils import load_model
        _stt_models[name] = load_model(name)
    return _stt_models[name]


def _silent_warmup_wav(path, seconds=1.0, sample_rate=16000):
    """A trivial, silent WAV file — good enough to trigger MLX's real
    first-inference compilation (which cares about input SHAPE, not content)."""
    import wave
    import struct
    n_samples = int(seconds * sample_rate)
    with wave.open(path, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(sample_rate)
        w.writeframes(struct.pack(f"<{n_samples}h", *([0] * n_samples)))


def handle_load_stt(req):
    # Loading weights is NOT enough to make the first real use fast: MLX
    # lazily compiles/JIT's the computation graph on the first actual
    # inference, separately from loading weights. A real (discarded)
    # transcription here pays that one-time cost during install/startup
    # instead of on the parent's first real use.
    from mlx_audio.stt.generate import generate_transcription
    import tempfile
    model = get_stt_model(req["model"])
    with tempfile.TemporaryDirectory() as tmp:
        clip = os.path.join(tmp, "warmup.wav")
        _silent_warmup_wav(clip)
        generate_transcription(model=model, audio=clip, verbose=False)
    return {"status": "ok"}


def handle_transcribe(req):
    from mlx_audio.stt.generate import generate_transcription
    model = get_stt_model(req["model"])
    result = generate_transcription(model=model, audio=req["audio_path"], verbose=False)
    return {"text": result.text}


HANDLERS = {
    "transcribe": handle_transcribe,
    "load_stt": handle_load_stt,
}


@contextlib.contextmanager
def stdout_isolated():
    """Redirects the process's REAL stdout (file descriptor 1, not just the
    Python-level sys.stdout object) to /dev/null for the duration of the
    block. Necessary because mlx-audio prints plain (ANSI-colored) status
    lines straight to stdout even with verbose=False — confirmed directly:
    reassigning sys.stdout alone does NOT catch everything some libraries
    write, since certain output goes straight to the fd. Stdout here is our
    ONLY channel back to Go (one JSON object per line); anything else landing
    on it corrupts the protocol. Progress bars (tqdm) already default to
    stderr and are unaffected — this only isolates stdout.
    """
    sys.stdout.flush()
    saved_fd = os.dup(1)
    devnull_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(devnull_fd, 1)
    os.close(devnull_fd)
    try:
        yield
    finally:
        sys.stdout.flush()
        os.dup2(saved_fd, 1)
        os.close(saved_fd)


def main():
    # Go waits for this exact line on stderr before sending any requests —
    # it's the signal that imports have finished and the process can accept
    # work, distinct from stdout (which carries only JSON responses).
    sys.stderr.write("WORKER_READY\n")
    sys.stderr.flush()

    # MLX's cache limit defaults to the memory limit (i.e. unbounded) — its
    # own docs say so plainly. That's fine for a short-lived script, but this
    # process stays warm for up to voiceWorkerIdleTimeout (15 real minutes,
    # see voice_worker.go) and can field many calls in that window: every
    # live-preview tick during a recording (every ~1.2s — see
    # LIVE_PREVIEW_INTERVAL_MS in useMicDictation.ts) on top of every real
    # WhatsApp voice note and mic dictation. On Apple Silicon's UNIFIED memory,
    # MLX's growing cache is the SAME physical RAM everything else on the
    # machine needs — left unmanaged, a long voice-heavy session can run the
    # whole system out of memory, not just this process. mx.clear_cache()
    # after every single request keeps this process's footprint bounded by
    # "one inference's worth," not "every inference this process has ever run."
    import mlx.core as mx

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            with stdout_isolated():
                resp = HANDLERS[req["cmd"]](req)
        except Exception as e:  # noqa: BLE001 - reported to the caller, not swallowed
            resp = {"error": f"{type(e).__name__}: {e}"}
        finally:
            mx.clear_cache()
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
