/**
 * The studio's data, shared across every surface.
 *
 * The reading is immutable once loaded, so it is fetched once and kept. The
 * session changes constantly — takes arrive, cuts are made — so every mutation
 * returns the whole session from the server and replaces the local copy rather
 * than patching it, which keeps the client from ever inventing state the
 * backend does not have.
 */

/** A character's colour, keyed by the name the reading uses. */
const VOICE_COLOURS: Record<string, string> = {
  'Sócrates': 'var(--color-voice-socrates)',
  'Glaucón': 'var(--color-voice-glaucon)',
  'Russell': 'var(--color-voice-russell)',
  'Whitehead': 'var(--color-voice-whitehead)',
  'Wikipedia': 'var(--color-voice-wikipedia)',
  'Zenón': 'var(--color-voice-zenon)',
}

export interface Quote {
  clip: string
  personaje: string
  linea: number
  lineaFin: number | null
  f: number
  fFin: number | null
  dur: number
  texto: string
}

export interface Group { clips: string[]; pausas: number[] }
export interface Section { titulo: string; linea: number; f: number }
export interface Line {
  n: number
  f: number
  t: string
  /** lector.py: texto, titulo or pie. A printed caption is read with its figure, not here. */
  tipo?: string
}

/** A stretch of the passage read as one take. */
export interface Paragraph {
  index: number
  /** Indices into `lines`, inclusive. */
  from: number
  to: number
  f: number
  fEnd: number
  text: string
}

export interface Reading {
  paginas: number
  altoTira: number
  clips: Quote[]
  grupos: Group[]
  secciones: Section[]
  lamina: { linea: number; f: number; figura?: string } | null
  /** Written by preparar/lector.py; absent in lectura 07's marcas.json. */
  lectura?: { numero: number; titulo: string; video: string }
  /** Every figure the text names, and each mention of it (lector.py). */
  figuras?: { id: string; nombre: string; pie: string }[]
  eventos?: { figura: string; modo: 'pie' | 'breve'; n: number; f: number }[]
  pantalla?: { segundosFigura?: number; anchoFigura?: number }
  /** Stretches where a figure stays fixed on the right (2.f), passage lines 1-based. */
  persistencias?: { figura: string; nDesde: number; nHasta: number }[]
  lineas: Line[]
}

export interface Cue { quote: string; at: number }
export interface Cut { from: number; to: number }

/** One microphone reading, captured while recording so the waveform can be
 *  drawn without decoding the video. */
export interface Level { t: number; r: number }

/** Where the strip was at an instant of the take. */
export interface TracePoint { t: number; f: number }

export interface Take {
  id: number
  file: string
  label: string
  duration: number
  bytes: number
  from: number
  to: number
  paragraph: number
  plate: boolean
  /** Which figure a plate take shows; empty for lectura 07's lámina. */
  figure?: string
  cues: Cue[] | null
  cuts: Cut[] | null
  levels: Level[] | null
  trace: TracePoint[] | null
  recordedAt: string
}

export interface Session {
  takes: Take[] | null
  timeline: { entries: { takeId: number; muted: boolean }[] | null }
  updatedAt: string
}

