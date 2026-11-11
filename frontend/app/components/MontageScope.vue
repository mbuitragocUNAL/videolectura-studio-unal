<script setup lang="ts">
import type { Cut } from '~/composables/useStudio'
import type { Proposal, Span } from '~/composables/useTakeEditor'

/**
 * The montage scope: the same score pulled back far enough to see the takes as
 * material rather than as position.
 *
 * Three things make a cut decidable here. You can **hear** it — the player
 * skips what is cut, so playback is the montage and not the raw take. You can
 * **see** it — the window zooms to a quarter second, which is where a stumble
 * actually lives. And you can **read** it — the scroll trace names the line of
 * the passage under the playhead, so a cut is "from line 67 to 71" rather than
 * a pair of timestamps nobody can check.
 *
 * A cut that would swallow a quote is refused, because that quote has nowhere
 * else to live — unless the same quote sounds again later, which is what
 * happens when a passage was read twice.
 */
const studio = useStudio()
const { takes } = studio

// Shared, so clicking a take on the score jumps straight into editing it.
const selectedId = useState<number | null>('montage-take', () => null)
const selected = computed(() => takes.value.find(t => t.id === selectedId.value) ?? null)

const montage = useMontage()

// Prefer a take the video actually uses: landing on one that has been
// superseded or cut away to nothing is a puzzling thing to open on.
watchEffect(() => {
  if (takes.value.some(t => t.id === selectedId.value) || !takes.value.length) return
  selectedId.value = montage.ordered.value[0]?.id ?? takes.value[0]!.id
})

const editor = useTakeEditor(selected)
const { view, selection, endangered, proposals, duration } = editor

const busy = ref(false)

/* ---------- cuts ---------- */

/**
 * While an edge is being dragged the cut moves locally: a PATCH per mouse
 * event would be a hundred round trips to say one thing. The draft is
 * committed when the mouse comes up.
 */
const draft = ref<Cut[] | null>(null)
const cuts = computed<Cut[]>(() => draft.value ?? selected.value?.cuts ?? [])

async function writeCuts(next: Cut[]) {
  const t = selected.value
  if (!t) return
  busy.value = true
  try {
    await studio.patchTake(t.id, { cuts: mergeCuts(next) })
    draft.value = null
  } finally { busy.value = false }
}

function cutSpan(span: Span) {
  void writeCuts([...cuts.value, { from: span.from, to: span.to }])
  selection.value = null
}

/** Assigning a destructured ref from the template is a trap worth not laying. */
function onSelect(span: Span | null) {
  selection.value = span
}

function applySelection() {
  const sel = selection.value
  if (!sel || endangered.value.length) return
  cutSpan(sel)
}

function onRecut(index: number, span: Span) {
  const next = [...cuts.value]
  next[index] = { from: span.from, to: span.to }
  draft.value = next
}

function onUncut(index: number) {
  void writeCuts(cuts.value.filter((_, i) => i !== index))
}

function undoLast() {
  if (!cuts.value.length) return
  void writeCuts(cuts.value.slice(0, -1))
}

// Committing on mouseup rather than on every move is the whole point of the
// draft, so the listener lives here and not in the track.
useEventListener(window, 'mouseup', () => {
  if (draft.value) void writeCuts(draft.value)
})

/* ---------- the quotes, heard where they will land ---------- */

/*
The take's microphone track is silent where a quote plays: the clip sounds
through the headphones and ffmpeg mixes it in at render time. Previewing the
take therefore plays a hole, which is no use at all when what you are judging
is the gap between saying the character's name and the quote starting.

So the preview sounds the clips itself, at the instant each cue sits at. What
you hear is then what the render will produce.
*/
const clips = new Map<string, HTMLAudioElement>()

function clipAudio(id: string) {
  let audio = clips.get(id)
  if (!audio) {
    audio = new Audio(`/assets/audio/tts/${id}.mp3`)
    audio.preload = 'auto'
    clips.set(id, audio)
  }
  return audio
}

/** Cues already sounded in this pass, so a clip fires once per playthrough. */
let sounded = new Set<string>()

function hushClips() {
  clips.forEach((a) => { a.pause(); a.currentTime = 0 })
  sounded = new Set()
}

