/**
 * The transport: what the console sends to the presentation window, and the
 * state it mirrors back.
 *
 * Three rules from the way the reading actually works drive the design:
 *
 *  - Quotes are sequential, and the Socrates/Glaucón exchanges are single
 *    dialogues. One keypress fires a whole group back to back, and while a
 *    chain is playing no other chain is allowed, so quotes can never overlap.
 *  - The page never moves on its own. A quote used to pull the scroll along
 *    with it, which suited one long take over a whole dialogue; with a take
 *    per paragraph it slid the sentence being spoken out from under the
 *    reader. `espacio` still runs the scroll by hand.
 *  - The presentation window owns the recording clock. It stamps cues, levels
 *    and the scroll trace against recorded time, which is the only timeline
 *    that survives a pause. The console never times anything itself.
 */

export type Phase = 'idle' | 'armed' | 'recording' | 'paused'

export interface AudioDevice { id: string; label: string }

export interface PresenterState {
  f: number
  /** How much of the strip one screen shows, as a fraction. */
  viewport: number
  /** Where down the screen the reader's eye sits, 0..1. */
  anchor: number
  running: boolean
  speed: number
  plate: boolean
  phase: Phase
  muted: boolean
  recorded: number
  deviceId: string | null
  devices: AudioDevice[]
  window: { w: number; h: number; dpr: number } | null
}

const CHANNEL = 'estoc-studio'

/**
 * The channel, the preloaded clips and the chain timer are module state on
 * purpose: they are one connection to one presentation window, shared by every
 * component that asks for the transport.
 *
 * Holding them inside the composable gave each caller its own closure — the
 * console opened a channel, the record panel got a null one, and every button
 * it sent through quietly did nothing.
 */
let channel: BroadcastChannel | null = null
let chainTimer: ReturnType<typeof setTimeout> | null = null

/**
 * The clips, keyed by the quote they belong to and built on demand.
 *
 * They used to be an array built once inside connect(), indexed positionally
 * against studio.quotes. The console calls connect() before the reading has
 * finished loading, so quotes was still empty and the array was built with
 * nothing in it — and playGroup, finding no audio at an index, skipped every
 * clip in silence. Pressing N walked the whole group, fired no sound, and left
 * the counter at 0/19 with nothing to say it had failed.
 *
 * Keyed and lazy, there is no moment at which the map can be wrong: a clip is
 * created the first time it is asked for, whenever the reading arrived.
 */
const audios = new Map<string, HTMLAudioElement>()

function clipAudio(id: string) {
  let audio = audios.get(id)
  if (!audio) {
    audio = new Audio(`/assets/audio/tts/${id}.mp3`)
    audio.preload = 'auto'
    audios.set(id, audio)
  }
  return audio
}