export const useStudio = () => {
  const reading = useState<Reading | null>('reading', () => null)
  const session = useState<Session | null>('session', () => null)
  const error = useState<string | null>('studio-error', () => null)

  async function load() {
    try {
      const [r, s] = await Promise.all([
        $fetch<Reading>('/api/reading'),
        $fetch<Session>('/api/session'),
      ])
      reading.value = r
      session.value = s
      error.value = null
    } catch (e: any) {
      // 409: the studio has no reading open yet (a fresh clone). Making one
      // is the first thing to do, so go there.
      if (e?.statusCode === 409 || e?.status === 409) {
        await navigateTo('/lecturas')
        return
      }
      error.value = e?.message ?? 'no pude hablar con el servidor'
    }
  }

  async function refreshSession() {
    session.value = await $fetch<Session>('/api/session')
  }

  async function patchTake(id: number, body: Partial<{ label: string; cuts: Cut[]; cues: Cue[] }>) {
    session.value = await $fetch<Session>(`/api/takes/${id}`, { method: 'PATCH', body })
  }

  async function deleteTake(id: number) {
    session.value = await $fetch<Session>(`/api/takes/${id}`, { method: 'DELETE' })
  }

  const takes = computed(() => session.value?.takes ?? [])
  const quotes = computed(() => reading.value?.clips ?? [])
  const groups = computed(() => reading.value?.grupos ?? [])
  const sections = computed(() => reading.value?.secciones ?? [])
  const lines = computed(() => reading.value?.lineas ?? [])

  /**
   * The passage cut into paragraphs, which is the unit a take is recorded in.
   *
   * pdftotext gives no structure, only leading spaces, and they are not clean:
   * a paragraph opens at an indent of four, five or six depending on where the
   * line fell, while a continuation of a quoted speech sits at seven. So the
   * rule is an indent in 3..6, or any line after a blank one. Six and seven are
   * one space apart and the extraction does occasionally confuse them — but the
   * cost of that is a paragraph split in two, which is a slightly shorter take
   * and nothing worse.
   */
  const paragraphs = computed<Paragraph[]>(() => {
    const L = lines.value
    if (!L.length) return []

    const starts: number[] = []
    let previousBlank = true
    for (let i = 0; i < L.length; i++) {
      const text = L[i]!.t
      const blank = !text.trim()
      if (!blank) {
        const indent = text.length - text.trimStart().length
        if (previousBlank || (indent >= 3 && indent <= 6)) starts.push(i)
      }
      previousBlank = blank
    }
    if (!starts.length) return []

    return starts.map((from, index) => {
      // Up to the line before the next paragraph opens, ignoring trailing blanks.
      let to = (starts[index + 1] ?? L.length) - 1
      while (to > from && !L[to]!.t.trim()) to--
      return {
        index,
        from,
        to,
        f: L[from]!.f,
        fEnd: L[to]!.f,
        text: L.slice(from, to + 1).filter(l => l.tipo !== 'pie').map(l => l.t.trim()).join(' ').trim(),
      }
    })
  })

  /**
   * Paragraphs that are the tail of a quote, and so are nobody's take.
   *
   * A quote is one clip of audio, but the extraction splits its lines like any
   * other text: `07-russell` alone comes out as six paragraphs. Only the first
   * of them is a thing to record — the rest are the same clip continuing, and
   * counting them as unrecorded says thirty paragraphs are missing when twelve
   * of those do not exist.
   *
   * The quote's own line span decides this, not the indentation that split
   * them, so there is nothing to guess.
   */
  const quoteTails = computed(() => {
    const tails = new Set<number>()
    const P = paragraphs.value
    if (!P.length) return tails

    const paragraphOfLine = (n: number) => {
      const i = lines.value.findIndex(l => l.n === n)
      if (i < 0) return -1
      return P.findIndex(p => i >= p.from && i <= p.to)
    }

    for (const q of quotes.value) {
      const first = paragraphOfLine(q.linea)
      const last = paragraphOfLine(q.lineaFin ?? q.linea)
      if (first < 0 || last < 0) continue
      for (let i = first + 1; i <= last; i++) tails.add(i)
    }
    return tails
  })

  /** The quote a paragraph contains, if any. One paragraph, one turn. */
  function quoteOfParagraph(index: number) {
    const p = paragraphs.value[index]
    if (!p) return null
    const first = lines.value[p.from]?.n ?? 0
    const last = lines.value[p.to]?.n ?? 0
    return quotes.value.find(q => q.linea >= first && q.linea <= last) ?? null
  }

  /**
   * Quotes that have actually been fired into a take.
   *
   * The old counter tracked a pointer through the list, which only meant
   * anything while the whole reading was one pass. Recording a paragraph at a
   * time, in whatever order, the honest question is which clips are stamped
   * on a take that is going into the video.
   */
  const firedQuotes = computed(() => {
    const fired = new Set<string>()
    for (const t of takes.value) {
      for (const c of t.cues ?? []) fired.add(c.quote)
    }
    return fired
  })

  /** The paragraph holding passage line n (1-based), or -1. */
  function paragraphOfLine(n: number) {
    return paragraphs.value.findIndex(
      p => n >= (lines.value[p.from]?.n ?? 0) && n <= (lines.value[p.to]?.n ?? 0),
    )
  }

  /** The printed name of a figure. The empty id is lectura 07's lámina. */
  function figureName(id: string) {
    if (!id) return 'Lámina F'
    return reading.value?.figuras?.find(f => f.id === id)?.nombre ?? id
  }

  /**
   * Every time the text names a figure, on the paragraph whose sentence does.
   *
   * Lineamiento 3.2.3: the first mention shows the figure whole with its
   * caption, and the caption is read (2.d) — a take of its own, beside the
   * paragraph's reading. Every later mention shows it again without the
   * caption for two or three seconds (2.e), which the render does on its own.
   * After either, the page comes back with a return arrow.
   *
   * Lectura 07 has no events, only its lámina, which is one first mention.
   */
  const figureEvents = computed(() => {
    const r = reading.value
    if (!r) return []
    const events = r.eventos?.length
      ? r.eventos
      : r.lamina ? [{ figura: '', modo: 'pie' as const, n: r.lamina.linea, f: r.lamina.f }] : []
    return events.map(e => ({
      ...e, nombre: figureName(e.figura), paragraph: paragraphOfLine(e.n),
    }))
  })

  /** The newest kept take showing a figure with its caption, if recorded. */
  function figureTake(id: string) {
    const all = takes.value
      .filter(t => t.plate && (t.figure ?? '') === id)
      .sort((a, b) => a.id - b.id)
    const newest = all[all.length - 1]
    if (!newest) return null
    const removed = (newest.cuts ?? []).reduce((s, c) => s + (c.to - c.from), 0)
    return newest.duration - removed > 0 ? newest : null
  }

  /**
   * The speaker's name printed before a turn, which the reader says aloud.
   *
   * The page reads «Sócrates. Y ahora, dije…». Those first words are printed,
   * so they are read like everything else, but in the narrator's voice: it is
   * Wilczek naming who speaks, not the character. The synthesised clip starts
   * after them, which is why the TTS text leaves the label out.
   */
  function speakerLabel(index: number) {
    const quote = quoteOfParagraph(index)
    const p = paragraphs.value[index]
    if (!quote || !p) return null
    const opening = lines.value[p.from]?.t.trim() ?? ''
    return opening.startsWith(`${quote.personaje}.`) ? quote.personaje : null
  }

  /**
   * Whether the reader spoke before firing the clip.
   *
   * The microphone hears nothing while a quote plays, so any sound before the
   * cue is the reader, and on a turn that is the name. Half a second of quiet
   * where the name should be means the take is missing it — which is the one
   * thing the envelope can prove without anybody listening to twenty minutes
   * of audio again.
   */
  function saysTheName(take: Take, clip: string) {
    const cue = (take.cues ?? []).find(c => c.quote === clip)
    if (!cue) return false
    const spoken = (take.levels ?? []).filter(l => l.t < cue.at && l.r > 0.02).length
    return spoken * 0.05 >= 0.3
  }

  /** Paragraphs whose turn is missing its spoken name, or its clip. */
  const turnsToRedo = computed(() =>
    paragraphs.value.filter((p) => {
      const label = speakerLabel(p.index)
      if (!label) return false
      const take = takeOfParagraph(p.index)
      const quote = quoteOfParagraph(p.index)
      if (!take || !quote) return true
      return !saysTheName(take, quote.clip)
    }),
  )

  /** Every take of a paragraph, oldest first. */
  function takesOfParagraph(index: number) {
    return takes.value
      .filter(t => t.paragraph === index && !t.plate)
      .sort((a, b) => a.id - b.id)
  }

  /**
   * The take of a paragraph that the render will actually use: the newest one,
   * and only if its cuts left something. Reading a paragraph again supersedes
   * the earlier attempt rather than adding a second copy of the sentence, and
   * a take cut down to nothing is no take at all.
   */
  function takeOfParagraph(index: number) {
    const all = takesOfParagraph(index)
    const newest = all[all.length - 1]
    if (!newest) return null
    const removed = (newest.cuts ?? []).reduce((s, c) => s + (c.to - c.from), 0)
    return newest.duration - removed > 0 ? newest : null
  }

  /** The paragraph the reader's eye is in. */
  function paragraphAt(f: number): Paragraph | null {
    const P = paragraphs.value
    if (!P.length) return null
    let lo = 0
    let hi = P.length - 1
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2)
      if (P[mid]!.f <= f) lo = mid
      else hi = mid - 1
    }
    return P[lo]!
  }

  /**
   * The takes the video is made of: the newest of each paragraph (and of each
   * plate), as the render picks them. Re-reading a paragraph leaves the old
   * take on disk, and counting it too made a 18-minute reading show 28.
   */
  const inUse = computed(() => {
    const newest = new Map<string, Take>()
    for (const t of takes.value) {
      const key = `${t.paragraph}|${t.plate ? 1 : 0}|${(t as any).figure ?? ''}`
      const seen = newest.get(key)
      if (!seen || seen.id < t.id) newest.set(key, t)
    }
    return [...newest.values()]
  })

  /** Total runtime once every cut is applied — the number the rules care about. */
  const keptSeconds = computed(() =>
    inUse.value.reduce((total, t) => {
      const removed = (t.cuts ?? []).reduce((s, c) => s + (c.to - c.from), 0)
      return total + Math.max(0, t.duration - removed)
    }, 0),
  )

  const overBudget = computed(() => keptSeconds.value > 25 * 60)

  function colourOf(character: string) {
    return VOICE_COLOURS[character] ?? 'var(--color-score-dim)'
  }

  /**
   * The line of the passage at a strip position. Cues pin time to position at
   * nineteen points; the trace pins it everywhere, which is what lets a cut be
   * named after a line instead of a timestamp.
   */
  function lineAt(f: number): Line | null {
    const L = lines.value
    if (!L.length) return null
    let lo = 0
    let hi = L.length - 1
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2)
      if (L[mid]!.f <= f) lo = mid
      else hi = mid - 1
    }
    return L[lo]!
  }

  /** Where a take was in the passage at a given second of its recording. */
  function positionAt(take: Take, seconds: number): number | null {
    const trace = take.trace
    if (!trace?.length) return null
    if (seconds <= trace[0]!.t) return trace[0]!.f
    const last = trace[trace.length - 1]!
    if (seconds >= last.t) return last.f
    let lo = 0
    let hi = trace.length - 1
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (trace[mid]!.t < seconds) lo = mid + 1
      else hi = mid
    }
    const b = trace[lo]!
    const a = trace[lo - 1]!
    const span = b.t - a.t
    return span <= 0 ? b.f : a.f + ((b.f - a.f) * (seconds - a.t)) / span
  }

  function quoteById(id: string) {
    return quotes.value.find(q => q.clip === id)
  }

  /** Which group a quote belongs to, so a whole exchange fires as one. */
  function groupOfQuote(id: string) {
    return groups.value.findIndex(g => g.clips.includes(id))
  }

  function groupDuration(index: number) {
    const g = groups.value[index]
    if (!g) return 0
    const clips = g.clips.reduce((s, id) => s + (quoteById(id)?.dur ?? 0), 0)
    return clips + g.pausas.reduce((s, p) => s + p, 0)
  }

  return {
    reading, session, error, load, refreshSession, patchTake, deleteTake,
    takes, quotes, groups, sections, lines, paragraphs, paragraphAt, takeOfParagraph, takesOfParagraph, quoteTails,
    quoteOfParagraph, firedQuotes, figureEvents, figureTake, figureName,
    speakerLabel, saysTheName, turnsToRedo,
    keptSeconds, overBudget,
    colourOf, quoteById, groupOfQuote, groupDuration, lineAt, positionAt,
  }
}

/** mm:ss, the only time format the studio shows. */
export function clock(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) seconds = 0
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
