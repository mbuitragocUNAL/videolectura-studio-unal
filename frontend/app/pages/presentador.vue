<script setup lang="ts">
/**
 * The window that gets recorded. Lineamiento 2.b allows nothing on screen but
 * the page of the book, so this route has no interface at all: every control
 * arrives over the channel from the console.
 *
 * It also owns the microphone and the recording clock. That ownership matters:
 * a paused recording keeps wall time running but produces no frames, so a cue
 * timed against `Date.now()` would land later in the file than it did in the
 * room. Everything the take remembers — cues, levels, scroll trace — is stamped
 * against *recorded* time, which only this window can measure.
 */
definePageMeta({ layout: false })

const studio = useStudio()

// The pages come from tira.json, which the preparation step writes for each
// reading. Lectura 07 predates it and keeps its twelve pages as a fallback.
// Each page carries its size so the strip has its full height before a single
// image has arrived: measuring a strip of unloaded images gave a height of 0,
// and with that the page could not scroll at all.
interface StripPage { archivo: string; ancho?: number; alto?: number }
const PAGES = ref<StripPage[]>([])
async function loadPages() {
  try {
    const tira = await $fetch<{ paginas: StripPage[] }>('/assets/tira.json')
    PAGES.value = tira.paginas
  } catch {
    PAGES.value = Array.from({ length: 12 }, (_, i) => ({ archivo: `t-0${61 + i}.png` }))
  }
}

// Bound, not literal: the assets are served by the Go studio at runtime, and a
// literal src would make Vite try to resolve it as a module at build time.
// A reading prepared by preparar/lector.py names its lámina; lectura 07 does not.
const figureId = ref('')
const PLATE = computed(() => {
  const id = figureId.value || studio.reading.value?.lamina?.figura
  return id ? `/assets/figuras/${id}-pie.png` : '/assets/paginas/laminaF-446.png'
})
const CHANNEL = 'estoc-studio'

const strip = useTemplateRef<HTMLElement>('strip')
const plateVisible = ref(false)
const arrowAt = ref<number | null>(null)

// `height` is the whole strip; `max` is as far as it can scroll. Both matter,
// and confusing them is what put the console out of step with the page.
const scroll = reactive({ y: 0, max: 0, height: 0, speed: 18, running: false })

/** idle → armed (mic open, metering) → recording ⇄ paused → armed */
type Phase = 'idle' | 'armed' | 'recording' | 'paused'
const phase = ref<Phase>('idle')
const muted = ref(false)
const deviceId = ref<string | null>(null)
const devices = ref<{ id: string; label: string }[]>([])

let channel: BroadcastChannel | null = null
let micStream: MediaStream | null = null
let screenStream: MediaStream | null = null
let meter: { ctx: AudioContext; timer: ReturnType<typeof setInterval> } | null = null

/**
 * Everything one take accumulates, owned by the recorder that produces it.
 *
 * These used to be module-level and were cleared at the top of startRecording.
 * That is a race, and a destructive one. stopRecording() sets the phase
 * synchronously, but MediaRecorder fires its last `dataavailable` and its
 * `onstop` afterwards — so a take begun in that gap cleared the buffers out
 * from under a recorder that was still finishing, and the old recorder's
 * trailing chunks landed in the new take's array. What reached the disk was a
 * WebM with a foreign block in front of its EBML header, which no player and
 * no ffprobe will open, and a cue/level/trace set belonging to the wrong take.
 * A capture now belongs to its recorder, and nothing else can reach it.
 */
interface Capture {
  recorder: MediaRecorder
  chunks: Blob[]
  cues: { quote: string; at: number }[]
  levels: { t: number; r: number }[]
  trace: { t: number; f: number }[]
  /** Milliseconds banked before the current segment: the clock stops on pause. */
  recordedMs: number
  segmentStart: number
}

let capture: Capture | null = null

