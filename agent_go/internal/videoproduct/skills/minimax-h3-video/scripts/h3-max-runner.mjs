#!/usr/bin/env node
/**
 * Durable MiniMax H3 Max queue runner for Video Studio.
 *
 * The input JSON is intentionally non-secret. Credentials come only from
 * SECRET_FAL_KEY / SECRET_FAL_AI_KEY (or their unprefixed equivalents) and
 * are never serialized or printed.
 */
import { appendFile, mkdir, readFile, rename, writeFile } from 'node:fs/promises'
import { spawn } from 'node:child_process'
import { basename, dirname, resolve } from 'node:path'

const QUEUE_ORIGIN = 'https://queue.fal.run'
const ROUTES = new Set([
  'minimax/h3-max/text-to-video',
  'minimax/h3-max/image-to-video',
  'minimax/h3-max/reference-to-video',
  'minimax/h3-max-turbo/image-to-video',
])
const H3_MAX_IMAGE_ROUTE = 'minimax/h3-max/image-to-video'
const H3_MAX_TURBO_IMAGE_ROUTE = 'minimax/h3-max-turbo/image-to-video'
const ASPECT_RATIOS = new Set(['adaptive', '21:9', '16:9', '4:3', '1:1', '3:4', '9:16'])

function usage(exitCode = 0) {
  console.log(`Usage:
  node ${basename(process.argv[1])} validate --input job.json
  node ${basename(process.argv[1])} submit --input job.json --state job-state.json
  node ${basename(process.argv[1])} status --state job-state.json
  node ${basename(process.argv[1])} wait --state job-state.json --output shot.mp4 [--timeout-seconds 900] [--poll-seconds 10]
  node ${basename(process.argv[1])} tail-reference --input predecessor.mp4 --output predecessor-tail.mp4 [--seconds 2|3]

Input JSON must include endpoint and prompt. endpoint is one of:
  minimax/h3-max/text-to-video
  minimax/h3-max/image-to-video
  minimax/h3-max/reference-to-video
  minimax/h3-max-turbo/image-to-video (initial image-controlled anchor only)

An omitted duration falls back to 15 seconds. Plan and send an explicit duration
for every shot; the fallback is not a reason to pad a shorter beat. The runner writes JSON-lines progress to stdout and <state>.log.jsonl. It never
re-submits an existing state file; use status or wait to resume that request.`)
  process.exit(exitCode)
}

function arg(name, { required = false, fallback } = {}) {
  const index = process.argv.indexOf(name)
  const value = index >= 0 ? process.argv[index + 1] : fallback
  if (required && (!value || value.startsWith('--'))) throw new Error(`Missing ${name}`)
  return value
}

const command = process.argv[2]
if (!command || command === '--help' || command === '-h') usage()

let logPath
let logWrites = Promise.resolve()
function event(kind, fields = {}) {
  const line = JSON.stringify({ timestamp: new Date().toISOString(), event: kind, ...fields })
  console.log(line)
  if (logPath) logWrites = logWrites.then(() => appendFile(logPath, `${line}\n`))
}

async function readJson(path) {
  try {
    return JSON.parse(await readFile(path, 'utf8'))
  } catch (error) {
    throw new Error(`Could not read JSON ${path}: ${error instanceof Error ? error.message : error}`)
  }
}

async function writeJsonAtomic(path, value) {
  await mkdir(dirname(path), { recursive: true })
  const temp = `${path}.tmp-${process.pid}`
  await writeFile(temp, `${JSON.stringify(value, null, 2)}\n`)
  await rename(temp, path)
}

function run(command, args) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''
    let stderr = ''
    child.stdout.on('data', chunk => { stdout += chunk })
    child.stderr.on('data', chunk => { stderr += chunk })
    child.on('error', error => reject(new Error(`Could not start ${command}: ${error.message}`)))
    child.on('close', code => code === 0
      ? resolvePromise({ stdout, stderr })
      : reject(new Error(`${command} failed (${code}): ${stderr.trim() || stdout.trim()}`)))
  })
}