export const useTransport = () => {
  const studio = useStudio()

  const state = useState<PresenterState>('presenter', () => ({
    f: 0, viewport: 0, anchor: 0.4, running: false, speed: 18, plate: false,
    phase: 'idle', muted: false, recorded: 0,
    deviceId: null, devices: [], window: null,
  }))

  const connected = useState('presenter-connected', () => false)
  const nextQuote = useState('next-quote', () => 0)
  const playing = useState('chain-playing', () => false)
  const playingQuote = useState('chain-quote', () => -1)
  const level = useState('mic-level', () => ({ rms: 0, peak: 0 }))
  const notice = useState<string | null>('transport-notice', () => null)
  const uploading = useState('uploading', () => false)
  const takeFrom = useState('take-from', () => 0)

  /**
   * Which paragraph the next take reads.
   *
   * Explicit, not worked out from the scroll. At the top of the strip the page
   * cannot scroll far enough to put the first paragraphs under the eye line,
   * so the reported position clamps and lands on the third — which made the
   * first two impossible to select at all.
   */
  const paragraph = useState('paragraph', () => 0)

  /** Whether the next take shows the lámina rather than the page. */
  const plate = useState('take-is-plate', () => false)
  /** Which figure, when the take shows one. Empty for lectura 07's lámina. */
  const figure = useState('take-figure', () => '')

  function connect() {
    if (channel || typeof window === 'undefined') return
    channel = new BroadcastChannel(CHANNEL)
    channel.onmessage = (e) => {
      const m = e.data
      if (!m) return
      switch (m.type) {
        case 'state':
          Object.assign(state.value, m.state)
          connected.value = true
          break
        case 'level':
          level.value = { rms: m.rms, peak: m.peak }
          break
        case 'recording-started':
          takeFrom.value = m.from ?? state.value.f
          // The scroll is deliberately *not* started here. A take is one
          // screenful read against a still page: matching your voice to a
          // moving strip is the thing that made this hard, and the montage
          // concatenates the takes anyway. Press space if you want it moving.
          break
        case 'recording-stopped':
          void upload(m)
          break
        case 'error':
          notice.value = m.message
          break
      }
    }
    // Warm the clips so a cue fires without a fetch in between. This waits for
    // the reading rather than assuming it has arrived, which is the mistake
    // that used to leave the console mute.
    watch(studio.quotes, (quotes) => {
      for (const q of quotes) clipAudio(q.clip)
    }, { immediate: true })
  }

  function disconnect() {
    channel?.close()
    channel = null
  }

  function send(command: string, value?: unknown) {
    channel?.postMessage({ to: 'presenter', command, value })
  }

  function openPresenter() {
    window.open('/presentador', 'presentador', 'width=1024,height=576,left=0,top=140')
  }

  /* ---------- microphone ---------- */

  const armMic = (id?: string | null) => send('arm', id ?? state.value.deviceId)
  const refreshDevices = () => send('devices')
  const toggleMute = () => send('mute', !state.value.muted)

  /* ---------- quotes ---------- */

  const indexOf = (id: string) => studio.quotes.value.findIndex(q => q.clip === id)

  /**
   * Sound one quote, and only that one.
   *
   * It used to fire the whole exchange: one keypress played Sócrates, then
   * Glaucón, then Sócrates again, because a take was one long pass over the
   * whole dialogue. A take is now one paragraph and a paragraph is one turn,
   * so chaining plays four clips into a recording that wanted one — and left
   * no way back to the turn you meant, since the chain had already walked
   * past it.
   *
   * What decides which clip sounds is the paragraph you are on, so pressing
   * it again simply plays it again.
   */
  function playQuote(clip: string) {
    if (playing.value) return
    const i = indexOf(clip)
    if (i < 0) return

    const audio = clipAudio(clip)
    playing.value = true
    playingQuote.value = i
    audio.onended = () => {
      playing.value = false
      playingQuote.value = -1
    }
    audio.currentTime = 0
    audio.play().catch((err) => {
      notice.value = `No pude sonar «${clip}»: ${err?.message ?? err}`
      stopChain()
    })
    // The presenter stamps it; only it knows how much has actually recorded.
    send('mark', clip)
    if (i >= nextQuote.value) nextQuote.value = i + 1
  }

  /** The quote of the paragraph being recorded — what N sounds. */
  const currentQuote = computed(() => studio.quoteOfParagraph(paragraph.value))

  /**
   * Nothing happens on a paragraph with no quote in it.
   *
   * Falling back to "the next one still unplayed" would be worse than a dead
   * key: a stray press while reading narration would stamp a cue into that
   * take, and the render would drop a character's line into the middle of a
   * paragraph they have nothing to do with.
   */
  function playNext() {
    const here = currentQuote.value
    if (here) playQuote(here.clip)
  }

  /** Play this paragraph's quote again — the way back to the turn you fluffed. */
  function replayPrevious() {
    const here = currentQuote.value
    if (here) playQuote(here.clip)
  }

  function stopChain() {
    if (chainTimer) clearTimeout(chainTimer)
    audios.forEach((a) => { a.pause(); a.onended = null })
    playing.value = false
    playingQuote.value = -1
    send('release')
  }

  /* ---------- recording ---------- */

  const startTake = () => {
    if (state.value.phase === 'recording' || state.value.phase === 'paused') return
    notice.value = null
    send('record')
  }
  const pauseTake = () => send('pause')
  const resumeTake = () => send('resume')
  const stopTake = () => { stopChain(); send('stop') }

  interface StoppedPayload {
    blob: Blob
    to: number
    duration: number
    cues: { quote: string; at: number }[]
    levels: { t: number; r: number }[]
    trace: { t: number; f: number }[]
  }

  async function upload(payload: StoppedPayload) {
    if (!payload.blob || payload.duration < 0.5) {
      notice.value = 'La toma duró menos de un segundo; no la guardé.'
      return
    }
    const label = plate.value ? studio.figureName(figure.value) : labelOf(paragraph.value)
    const form = new FormData()
    form.append('meta', JSON.stringify({
      label,
      paragraph: paragraph.value,
      plate: plate.value,
      figure: plate.value ? figure.value : '',
      from: takeFrom.value,
      to: payload.to,
      cues: payload.cues,
      levels: payload.levels,
      trace: payload.trace,
    }))
    form.append('file', payload.blob, 'take.webm')

    uploading.value = true
    notice.value = `Guardando «${label}»…`
    try {
      await $fetch('/api/takes', { method: 'POST', body: form })
      await studio.refreshSession()
      notice.value = `Toma guardada: ${label} · ${clock(payload.duration)}`
    } catch (e: any) {
      notice.value = `No pude guardar la toma: ${e?.message ?? e}`
    } finally {
      uploading.value = false
    }
  }

  /**
   * Takes are named after the paragraph they read, so the montage list can be
   * read down like the passage itself. Naming them after the section gave
   * seventy-nine takes eight distinct names between them.
   */
  function labelOf(index: number) {
    const p = studio.paragraphs.value[index]
    if (!p) return 'toma'
    const opening = p.text.slice(0, 46).trim()
    return `¶${index + 1} · ${opening}${p.text.length > 46 ? '…' : ''}`
  }

  /* ---------- scroll ---------- */

  const setSpeed = (v: number) => send('speed', v)
  const toggleRun = () => send('run', !state.value.running)
  const seek = (f: number) => send('seek', f)
  const nudge = (px: number) => send('nudge', px)
  /**
   * Show or hide the figure with its caption. Without one chosen from the
   * list, the first the current paragraph names with its caption.
   */
  function togglePlate() {
    if (!state.value.plate && !plate.value) {
      const here = studio.figureEvents.value
        .find(e => e.paragraph === paragraph.value && e.modo === 'pie')
      if (here) figure.value = here.figura
    }
    send('figure', figure.value)
    send('plate', !state.value.plate)
  }
  const putArrow = (at: number | null) => send('arrow', at)

  /** Jump to a section, which is how you go back and re-read one. */
  function goToSection(index: number) {
    const s = studio.sections.value[index]
    if (s) seek(s.f)
  }

  const recording = computed(() => state.value.phase === 'recording')
  const paused = computed(() => state.value.phase === 'paused')
  const armed = computed(() => state.value.phase !== 'idle')
  const busy = computed(() => recording.value || paused.value)

  const dbfs = computed(() =>
    level.value.rms > 0 ? 20 * Math.log10(level.value.rms) : -99,
  )

  const deviceLabel = computed(() =>
    state.value.devices.find(d => d.id === state.value.deviceId)?.label ?? null,
  )

  return {
    state, connected, notice, level, dbfs, uploading, paragraph, plate, figure,
    nextQuote, playing, playingQuote,
    recording, paused, armed, busy, deviceLabel,
    connect, disconnect, openPresenter,
    armMic, refreshDevices, toggleMute,
    playNext, replayPrevious, playQuote, currentQuote, stopChain,
    startTake, pauseTake, resumeTake, stopTake,
    setSpeed, toggleRun, seek, nudge, togglePlate, putArrow, goToSection,
  }
}