/** Seconds of actual recording so far. */
function recordedSeconds() {
  if (!capture) return 0
  const live = phase.value === 'recording' ? performance.now() - capture.segmentStart : 0
  return (capture.recordedMs + live) / 1000
}

/* ---------------- scroll ---------------- */

function measure() {
  if (!strip.value) return
  scroll.height = strip.value.scrollHeight
  scroll.max = Math.max(0, scroll.height - window.innerHeight)
}

function paint() {
  scroll.y = Math.min(Math.max(scroll.y, 0), scroll.max)
  if (strip.value) strip.value.style.transform = `translateY(${-scroll.y}px)`
}

/**
 * Where the reader's eye is, as a fraction of the whole strip.
 *
 * Not the top of the window: nobody reads the top edge of a page. The line
 * being spoken sits a little above the middle, with what has just been said
 * still visible above it and what is coming already in view below — which is
 * what makes a prompter readable instead of a race against the top border.
 *
 * It is also the coordinate everything else speaks in, so seeking to a
 * section or a line puts that line where the eye already is rather than
 * against the top border — which is progressively wrong the further into the
 * strip you are.
 */
const ANCHOR = 0.4

const anchorPx = () => window.innerHeight * ANCHOR

/**
 * Where the top of the window sits, as a fraction of the whole strip.
 *
 * It must be divided by the strip's full height, because that is what
 * marcas.py divides by when it places every line, section and quote: `y /
 * total`. Dividing by `scroll.max` — the strip minus one screen — looks almost
 * right at the top of the reading and drifts steadily the rest of the way,
 * about seven lines by the middle and more than a page by the end. Sections
 * landed in the wrong paragraph and cues fired against the wrong words.
 */
const position = () => (scroll.height ? (scroll.y + anchorPx()) / scroll.height : 0)

/** How much of the strip one screen shows, so the console can say what is on it. */
const viewport = () => (scroll.height ? window.innerHeight / scroll.height : 0)

/** Put a strip fraction under the reader's eye, without scrolling past the end. */
function goTo(f: number) {
  scroll.y = Math.min(Math.max(f * scroll.height - anchorPx(), 0), scroll.max)
}

// The strip changes height while its images load and whenever the window is
// resized. Re-measure every time, keeping the same line under the eye, so a
// seek is never clamped against a height that is no longer true.
useResizeObserver(strip, () => {
  const f = position()
  measure()
  if (f) goTo(f)
  paint()
})

let last: number | null = null
function loop(now: number) {
  // The page only ever moves because the reader asked it to. A quote used to
  // drag the scroll along with its audio, which made sense when one take
  // covered a whole dialogue; recording a paragraph at a time it moved the
  // very sentence being spoken out from under the reader.
  if (last !== null && scroll.running) {
    scroll.y += (scroll.speed * (now - last)) / 1000
    paint()
  }
  last = now
  requestAnimationFrame(loop)
}

/* ---------------- microphone ---------------- */

async function listDevices() {
  const all = await navigator.mediaDevices.enumerateDevices()
  devices.value = all
    .filter(d => d.kind === 'audioinput')
    .map(d => ({ id: d.deviceId, label: d.label || 'Micrófono sin nombre' }))
}

/**
 * Open the microphone and start metering, without recording anything. This is
 * the pre-flight: you can see which device is live and watch your own level
 * before committing to a take.
 */
async function armMic(id?: string | null) {
  try {
    releaseMic()
    micStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        // The browser's cleanup pumps the noise floor between sentences and
        // fights the mix in post.
        echoCancellation: false,
        noiseSuppression: false,
        autoGainControl: false,
        ...(id ? { deviceId: { exact: id } } : {}),
      },
    })
    deviceId.value = micStream.getAudioTracks()[0]?.getSettings().deviceId ?? id ?? null
    applyMute()
    startMeter(micStream)
    if (phase.value === 'idle') phase.value = 'armed'
    // Labels are only exposed once permission has been granted.
    await listDevices()
    report()
  } catch (err) {
    fail(err)
  }
}