async function mediaDuration(path) {
  const { stdout } = await run('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'default=noprint_wrappers=1:nokey=1', path])
  const duration = Number.parseFloat(stdout.trim())
  if (!Number.isFinite(duration) || duration <= 0) throw new Error(`ffprobe returned no usable duration for ${path}`)
  return duration
}

async function extractTailReference(inputPath, outputPath, seconds) {
  const sourceDuration = await mediaDuration(inputPath)
  if (sourceDuration < seconds) throw new Error(`Predecessor is ${sourceDuration.toFixed(2)}s; cannot extract the requested ${seconds}s continuity tail`)
  await mkdir(dirname(outputPath), { recursive: true })
  const temporary = `${outputPath}.tmp-${process.pid}.mp4`
  try {
    // Deterministic reference preparation only; do not modify delivery clips.
    await run('ffmpeg', ['-y', '-sseof', `-${seconds}`, '-i', inputPath, '-t', String(seconds), '-map', '0:v:0', '-map', '0:a?', '-c:v', 'libx264', '-preset', 'medium', '-crf', '18', '-c:a', 'aac', '-movflags', '+faststart', temporary])
    const actualDuration = await mediaDuration(temporary)
    if (actualDuration < seconds - 0.15 || actualDuration > seconds + 0.15) throw new Error(`Tail extraction produced ${actualDuration.toFixed(2)}s, expected ${seconds}s`)
    await rename(temporary, outputPath)
    event('continuity_tail_ready', { input_path: inputPath, output_path: outputPath, seconds, source_duration_seconds: sourceDuration, output_duration_seconds: actualDuration })
  } finally {
    await import('node:fs/promises').then(({ rm }) => rm(temporary, { force: true }))
  }
}

function isUrl(value) {
  try { return ['http:', 'https:'].includes(new URL(value).protocol) } catch { return false }
}

function validateInput(raw) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('Input must be a JSON object')
  const { endpoint } = raw
  if (!ROUTES.has(endpoint)) throw new Error(`endpoint must be one of: ${[...ROUTES].join(', ')}`)
  if (typeof raw.prompt !== 'string' || !raw.prompt.trim()) throw new Error('prompt is required')
  if (raw.prompt.length > 50000) throw new Error('prompt exceeds the 50,000-character endpoint limit')

  const input = { ...raw }
  delete input.endpoint
  input.prompt = input.prompt.trim()
  input.duration ??= 15
  input.resolution ??= '480P'
  input.prompt_expansion_mode ??= 'balanced'
  input.enable_safety_checker ??= true
  input.sync_mode ??= false

  if (!Number.isInteger(input.duration) || input.duration < 5 || input.duration > 15) throw new Error('duration must be an integer from 5 to 15 seconds')
  if (!['480P', '768P'].includes(input.resolution)) throw new Error('resolution must be 480P or 768P')
  if (!['balanced', 'quality'].includes(input.prompt_expansion_mode)) throw new Error('prompt_expansion_mode must be balanced or quality')
  if (typeof input.enable_safety_checker !== 'boolean' || typeof input.sync_mode !== 'boolean') throw new Error('enable_safety_checker and sync_mode must be booleans')
  if (input.sync_mode) throw new Error('sync_mode must be false: Video Studio requires a durable CDN result and local download')
  if (input.seed != null && !Number.isInteger(input.seed)) throw new Error('seed must be an integer when supplied')

  for (const field of ['reference_image_urls', 'reference_video_urls', 'reference_audio_urls']) {
    if (input[field] == null) continue
    if (!Array.isArray(input[field]) || input[field].some(value => typeof value !== 'string' || !isUrl(value))) {
      throw new Error(`${field} must be an array of HTTP(S) URLs`)
    }
  }
  const images = input.reference_image_urls ?? []
  const videos = input.reference_video_urls ?? []
  const audio = input.reference_audio_urls ?? []
  if (images.length > 9 || videos.length > 3 || audio.length > 3 || images.length + videos.length + audio.length > 12) {
    throw new Error('reference limits are 9 images, 3 videos, 3 audio clips, and 12 files total')
  }
  if (audio.length && !images.length && !videos.length) throw new Error('reference_audio_urls requires at least one image or video reference')

  if (endpoint === 'minimax/h3-max/text-to-video') {
    input.aspect_ratio ??= '16:9'
    if (!ASPECT_RATIOS.has(input.aspect_ratio) || input.aspect_ratio === 'adaptive') throw new Error('text-to-video aspect_ratio must be 21:9, 16:9, 4:3, 1:1, 3:4, or 9:16')
    if (images.length || videos.length || audio.length || input.image_url || input.end_image_url) throw new Error('text-to-video cannot include reference or image-frame fields; choose the matching H3 Max route')
  }
  if (endpoint === H3_MAX_IMAGE_ROUTE) {
    if (typeof input.image_url !== 'string' || !isUrl(input.image_url)) throw new Error('image-to-video requires an HTTP(S) image_url')
    if (input.end_image_url != null && (typeof input.end_image_url !== 'string' || !isUrl(input.end_image_url))) throw new Error('end_image_url must be an HTTP(S) URL when supplied')
    delete input.aspect_ratio
    if (images.length || videos.length || audio.length) throw new Error('image-to-video accepts image_url/end_image_url only; use reference-to-video for multimodal conditioning')
  }
  if (endpoint === H3_MAX_TURBO_IMAGE_ROUTE) {
    if (typeof input.image_url !== 'string' || !isUrl(input.image_url)) throw new Error('H3 Max Turbo is approved only for an initial image-controlled anchor and requires an HTTP(S) image_url')
    if (input.end_image_url != null && (typeof input.end_image_url !== 'string' || !isUrl(input.end_image_url))) throw new Error('end_image_url must be an HTTP(S) URL when supplied')
    if (input.end_image_url && !input.image_url) throw new Error('end_image_url requires image_url')
    if (input.aspect_ratio != null) throw new Error('H3 Max Turbo image-to-video follows image_url aspect ratio; do not send aspect_ratio')
    if (images.length || videos.length || audio.length) throw new Error('H3 Max Turbo is only for the initial image-controlled anchor; use H3 Max reference-to-video for continuity or multimodal conditioning')
  }
  if (endpoint === 'minimax/h3-max/reference-to-video') {
    input.aspect_ratio ??= 'adaptive'
    if (!ASPECT_RATIOS.has(input.aspect_ratio)) throw new Error(`reference-to-video aspect_ratio must be one of: ${[...ASPECT_RATIOS].join(', ')}`)
    if (!images.length && !videos.length && !audio.length) throw new Error('reference-to-video requires at least one reference; choose text-to-video for a prompt-only shot')
    if (input.image_url || input.end_image_url) throw new Error('reference-to-video uses reference_*_urls; do not mix image-to-video frame fields')
  }
  return { endpoint, input }
}