/** Sound any cue the playhead has just passed, and arm the ones ahead of it. */
function soundCuesAt(seconds: number) {
  for (const cue of selected.value?.cues ?? []) {
    const started = seconds >= cue.at && seconds < cue.at + 0.35
    if (started && !sounded.has(cue.quote)) {
      sounded.add(cue.quote)
      const audio = clipAudio(cue.quote)
      audio.currentTime = 0
      void audio.play().catch(() => {})
    }
    if (seconds < cue.at) sounded.delete(cue.quote)
  }
}

onUnmounted(hushClips)

/* ---------- the player ---------- */

const player = useTemplateRef<HTMLVideoElement>('player')
const time = ref(0)
const playing = ref(false)
const mediaUrl = computed(() => (selected.value ? `/api/takes/${selected.value.id}/media` : ''))

watch(selectedId, () => {
  hushClips()
  time.value = 0
  playing.value = false
  selection.value = null
  draft.value = null
})

/**
 * Playback is of the montage, not the recording: when the head walks into a
 * cut it steps over it. Hearing the splice is the only way to know whether it
 * lands, and the render is too slow a way to ask.
 */
let frame = 0
function follow() {
  const v = player.value
  if (v && !v.paused) {
    const jump = editor.skipCuts(v.currentTime)
    if (jump !== null && jump < duration.value) v.currentTime = jump
    time.value = v.currentTime
    soundCuesAt(time.value)
    editor.reveal(time.value)
  }
  frame = requestAnimationFrame(follow)
}
onMounted(() => { frame = requestAnimationFrame(follow) })
onUnmounted(() => cancelAnimationFrame(frame))

function seek(seconds: number) {
  time.value = seconds
  hushClips()
  const v = player.value
  if (v) v.currentTime = seconds
}

function togglePlay() {
  const v = player.value
  if (!v) return
  if (v.paused) { void v.play(); playing.value = true }
  else { v.pause(); hushClips(); playing.value = false }
}

function nudgeHead(seconds: number) {
  seek(Math.min(duration.value, Math.max(0, time.value + seconds)))
}

/* ---------- where we are in the passage ---------- */

/** The line of the book under the playhead, and the two around it. */
const context = computed(() => {
  const t = selected.value
  if (!t) return null
  const f = studio.positionAt(t, time.value)
  if (f === null) return null
  const here = studio.lineAt(f)
  if (!here) return null
  const all = studio.lines.value
  const i = all.findIndex(l => l.n === here.n)
  return {
    previous: all[i - 1]?.t ?? '',
    here: here,
    next: all[i + 1]?.t ?? '',
  }
})

function linesOf(span: Span) {
  const t = selected.value
  if (!t) return null
  const a = studio.positionAt(t, span.from)
  const b = studio.positionAt(t, span.to)
  if (a === null || b === null) return null
  const first = studio.lineAt(a)
  const last = studio.lineAt(b)
  return first && last ? { first, last } : null
}

const selectionLines = computed(() => (selection.value ? linesOf(selection.value) : null))

/* ---------- proposals ---------- */

const focus = ref<Proposal | null>(null)

watch(proposals, (list) => {
  if (focus.value && !list.includes(focus.value)) focus.value = null
})

function look(p: Proposal) {
  focus.value = p
  editor.zoomToSpan(p)
  seek(Math.max(0, p.from - 0.5))
}

function accept(p: Proposal) {
  cutSpan(p)
  focus.value = null
}

function acceptAll(kind: Proposal['kind']) {
  const batch = proposals.value.filter(p => p.kind === kind)
  if (!batch.length) return
  void writeCuts([...cuts.value, ...batch.map(p => ({ from: p.from, to: p.to }))])
}

/* ---------- keyboard ---------- */
/* The montage owns the keys while it is on screen; index.vue stands back. */

function onKey(e: KeyboardEvent) {
  if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
  const fine = e.shiftKey ? 0.1 : 1
  switch (e.key) {
    case ' ': e.preventDefault(); togglePlay(); break
    case 'ArrowRight': e.preventDefault(); nudgeHead(fine); break
    case 'ArrowLeft': e.preventDefault(); nudgeHead(-fine); break
    case 'i': case 'I':
      selection.value = { from: time.value, to: Math.max(selection.value?.to ?? time.value, time.value) }
      break
    case 'o': case 'O':
      selection.value = { from: Math.min(selection.value?.from ?? time.value, time.value), to: time.value }
      break
    case 'x': case 'X': case 'Backspace': case 'Delete':
      e.preventDefault(); applySelection(); break
    case 'z': case 'Z': undoLast(); break
    case 'e': case 'E': if (selection.value) editor.zoomToSpan(selection.value); break
    case 'f': case 'F': editor.fit(); break
    case '+': case '=': editor.zoomAround(time.value, 1 / 1.6); break
    case '-': case '_': editor.zoomAround(time.value, 1.6); break
    case 'Escape': selection.value = null; focus.value = null; break
    case 'Tab': {
      e.preventDefault()
      const list = proposals.value
      if (!list.length) break
      const i = focus.value ? list.indexOf(focus.value) : -1
      look(list[(i + 1) % list.length]!)
      break
    }
    case 'Enter': if (focus.value) accept(focus.value); break
  }
}
useEventListener(window, 'keydown', onKey)