function releaseMic() {
  stopMeter()
  micStream?.getTracks().forEach(t => t.stop())
  micStream = null
}

function applyMute() {
  micStream?.getAudioTracks().forEach((t) => { t.enabled = !muted.value })
}

function startMeter(stream: MediaStream) {
  stopMeter()
  const ctx = new AudioContext()
  const analyser = ctx.createAnalyser()
  analyser.fftSize = 2048
  ctx.createMediaStreamSource(stream).connect(analyser)
  const data = new Float32Array(analyser.fftSize)

  // 20 Hz is plenty to watch levels and cheap enough to ship over the channel.
  const timer = setInterval(() => {
    analyser.getFloatTimeDomainData(data)
    let sum = 0
    let peak = 0
    for (const v of data) {
      sum += v * v
      const a = Math.abs(v)
      if (a > peak) peak = a
    }
    const rms = muted.value ? 0 : Math.sqrt(sum / data.length)
    channel?.postMessage({ type: 'level', rms, peak: muted.value ? 0 : peak })

    if (phase.value === 'recording' && capture) {
      const t = recordedSeconds()
      capture.levels.push({ t, r: Math.round(rms * 1000) / 1000 })
      capture.trace.push({ t, f: Math.round(position() * 1e5) / 1e5 })
    }
  }, 50)

  meter = { ctx, timer }
}

function stopMeter() {
  if (!meter) return
  clearInterval(meter.timer)
  void meter.ctx.close()
  meter = null
}

/* ---------------- recording ---------------- */

function supportedType() {
  return ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/webm']
    .find(t => MediaRecorder.isTypeSupported(t)) ?? ''
}

async function startRecording() {
  // The guard that stops an impatient second click from spawning a second take.
  if (phase.value === 'recording' || phase.value === 'paused') return
  try {
    if (!micStream) await armMic(deviceId.value)
    if (!micStream) return

    screenStream = await navigator.mediaDevices.getDisplayMedia({
      video: { frameRate: { ideal: 30 }, width: { ideal: 1920 }, height: { ideal: 1080 } },
      audio: false,
      // Points the picker at this tab, so the capture is exactly the page.
      preferCurrentTab: true,
    } as DisplayMediaStreamOptions)

    warnAboutFraming(screenStream)

    const recorder = new MediaRecorder(
      new MediaStream([...screenStream.getVideoTracks(), ...micStream.getAudioTracks()]),
      { mimeType: supportedType(), videoBitsPerSecond: 8_000_000, audioBitsPerSecond: 192_000 },
    )
    const take: Capture = {
      recorder, chunks: [], cues: [], levels: [], trace: [], recordedMs: 0, segmentStart: 0,
    }
    // The handlers close over this take, so a recorder that is still flushing
    // writes into its own buffers however long it takes to get there.
    recorder.ondataavailable = e => { if (e.data.size) take.chunks.push(e.data) }
    recorder.onstop = () => void finish(take)
    capture = take

    // If the capture is stopped from Chrome's own bar, close the take cleanly
    // rather than losing it.
    screenStream.getVideoTracks()[0]?.addEventListener('ended', () => {
      if (phase.value === 'recording' || phase.value === 'paused') stopRecording()
    })

    recorder.start(1000)
    take.segmentStart = performance.now()
    phase.value = 'recording'
    channel?.postMessage({ type: 'recording-started', from: position() })
    report()
  } catch (err) {
    // A cancelled screen-share leaves the mic armed and nothing recording,
    // which is exactly where the operator should land: ready to try again.
    phase.value = micStream ? 'armed' : 'idle'
    screenStream?.getTracks().forEach(track => track.stop())
    screenStream = null
    fail(err)
    report()
  }
}

function pauseRecording() {
  if (phase.value !== 'recording' || !capture) return
  capture.recorder.pause()
  capture.recordedMs += performance.now() - capture.segmentStart
  phase.value = 'paused'
  // The page must stop moving too, or you resume somewhere you never read.
  scroll.running = false
  report()
}

