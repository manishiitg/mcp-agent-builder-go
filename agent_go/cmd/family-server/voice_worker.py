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
_tts_models = {}


def get_stt_model(name):
    if name not in _stt_models:
        from mlx_audio.stt.utils import load_model
        _stt_models[name] = load_model(name)
    return _stt_models[name]


def get_tts_model(name):
    if name not in _tts_models:
        from mlx_audio.tts.utils import load_model
        _tts_models[name] = load_model(name)
    return _tts_models[name]


def _silent_warmup_wav(path, seconds=1.0, sample_rate=16000):
    """A trivial, silent WAV file — good enough to trigger MLX's real
    first-inference compilation (which cares about input SHAPE, not content),
    without depending on the OTHER model having warmed up first. STT and TTS
    warm up in different orders depending on the caller (install warms TTS
    first, server startup warms STT first — see voice_mlx_env.go/main.go), so
    each warm-up path must be fully self-contained."""
    import wave
    import struct
    n_samples = int(seconds * sample_rate)
    with wave.open(path, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(sample_rate)
        w.writeframes(struct.pack(f"<{n_samples}h", *([0] * n_samples)))


def handle_load_tts(req):
    # Loading weights is NOT enough to make the first real use fast: MLX
    # lazily compiles/JIT's the computation graph on the first actual
    # inference, separately from loading weights. Measured directly: first
    # real generate_audio call after a fresh load took 3.456s; the second,
    # with everything already compiled, took 0.217s. Running one real
    # (discarded) synthesis here pays that one-time cost during install/
    # startup instead of on the parent's first real "Listen" click.
    from mlx_audio.tts.generate import generate_audio
    import tempfile
    model = get_tts_model(req["model"])
    with tempfile.TemporaryDirectory() as tmp:
        generate_audio(
            text="Ready.", model=model, voice="af_heart", lang_code="a",
            file_prefix=os.path.join(tmp, "warmup"), audio_format="wav",
            join_audio=True, verbose=False,
        )
    return {"status": "ok"}


def handle_load_stt(req):
    # Same reasoning as handle_load_tts — a real (discarded) transcription,
    # not just loaded weights, is what actually pays MLX's compilation cost.
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


def handle_speak(req):
    from mlx_audio.tts.generate import generate_audio
    model = get_tts_model(req["model"])
    out_dir = req["out_dir"]
    os.makedirs(out_dir, exist_ok=True)
    prefix = os.path.join(out_dir, "out")
    generate_audio(
        text=req["text"],
        model=model,
        voice=req["voice"],
        lang_code=req.get("lang_code", "a"),
        file_prefix=prefix,
        audio_format="wav",
        join_audio=True,
        verbose=False,
    )
    return {"path": prefix + ".wav"}


HANDLERS = {
    "transcribe": handle_transcribe,
    "speak": handle_speak,
    "load_stt": handle_load_stt,
    "load_tts": handle_load_tts,
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
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
