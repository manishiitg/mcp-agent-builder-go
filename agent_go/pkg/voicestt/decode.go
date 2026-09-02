package voicestt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ReadWAV parses a RIFF/WAVE file holding 16-bit PCM and returns mono float32
// samples plus the file's sample rate. Multi-channel audio is averaged down
// to mono. It does not resample; callers needing SampleRate use DecodeFile.
func ReadWAV(path string) (samples []float32, sampleRate int, err error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: caller-supplied media path, already access-checked by the caller.
	if err != nil {
		return nil, 0, err
	}
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, errors.New("not a WAV file")
	}
	var channels, bits int
	var dataOff, dataLen int
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return nil, 0, errors.New("truncated fmt chunk")
			}
			format := binary.LittleEndian.Uint16(b[body : body+2])
			channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
			// 1 = PCM, 0xFFFE = WAVE_FORMAT_EXTENSIBLE (PCM in practice here).
			if format != 1 && format != 0xFFFE {
				return nil, 0, fmt.Errorf("unsupported WAV format %d", format)
			}
		case "data":
			dataOff, dataLen = body, size
		}
		off = body + size + size%2
	}
	if dataLen == 0 || sampleRate == 0 || channels == 0 {
		return nil, 0, errors.New("WAV has no audio data")
	}
	if bits != 16 {
		return nil, 0, fmt.Errorf("unsupported WAV bit depth %d", bits)
	}
	if dataOff+dataLen > len(b) {
		dataLen = len(b) - dataOff
	}
	frames := dataLen / (2 * channels)
	samples = make([]float32, frames)
	for i := 0; i < frames; i++ {
		var sum float32
		for c := 0; c < channels; c++ {
			p := dataOff + (i*channels+c)*2
			sum += float32(int16(binary.LittleEndian.Uint16(b[p:p+2]))) / 32768.0 //nolint:gosec // G115: PCM16 reinterpretation.
		}
		samples[i] = sum / float32(channels)
	}
	return samples, sampleRate, nil
}

// DecodeFile turns any audio file the platform can read into 16kHz mono
// float32 samples ready for Manager.Transcribe. A 16kHz PCM WAV is parsed
// directly; anything else (WhatsApp's ogg/opus voice notes, a browser's
// webm/opus or mp4 recording) is converted by the first available tool:
// macOS's built-in afconvert (no install, decodes Opus since macOS 15), then
// ffmpeg. The engine itself only ever sees PCM, so this is the one place a
// container or codec is dealt with.
func DecodeFile(ctx context.Context, path string) ([]float32, error) {
	if samples, rate, err := ReadWAV(path); err == nil && rate == SampleRate {
		return samples, nil
	}
	tmp, err := os.CreateTemp("", "voicestt-*.wav")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	var errs []string
	for _, conv := range converters() {
		if _, lookErr := exec.LookPath(conv.bin); lookErr != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, conv.bin, conv.args(path, tmpPath)...) //nolint:gosec // G204: fixed converter binaries, args are paths.
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", conv.bin, strings.TrimSpace(lastLine(string(out)))))
			continue
		}
		samples, rate, readErr := ReadWAV(tmpPath)
		if readErr != nil {
			errs = append(errs, fmt.Sprintf("%s produced unreadable WAV: %v", conv.bin, readErr))
			continue
		}
		if rate != SampleRate {
			errs = append(errs, fmt.Sprintf("%s produced %dHz, want %d", conv.bin, rate, SampleRate))
			continue
		}
		return samples, nil
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("cannot decode %s: no audio converter found (afconvert on macOS or ffmpeg)", filepath.Base(path))
	}
	return nil, fmt.Errorf("cannot decode %s: %s", filepath.Base(path), strings.Join(errs, "; "))
}

type converter struct {
	bin  string
	args func(in, out string) []string
}

func converters() []converter {
	var list []converter
	if runtime.GOOS == "darwin" {
		list = append(list, converter{bin: "afconvert", args: func(in, out string) []string {
			// -d LEI16@16000: little-endian 16-bit PCM at 16kHz; -c 1: mono.
			return []string{"-f", "WAVE", "-d", "LEI16@16000", "-c", "1", in, out}
		}})
	}
	list = append(list, converter{bin: "ffmpeg", args: func(in, out string) []string {
		return []string{"-loglevel", "error", "-y", "-i", in, "-ar", fmt.Sprint(SampleRate), "-ac", "1", "-c:a", "pcm_s16le", out}
	}})
	return list
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