function resumeRecording() {
  if (phase.value !== 'paused' || !capture) return
  capture.recorder.resume()
  capture.segmentStart = performance.now()
  phase.value = 'recording'
  report()
}

function stopRecording() {
  if (phase.value !== 'recording' && phase.value !== 'paused') return
  if (phase.value === 'recording' && capture) {
    capture.recordedMs += performance.now() - capture.segmentStart
  }
  capture?.recorder.stop()
  screenStream?.getTracks().forEach(t => t.stop())
  screenStream = null
  scroll.running = false
  phase.value = 'armed'
  report()
}

/**
 * Say so, before a word is read, if the capture is not going to deliver 1080p
 * 16:9.
 *
 * Tab capture hands over the tab as it actually is: the 1920×1080 asked for
 * above is a preference, not a promise, and a window that is not maximised
 * records at whatever shape it happens to have. The render scales to 1080 tall
 * and lets the width follow, so a narrow window becomes a narrow video — and
 * nothing about the recording looks wrong until ffmpeg reports the size of a
 * finished take. Half an hour of reading is far too late to find out.
 */
function warnAboutFraming(stream: MediaStream) {
  const settings = stream.getVideoTracks()[0]?.getSettings()
  const w = settings?.width
  const h = settings?.height
  if (!w || !h) return

  const ratio = w / h
  const complaints: string[] = []
  if (Math.abs(ratio - 16 / 9) > 0.04) {
    complaints.push(`la proporción es ${ratio.toFixed(2)}:1 y no 1.78:1 (16:9)`)
  }
  if (h < 1080) {
    complaints.push(`la altura es de ${h} px y no de 1080`)
  }
  if (!complaints.length) return

  channel?.postMessage({
    type: 'error',
    message: `Ojo: estás capturando ${w}×${h}, así que ${complaints.join(' y ')}. `
      + 'El video se entrega con esa forma. Maximiza la ventana de presentación '
      + '(o ajústala a 16:9) y vuelve a grabar.',
  })
}

async function finish(take: Capture) {
  const blob = new Blob(take.chunks, { type: take.chunks[0]?.type ?? 'video/webm' })

  // A WebM begins with the EBML header, 1A 45 DF A3. If this one does not, the
  // file is unreadable to ffprobe and to the montage, and the only thing worse
  // than losing the take is not being told until after the next half hour of
  // reading. It is uploaded anyway — the server salvages what it can, and that
  // beats discarding it here.
  const head = new Uint8Array(await blob.slice(0, 4).arrayBuffer())
  if (!(head[0] === 0x1a && head[1] === 0x45 && head[2] === 0xdf && head[3] === 0xa3)) {
    channel?.postMessage({
      type: 'error',
      message: 'Esta toma salió con la cabecera dañada. La subo igual y el '
        + 'servidor intentará rescatarla, pero revísala en Montaje antes de seguir.',
    })
  }

  channel?.postMessage({
    type: 'recording-stopped',
    blob,
    to: position(),
    duration: take.recordedMs / 1000,
    cues: [...take.cues],
    levels: [...take.levels],
    trace: [...take.trace],
  })
}

/** A quote fired: stamp it against recorded time, not wall time. */
function markCue(quote: string) {
  if (phase.value !== 'recording' || !capture) return
  const at = recordedSeconds()
  const existing = capture.cues.find(c => c.quote === quote)
  if (existing) existing.at = at
  else capture.cues.push({ quote, at })
}

// explainMediaError is shared with the console: one wording for one failure.
function fail(err: unknown) {
  channel?.postMessage({ type: 'error', message: explainMediaError(err) })
}

/* ---------------- channel ---------------- */