function credentials() {
  const key = process.env.SECRET_FAL_KEY || process.env.SECRET_FAL_AI_KEY || process.env.FAL_KEY || process.env.FAL_AI_KEY
  if (!key) throw new Error('Missing Fal credential: set SECRET_FAL_KEY, SECRET_FAL_AI_KEY, FAL_KEY, or FAL_AI_KEY')
  return key
}

async function api(url, { method = 'GET', body } = {}) {
  const response = await fetch(url, {
    method,
    headers: { Authorization: `Key ${credentials()}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const text = await response.text()
  let data
  try { data = text ? JSON.parse(text) : {} } catch { data = { raw: text.slice(0, 2000) } }
  if (!response.ok) throw new Error(`Fal ${method} ${new URL(url).pathname} failed (${response.status}): ${JSON.stringify(data)}`)
  return data
}

function statePath() { return resolve(arg('--state', { required: true })) }
async function loadState(path) {
  const state = await readJson(path)
  if (!state.request_id || !ROUTES.has(state.endpoint) || !state.input) throw new Error('Invalid runner state: endpoint, request_id, and input are required')
  return state
}

async function persist(path, state) {
  state.updated_at = new Date().toISOString()
  await writeJsonAtomic(path, state)
}

async function status(state, includeLogs = true) {
  const url = state.status_url || `${QUEUE_ORIGIN}/${state.endpoint}/requests/${encodeURIComponent(state.request_id)}/status`
  const update = await api(`${url}${includeLogs ? `${url.includes('?') ? '&' : '?'}logs=1` : ''}`)
  state.status = update.status
  state.last_status = update
  state.status_history ??= []
  const previous = state.status_history.at(-1)?.status
  if (previous !== update.status) {
    state.status_history.push({ at: new Date().toISOString(), status: update.status, queue_position: update.queue_position ?? null })
    event('queue_status', { request_id: state.request_id, status: update.status, queue_position: update.queue_position ?? null })
  }
  const messages = Array.isArray(update.logs)
    ? update.logs
    : Array.isArray(update.logs?.logs)
      ? update.logs.logs
      : update.logs && typeof update.logs === 'object'
        ? Object.values(update.logs)
        : []
  for (const log of messages) event('provider_log', { request_id: state.request_id, message: typeof log === 'string' ? log : log.message ?? JSON.stringify(log) })
  return update
}

async function downloadResult(state, outputPath) {
  const resultUrl = state.response_url || `${QUEUE_ORIGIN}/${state.endpoint}/requests/${encodeURIComponent(state.request_id)}`
  event('result_fetch_started', { request_id: state.request_id })
  const result = await api(resultUrl)
  const videoUrl = result?.video?.url
  if (!isUrl(videoUrl)) throw new Error(`Completed request returned no video.url: ${JSON.stringify(result)}`)
  const response = await fetch(videoUrl)
  if (!response.ok) throw new Error(`Video download failed (${response.status})`)
  const bytes = Buffer.from(await response.arrayBuffer())
  if (!bytes.length) throw new Error('Video download was empty')
  await mkdir(dirname(outputPath), { recursive: true })
  await writeFile(outputPath, bytes)
  state.result = { video_url: videoUrl, seed: result.seed ?? null, timings: result.timings ?? null, expanded_prompt: result.expanded_prompt ?? null }
  state.output_path = outputPath
  event('download_complete', { request_id: state.request_id, output_path: outputPath, bytes: bytes.length, timings: result.timings ?? null })
}

async function main() {
  if (!['validate', 'submit', 'status', 'wait', 'tail-reference'].includes(command)) usage(1)
  if (command === 'tail-reference') {
    const seconds = Number(arg('--seconds', { fallback: '3' }))
    if (!Number.isInteger(seconds) || seconds < 2 || seconds > 3) throw new Error('continuity-tail seconds must be either 2 or 3')
    const input = resolve(arg('--input', { required: true }))
    const output = resolve(arg('--output', { required: true }))
    if (input === output) throw new Error('continuity-tail input and output must be different files')
    await extractTailReference(input, output, seconds)
    return
  }
  const state = command === 'validate' || command === 'submit' ? null : await loadState(statePath())
  if (command === 'validate') {
    const job = validateInput(await readJson(resolve(arg('--input', { required: true }))))
    event('input_validated', { endpoint: job.endpoint, duration: job.input.duration, resolution: job.input.resolution })
    return
  }
  if (command === 'submit') {
    const path = statePath()
    try { await readFile(path); throw new Error(`State already exists at ${path}; use status or wait to resume instead of submitting again`) } catch (error) { if (error?.code !== 'ENOENT') throw error }
    const job = validateInput(await readJson(resolve(arg('--input', { required: true }))))
    logPath = `${path}.log.jsonl`
    event('submit_started', { endpoint: job.endpoint, duration: job.input.duration, resolution: job.input.resolution })
    const queued = await api(`${QUEUE_ORIGIN}/${job.endpoint}`, { method: 'POST', body: job.input })
    if (!queued.request_id) throw new Error(`Fal queue response omitted request_id: ${JSON.stringify(queued)}`)
    const next = {
      version: 1, endpoint: job.endpoint, input: job.input, request_id: queued.request_id,
      status: queued.status, status_url: queued.status_url, response_url: queued.response_url,
      submitted_at: new Date().toISOString(), status_history: [{ at: new Date().toISOString(), status: queued.status, queue_position: queued.queue_position ?? null }],
    }
    await persist(path, next)
    event('submitted', { request_id: next.request_id, status: next.status, state: path })
    return
  }
  const path = statePath()
  logPath = `${path}.log.jsonl`
  if (command === 'status') {
    const update = await status(state)
    await persist(path, state)
    event('status_complete', { request_id: state.request_id, status: update.status })
    return
  }
  const output = resolve(arg('--output', { required: true }))
  const timeoutSeconds = Number(arg('--timeout-seconds', { fallback: '900' }))
  const pollSeconds = Number(arg('--poll-seconds', { fallback: '10' }))
  if (!Number.isFinite(timeoutSeconds) || timeoutSeconds <= 0 || !Number.isFinite(pollSeconds) || pollSeconds < 2) throw new Error('timeout-seconds must be positive and poll-seconds must be at least 2')
  const deadline = Date.now() + timeoutSeconds * 1000
  event('wait_started', { request_id: state.request_id, timeout_seconds: timeoutSeconds, poll_seconds: pollSeconds })
  while (Date.now() < deadline) {
    const update = await status(state)
    await persist(path, state)
    if (update.status === 'COMPLETED') {
      await downloadResult(state, output)
      await persist(path, state)
      event('completed', { request_id: state.request_id, output_path: output })
      return
    }
    if (!['IN_QUEUE', 'IN_PROGRESS'].includes(update.status)) throw new Error(`Provider terminal failure for ${state.request_id}: ${JSON.stringify(update)}`)
    await new Promise(resolve => setTimeout(resolve, pollSeconds * 1000))
  }
  await persist(path, state)
  throw new Error(`Timed out waiting locally after ${timeoutSeconds}s; request ${state.request_id} remains resumable via status or wait`)
}

main()
  .catch(error => {
    event('error', { message: error instanceof Error ? error.message : String(error) })
    process.exitCode = 1
  })
  .finally(() => logWrites.catch(() => {}))