/* ---------- the takes ---------- */

async function remove(id: number) {
  busy.value = true
  try {
    await studio.deleteTake(id)
    if (selectedId.value === id) selectedId.value = null
  } finally { busy.value = false }
}

/* ---------- the overview ---------- */

/** The whole take at a glance, with the zoomed window marked on it. */
const overview = useTemplateRef<HTMLElement>('overview')
function overviewClick(e: MouseEvent) {
  const el = overview.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const seconds = ((e.clientX - rect.left) / rect.width) * duration.value
  const span = view.value.to - view.value.from
  editor.clampView(seconds - span / 2, seconds + span / 2)
  seek(seconds)
}
const pctOf = (s: number) => (s / duration.value) * 100

const noDuration = computed(() => !!selected.value && selected.value.duration <= 0)
const hasTrace = computed(() => !!selected.value?.trace?.length)
</script>

<template>
  <div class="flex flex-col gap-2.5 min-h-0 overflow-hidden">
    <div v-if="!takes.length" class="grid place-items-center flex-1 text-sm text-score-faint">
      No hay tomas todavía. Graba la primera desde el scope de Lectura.
    </div>

    <template v-else>
      <!-- The finished reading, and the way into any take in it. Seventy-nine
           takes in a table is a scrollbar, not a list; as blocks on the
           montage they are the shape of the video. -->
      <div class="shrink-0 rounded-lg border border-score-grid bg-score-stave p-3">
        <MontagePreview @edit="id => selectedId = id" />
      </div>

      <div v-if="selected" class="flex-1 min-h-0 grid grid-cols-[minmax(0,1fr)_232px] gap-2.5">
        <!-- the editing surface -->
        <div class="flex flex-col gap-2 min-h-0">
          <UAlert
            v-if="noDuration"
            color="error"
            variant="subtle"
            title="Esta toma no tiene duración"
            description="El archivo llegó sin metadatos y el servidor no pudo medirlo. Reinicia el estudio para que lo repare."
          />

          <!-- picture and passage: where the head is, said twice -->
          <div class="flex gap-2.5 shrink-0">
            <video
              ref="player"
              :src="mediaUrl"
              class="w-[212px] aspect-video bg-black rounded-[3px] border border-score-grid object-contain shrink-0"
              preload="metadata"
              @pause="playing = false"
              @play="playing = true"
            />
            <div class="flex-1 min-w-0 rounded-[3px] border border-score-grid bg-score-stave px-3 py-2 flex flex-col justify-center">
              <template v-if="context">
                <p class="text-[10.5px] text-score-faint truncate">{{ context.previous.trim() || '·' }}</p>
                <p class="text-[13px] text-white leading-snug my-0.5">
                  <span class="text-signal tabular text-[10px] mr-2">{{ context.here.n }}</span>
                  {{ context.here.t.trim() || '—' }}
                </p>
                <p class="text-[10.5px] text-score-faint truncate">{{ context.next.trim() || '·' }}</p>
              </template>
              <p v-else class="text-[11px] text-score-faint">
                Esta toma no guardó la traza del scroll, así que no puedo decirte
                qué línea estás oyendo.
              </p>
            </div>
          </div>

          <!-- the whole take, with the window marked -->
          <div
            ref="overview"
            class="relative h-[24px] shrink-0 rounded-[3px] border border-score-grid bg-[#0d0f12] cursor-pointer overflow-hidden"
            @mousedown="overviewClick"
          >
            <div
              v-for="(c, i) in cuts"
              :key="i"
              class="absolute inset-y-0 bg-[var(--color-voice-zenon)]/35"
              :style="{ left: `${pctOf(c.from)}%`, width: `${pctOf(c.to - c.from)}%` }"
            />
            <div
              v-for="(p, i) in proposals"
              :key="`p${i}`"
              class="absolute inset-y-0 border-x border-dashed border-[var(--color-voice-socrates)]/60 bg-[var(--color-voice-socrates)]/15"
              :style="{ left: `${pctOf(p.from)}%`, width: `${pctOf(p.to - p.from)}%` }"
            />
            <div
              v-for="c in (selected.cues ?? [])"
              :key="c.quote"
              class="absolute inset-y-0 w-px"
              :style="{ left: `${pctOf(c.at)}%`, background: studio.colourOf(studio.quoteById(c.quote)?.personaje ?? '') }"
            />
            <div
              class="absolute inset-y-0 border-x border-white/45 bg-white/[0.07]"
              :style="{ left: `${pctOf(view.from)}%`, width: `${pctOf(view.to - view.from)}%` }"
            />
            <div class="absolute inset-y-0 w-px bg-signal" :style="{ left: `${pctOf(time)}%` }" />
          </div>

          <!-- the window itself -->
          <div class="flex-1 min-h-[104px]">
            <TakeTrack
              :take="selected"
              :cuts="cuts"
              :view="view"
              :time="time"
              :selection="selection"
              :proposals="proposals"
              :focus="focus"
              @seek="seek"
              @select="onSelect"
              @view="s => editor.clampView(s.from, s.to)"
              @recut="onRecut"
              @uncut="onUncut"
              @take-proposal="accept"
            />
          </div>

          <!-- transport -->
          <div class="shrink-0 flex items-center gap-1.5">
            <UButton
              size="xs"
              :icon="playing ? 'i-lucide-pause' : 'i-lucide-play'"
              variant="subtle"
              @click="togglePlay"
            />
            <span class="tabular text-[12px] text-white">{{ clock(time) }}</span>
            <span class="tabular text-[11px] text-score-faint">
              / {{ clock(selected.duration) }} · montado {{ clock(editor.netTime(time)) }}
            </span>

            <UButton
              size="xs"
              variant="subtle"
              icon="i-lucide-scissors"
              class="ml-2"
              :disabled="!selection || !!endangered.length || busy"
              @click="applySelection"
            >Cortar<template #trailing><UKbd value="X" /></template></UButton>
            <UButton
              size="xs"
              variant="ghost"
              icon="i-lucide-undo-2"
              :disabled="!cuts.length || busy"
              @click="undoLast"
            >Deshacer<template #trailing><UKbd value="Z" /></template></UButton>
            <UButton
              size="xs"
              variant="ghost"
              icon="i-lucide-trash-2"
              class="!text-[var(--color-voice-zenon)]"
              :disabled="busy || !selected"
              title="descartar esta toma entera"
              @click="selected && remove(selected.id)"
            >Descartar</UButton>

            <div class="ml-auto flex items-center gap-1">
              <UButton size="xs" variant="ghost" icon="i-lucide-zoom-out" @click="editor.zoomAround(time, 1.6)" />
              <UButton size="xs" variant="ghost" icon="i-lucide-zoom-in" @click="editor.zoomAround(time, 1 / 1.6)" />
              <UButton
                size="xs"
                variant="ghost"
                :disabled="!selection"
                @click="selection && editor.zoomToSpan(selection)"
              >Selección<template #trailing><UKbd value="E" /></template></UButton>
              <UButton size="xs" variant="ghost" :disabled="!editor.zoomed.value" @click="editor.fit()">
                Todo<template #trailing><UKbd value="F" /></template>
              </UButton>
            </div>
          </div>

          <!-- what the selection means, in the coordinates the reader thinks in -->
          <div class="shrink-0 h-[16px] text-[11px]">
            <span
              v-if="endangered.length"
              class="flex items-center gap-1.5 text-[var(--color-voice-zenon)]"
            >
              <UIcon name="i-lucide-triangle-alert" class="size-3.5 shrink-0" />
              El corte se tragaría {{ endangered.length }} cita(s) que no vuelven a sonar: muévelo.
            </span>
            <span v-else-if="selectionLines && selection" class="text-score-dim">
              <b class="text-white tabular">{{ (selection.to - selection.from).toFixed(2) }} s</b>
              · quita de la línea
              <b class="text-white tabular">{{ selectionLines.first.n }}</b>
              a la <b class="text-white tabular">{{ selectionLines.last.n }}</b>
              <span class="text-score-faint"> · «{{ selectionLines.first.t.trim().slice(0, 52) }}…»</span>
            </span>
            <span v-else-if="!hasTrace" class="text-warning">
              Sin traza del scroll: los cortes se miden en segundos, no en líneas.
            </span>
            <span v-else class="text-score-faint">
              Arrastra sobre la onda para seleccionar · <b>I</b>/<b>O</b> marcan entrada y salida ·
              rueda para acercar · <b>Tab</b> recorre las propuestas.
            </span>
          </div>
        </div>

        <!-- proposals and cuts -->
        <div class="flex flex-col gap-2.5 min-h-0 overflow-auto">
          <div class="rounded-lg border border-score-grid bg-score-stave p-2.5">
            <div class="flex items-center justify-between text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
              <span class="flex items-center gap-1.5">
                <UIcon name="i-lucide-wand-sparkles" class="size-3" />Propuestas
              </span>
              <span class="tabular">{{ proposals.length }}</span>
            </div>

            <p v-if="!proposals.length" class="text-[11px] text-score-faint">
              Nada que proponer: no volviste atrás ni dejaste silencios largos.
            </p>

            <div v-else class="grid gap-1">
              <button
                v-for="(p, i) in proposals"
                :key="i"
                class="text-left rounded-[4px] px-2 py-1.5 transition-colors"
                :class="focus === p ? 'bg-white/[0.08]' : 'hover:bg-white/[0.04]'"
                @click="look(p)"
                @dblclick="accept(p)"
              >
                <div class="flex items-center gap-1.5">
                  <span
                    class="w-1.5 h-1.5 rounded-full shrink-0"
                    :style="{ background: p.kind === 'retake' ? 'var(--color-voice-socrates)' : 'var(--color-score-dim)' }"
                  />
                  <span class="text-[11.5px] text-white">
                    {{ p.kind === 'retake' ? 'relectura' : 'silencio' }}
                  </span>
                  <span class="ml-auto tabular text-[10.5px] text-score-dim">
                    −{{ (p.to - p.from).toFixed(1) }} s
                  </span>
                </div>
                <div class="text-[10px] text-score-faint mt-0.5 tabular">
                  {{ clock(p.from) }} → {{ clock(p.to) }}
                </div>
              </button>

              <div class="flex gap-1 mt-1">
                <UButton
                  v-if="proposals.some(p => p.kind === 'retake')"
                  size="xs"
                  variant="subtle"
                  block
                  :disabled="busy"
                  @click="acceptAll('retake')"
                >Todas las relecturas</UButton>
                <UButton
                  v-if="proposals.some(p => p.kind === 'silence')"
                  size="xs"
                  variant="ghost"
                  block
                  :disabled="busy"
                  @click="acceptAll('silence')"
                >Los silencios</UButton>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-score-grid bg-score-stave p-2.5">
            <div class="flex items-center justify-between text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
              <span class="flex items-center gap-1.5">
                <UIcon name="i-lucide-scissors" class="size-3" />Cortes
              </span>
              <span class="tabular">−{{ clock(editor.removed.value) }}</span>
            </div>

            <p v-if="!cuts.length" class="text-[11px] text-score-faint">
              Esta toma entera va al montaje.
            </p>

            <div v-else class="grid gap-1">
              <div
                v-for="(c, i) in cuts"
                :key="i"
                class="group flex items-center gap-1.5 rounded-[4px] px-2 py-1 hover:bg-white/[0.04]"
              >
                <button
                  class="flex-1 text-left tabular text-[10.5px] text-score-dim hover:text-white"
                  @click="editor.zoomToSpan(c); seek(Math.max(0, c.from - 0.5))"
                >
                  {{ clock(c.from) }} → {{ clock(c.to) }}
                  <span class="text-white ml-1">−{{ (c.to - c.from).toFixed(1) }} s</span>
                </button>
                <UButton
                  size="xs"
                  variant="ghost"
                  icon="i-lucide-x"
                  class="opacity-0 group-hover:opacity-100"
                  :disabled="busy"
                  @click="onUncut(i)"
                />
              </div>
            </div>

            <div class="flex items-baseline justify-between mt-2 pt-2 border-t border-score-grid">
              <span class="text-[10px] uppercase tracking-[0.1em] text-score-faint">Neto</span>
              <span class="tabular text-[13px] text-white">{{ clock(editor.kept.value) }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