/**
 * Everything posted over the channel has to survive structured cloning, and a
 * Vue ref holding an array hands out a reactive Proxy — which cannot be cloned.
 * `postMessage` then throws on every single call, the presentation goes silent,
 * and the console sits there believing nothing is connected.
 *
 * So the state is rebuilt from primitives, and the device list is copied field
 * by field rather than passed through.
 */
function report() {
  if (!channel) return
  try {
    channel.postMessage({
      type: 'state',
      state: {
        f: position(),
        viewport: viewport(),
        anchor: ANCHOR,
        running: scroll.running,
        speed: scroll.speed,
        plate: plateVisible.value,
        phase: phase.value,
        muted: muted.value,
        recorded: recordedSeconds(),
        deviceId: deviceId.value,
        devices: devices.value.map(d => ({ id: d.id, label: d.label })),
        window: {
          w: window.innerWidth,
          h: window.innerHeight,
          dpr: window.devicePixelRatio || 1,
        },
      },
    })
  } catch (err) {
    // A silent channel is the worst possible failure here, so say so loudly.
    console.error('el presentador no pudo emitir su estado:', err)
  }
}

// Registered synchronously: a lifecycle hook set up after an await has no
// component instance to attach to, and Vue fails app initialisation for it.
let beat: ReturnType<typeof setInterval> | null = null

onUnmounted(() => {
  if (beat) clearInterval(beat)
  window.removeEventListener('resize', measure)
  stopRecording()
  releaseMic()
  channel?.close()
})

onMounted(async () => {
  channel = new BroadcastChannel(CHANNEL)
  channel.onmessage = (e) => {
    const m = e.data
    if (!m || m.to !== 'presenter') return
    switch (m.command) {
      case 'run': scroll.running = m.value; break
      case 'speed': scroll.speed = m.value; break
      case 'seek': goTo(m.value); paint(); break
      case 'nudge': scroll.y += m.value; paint(); break
      case 'figure': figureId.value = m.value ?? ''; break
      case 'plate': plateVisible.value = m.value; break
      case 'arrow': arrowAt.value = m.value; break
      case 'arm': void armMic(m.value); break
      case 'devices': void listDevices().then(report); break
      case 'mute': muted.value = !!m.value; applyMute(); break
      case 'record': void startRecording(); break
      case 'pause': pauseRecording(); break
      case 'resume': resumeRecording(); break
      case 'stop': stopRecording(); break
      case 'mark': markCue(m.value); break
    }
    report()
  }

  window.addEventListener('resize', measure)
  beat = setInterval(report, 100)
  requestAnimationFrame(loop)

  if (!studio.reading.value) await studio.load()
  await loadPages()
  await nextTick()
  measure()
})
</script>

<template>
  <div class="presenter">
    <div ref="strip" class="strip">
      <img v-for="p in PAGES" :key="p.archivo" :src="`/assets/paginas/${p.archivo}`" :alt="p.archivo"
           :width="p.ancho" :height="p.alto">
    </div>

    <!-- The plate, full screen with its caption and nothing else (2.d). -->
    <div v-show="plateVisible" class="plate">
      <img :src="PLATE" alt="Lámina">
    </div>

    <!-- The only other non-page pixel the rules allow. -->
    <div v-if="arrowAt !== null" class="arrow" :style="{ top: `calc(${arrowAt * 100}% - 18px)` }">➤</div>
  </div>
</template>

<style scoped>
.presenter {
  position: fixed;
  inset: 0;
  background: #fff;
  overflow: hidden;
  cursor: none;
}
.strip { width: 100vw; will-change: transform; }
.strip img { display: block; width: 100%; height: auto; }

.plate {
  position: fixed;
  inset: 0;
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
}
.plate img { max-width: 96vw; max-height: 96vh; object-fit: contain; }

.arrow {
  position: fixed;
  left: 2vw;
  font-size: 2.4vw;
  line-height: 1;
  color: #c0392b;
  pointer-events: none;
  text-shadow: 0 0 6px #fff, 0 0 6px #fff;
}
</style>
