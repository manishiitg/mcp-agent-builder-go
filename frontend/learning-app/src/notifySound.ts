// A short, synthesized "reply is ready" chime for Child Mode — no audio
// asset needed (avoids bundling/licensing a sound file for two tones). Off by
// default; a parent opts in from Settings because Quill can take anywhere
// from a few seconds to several minutes to reply (see turntrace.go's own
// logged totals), and a child who's wandered off in the meantime has no other
// way to know a reply actually landed.

const REMINDER_SOUND_KEY = 'sq-child-reminder-sound'

export function readReminderSoundPref(): boolean {
  try {
    return localStorage.getItem(REMINDER_SOUND_KEY) === '1'
  } catch {
    return false
  }
}

export function persistReminderSoundPref(on: boolean) {
  try {
    localStorage.setItem(REMINDER_SOUND_KEY, on ? '1' : '0')
  } catch {
    // ignore — worst case the preference doesn't survive a reload
  }
}

// Two quick ascending tones (a common, unobtrusive "notification" shape).
// Best-effort: a browser that refuses to let audio play without a fresher
// user gesture should never break the actual reply from showing up.
export function playReminderChime() {
  try {
    const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!Ctx) return
    const ctx = new Ctx()
    const now = ctx.currentTime
    const tone = (freq: number, start: number, duration: number) => {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.type = 'sine'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0, now + start)
      gain.gain.linearRampToValueAtTime(0.22, now + start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.001, now + start + duration)
      osc.connect(gain)
      gain.connect(ctx.destination)
      osc.start(now + start)
      osc.stop(now + start + duration + 0.05)
    }
    tone(660, 0, 0.15)
    tone(880, 0.16, 0.22)
    setTimeout(() => ctx.close().catch(() => {}), 600)
  } catch {
    // Sound is a nice-to-have — never let it throw into a reply-handling path.
  }
}
