<script setup lang="ts">
/**
 * The console. Four scopes — reading, take, montage, render — are zoom levels
 * on one score rather than separate screens, so the mental model never changes
 * while you work.
 */
const studio = useStudio()
const transport = useTransport()
const { reading, error, takes, quotes, keptSeconds, overBudget } = studio
const { state, connected, notice, nextQuote, playing, recording, paused, busy } = transport

type Scope = 'lectura' | 'montaje' | 'render'

// The scope lives in the URL so a surface can be linked to and reopened where
// you left it.
const route = useRoute()
const router = useRouter()
const scope = computed<Scope>({
  get: () => (['lectura', 'montaje', 'render'].includes(String(route.query.scope))
    ? route.query.scope as Scope
    : 'lectura'),
  set: v => router.replace({ query: { ...route.query, scope: v } }),
})
const scopes: { value: Scope; label: string; icon: string }[] = [
  { value: 'lectura', label: 'Lectura', icon: 'i-lucide-book-open' },
  { value: 'montaje', label: 'Montaje', icon: 'i-lucide-scissors' },
  { value: 'render', label: 'Render', icon: 'i-lucide-clapperboard' },
]

/** Clicking a take on the score is a request to edit it, not to seek. */
const montageTake = useState<number | null>('montage-take', () => null)
function editTake(id: number) {
  montageTake.value = id
  scope.value = 'montaje'
}

const now = ref(Date.now())

// Lifecycle hooks must be registered synchronously during setup: awaiting
// first loses the component instance, and Vue then fails the whole app's
// initialisation rather than just this hook.
let beat: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  transport.connect()
  window.addEventListener('keydown', onKey)
  beat = setInterval(() => (now.value = Date.now()), 250)
  void studio.load()
})

onUnmounted(() => {
  if (beat) clearInterval(beat)
  window.removeEventListener('keydown', onKey)
  transport.disconnect()
})

/** Projected runtime of the whole reading at the current scroll speed. */
const projected = computed(() => {
  const strip = reading.value?.altoTira ?? 0
  if (!strip || !state.value.speed) return 0
  // The strip is rendered at 1920 wide from a 1700 px master.
  return (strip * (1920 / 1700)) / state.value.speed
})

/**
 * The quote that N will sound: the one in the paragraph being recorded, not a
 * pointer walking the list. A paragraph is one turn, so the paragraph you
 * chose is the turn you meant.
 */
const upcoming = transport.currentQuote
const alreadyFired = computed(() =>
  !!upcoming.value && studio.firedQuotes.value.has(upcoming.value.clip),
)

/**
 * Move the strip by lines of the passage rather than by pixels.
 *
 * The arrows used to push 200 px of a 21 856 px strip — getting on for four
 * lines, which overshoots whatever you were aiming at, so placing the start of
 * a take meant nudging past it and running the scroll to creep back. A line is
 * the unit the reader is already in, and the studio knows where every one of
 * them sits.
 */
function stepLine(delta: number) {
  const all = studio.lines.value
  const here = studio.lineAt(state.value.f)
  if (!all.length || !here) {
    transport.nudge(delta * 200)
    return
  }
  const i = all.findIndex(l => l.n === here.n)
  const target = all[Math.min(all.length - 1, Math.max(0, i + delta))]
  if (target) transport.seek(target.f)
}

/**
 * Choose a paragraph and bring the page to it.
 *
 * The choice is what counts, and the scroll follows it. Working the other way
 * round — asking where the scroll is and calling that the paragraph — cannot
 * reach the first two: at the top of the strip the page has nowhere further to
 * go, the position clamps at four tenths of a screen in, and both of them are
 * already behind it.
 */
function goToParagraph(index: number) {
  const list = studio.paragraphs.value
  if (!list.length) return
  const chosen = Math.min(Math.max(index, 0), list.length - 1)
  const paragraph = list[chosen]!
  transport.paragraph.value = chosen
  // Choosing a paragraph is choosing to read it. Showing the lámina is a
  // separate act that sets this back afterwards, so clearing it here — the one
  // place the selection moves — keeps the two from fighting.
  transport.plate.value = false

  const span = state.value.viewport
  if (!span) {
    transport.seek(paragraph.f)
    return
  }
  // High enough that the whole paragraph is below the eye line when it fits,
  // never so high that there is no context above it.
  const height = paragraph.fEnd - paragraph.f
  const placement = Math.min(0.3, Math.max(0.08, 1 - height / span - 0.08))
  transport.seek(Math.max(0, paragraph.f + (state.value.anchor - placement) * span))
}

function stepParagraph(direction: 1 | -1) {
  goToParagraph(transport.paragraph.value + direction)
}

/** Choose a paragraph and roll on it. The seek is queued before the record. */
function recordParagraph(index: number) {
  goToParagraph(index)
  transport.startTake()
}

function onKey(e: KeyboardEvent) {
  if (e.target instanceof HTMLInputElement) return
  // The montage binds the same keys to editing — space plays the take, the
  // arrows walk the playhead — so while it is on screen it owns them. Nothing
  // is recorded from the montage anyway.
  if (scope.value === 'montaje' && !busy.value) return
  switch (e.key) {
    case ' ': e.preventDefault(); transport.toggleRun(); break
    case 'n': case 'N': case 'Enter': e.preventDefault(); transport.playNext(); break
    case 'p': case 'P': transport.replayPrevious(); break
    case 'r': case 'R':
      if (paused.value) transport.resumeTake()
      else if (recording.value) transport.pauseTake()
      else recordParagraph(transport.paragraph.value)
      break
    case 's': case 'S': transport.stopTake(); break
    case 'm': case 'M': transport.toggleMute(); break
    case 'l': case 'L': transport.togglePlate(); break
    case 'a': case 'A': transport.putArrow(state.value.plate ? null : 0.4); break
    case 'ArrowUp': e.preventDefault(); transport.setSpeed(state.value.speed + 1); break
    case 'ArrowDown': e.preventDefault(); transport.setSpeed(Math.max(1, state.value.speed - 1)); break
    case 'ArrowRight': e.preventDefault(); stepLine(e.shiftKey ? 10 : 1); break
    case 'ArrowLeft': e.preventDefault(); stepLine(e.shiftKey ? -10 : -1); break
    case 'PageDown': e.preventDefault(); stepParagraph(1); break
    case 'PageUp': e.preventDefault(); stepParagraph(-1); break
    case 'Escape': if (playing.value) transport.stopChain(); break
  }
}
</script>

<template>
  <div class="h-screen flex flex-col overflow-hidden">
    <header
      class="flex items-center gap-5 px-5 py-3 border-b border-score-grid bg-score-bg/90 backdrop-blur z-20"
    >
      <UButton to="/lecturas" variant="ghost" size="xs" icon="i-lucide-library" title="lecturas: cambiar, preparar o crear una" />
      <div class="font-semibold shrink-0" :title="reading?.lectura?.video">ME01 · Lectura <span class="text-signal">{{ String(reading?.lectura?.numero ?? 7).padStart(2, '0') }}</span></div>
      <div class="w-px h-5 bg-score-edge" />

      <div class="flex gap-0.5">
        <button
          v-for="s in scopes"
          :key="s.value"
          class="flex items-center gap-1.5 text-[10.5px] font-medium uppercase tracking-[0.07em] px-3 py-[7px] rounded-[3px] transition-colors"
          :class="scope === s.value
            ? 'bg-white text-score-bg font-semibold'
            : 'text-score-dim hover:text-white hover:bg-score-stave'"
          @click="scope = s.value"
        >
          <UIcon :name="s.icon" class="size-3.5" />
          {{ s.label }}
        </button>
      </div>

      <UButton
        v-if="!connected"
        color="neutral"
        variant="subtle"
        size="xs"
        icon="i-lucide-external-link"
        @click="transport.openPresenter()"
      >Abrir presentación</UButton>
      <UBadge v-else color="neutral" variant="subtle" size="sm" class="gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-[var(--color-voice-glaucon)]" />
        presentación enlazada
      </UBadge>

      <div class="ml-auto flex items-baseline gap-5 text-right shrink-0">
        <div v-if="busy">
          <div
            class="text-[19px] font-medium tabular"
            :class="paused ? 'text-warning' : 'text-[var(--color-voice-zenon)]'"
          >{{ paused ? '⏸' : '●' }} {{ clock(state.recorded) }}</div>
          <div class="text-[9px] uppercase tracking-[0.12em] text-score-faint">
            {{ paused ? 'en pausa' : 'grabando' }}
          </div>
        </div>
        <div>
          <div class="text-[19px] font-medium tabular">{{ clock(keptSeconds) }}</div>
          <div class="text-[9px] uppercase tracking-[0.12em] text-score-faint">montado</div>
        </div>
        <div>
          <div class="text-[19px] font-medium tabular" :class="overBudget ? 'text-warning' : ''">
            {{ clock(projected) }}
          </div>
          <div class="text-[9px] uppercase tracking-[0.12em] text-score-faint">proyección</div>
        </div>
        <div>
          <div class="text-[19px] font-medium tabular">
            {{ studio.firedQuotes.value.size }}/{{ quotes.length }}
          </div>
          <div class="text-[9px] uppercase tracking-[0.12em] text-score-faint">citas</div>
        </div>
      </div>
    </header>

    <UAlert
      v-if="error"
      color="error"
      variant="subtle"
      title="No hay conexión con el estudio"
      :description="error"
      class="mx-5 mt-4"
    />
    <UAlert
      v-else-if="notice && notice.trim()"
      color="neutral"
      variant="subtle"
      :title="notice"
      class="mx-5 mt-4"
      close
      @update:open="notice = null"
    />

    <section class="px-5 pt-4">
      <ScoreSystem
        :head="state.f"
        :seconds="projected"
        @seek="transport.seek"
        @pick="editTake"
      />
    </section>

    <!-- The rail is the recording console: a microphone, the cue you are about
         to fire, the scroll speed. None of it applies once you are cutting, and
         the 328 px it costs are 328 px of waveform, which is precision. -->
    <section
      class="grid gap-5 px-5 pt-4 pb-5 flex-1 min-h-0"
      :class="scope === 'montaje' ? 'grid-cols-1' : 'grid-cols-[1fr_328px]'"
    >
      <ParagraphScript
        v-if="scope === 'lectura'"
        :selected="transport.paragraph.value"
        @select="goToParagraph"
        @record="recordParagraph"
      />
      <MontageScope v-else-if="scope === 'montaje'" />
      <RenderScope v-else />

      <aside v-if="scope !== 'montaje'" class="flex flex-col gap-3 min-w-0 overflow-auto">
        <QuotePicker @select="goToParagraph" />

        <RecordPanel />

        <div class="shrink-0 grid grid-cols-2 gap-1.5">
          <UButton
            block
            class="col-span-2 !text-score-bg font-semibold"
            :style="upcoming && !playing ? { background: studio.colourOf(upcoming.personaje) } : undefined"
            :color="playing ? 'neutral' : undefined"
            :variant="playing ? 'subtle' : 'solid'"
            :icon="playing ? 'i-lucide-square' : 'i-lucide-volume-2'"
            :disabled="!upcoming || !connected"
            @click="playing ? transport.stopChain() : transport.playNext()"
          >
            {{ playing ? 'Cortar la cita' : 'Sonar la cita' }}
            <template #trailing><UKbd :value="playing ? 'Esc' : 'N'" /></template>
          </UButton>

          <!-- A take is one paragraph, so moving between them is a first-class
               control and not something buried in the keyboard. -->
          <UButton
            variant="subtle"
            block
            icon="i-lucide-chevron-up"
            :disabled="!connected || !studio.paragraphs.value.length"
            @click="stepParagraph(-1)"
          >Párrafo<template #trailing><UKbd value="Re Pág" /></template></UButton>

          <UButton
            variant="subtle"
            block
            icon="i-lucide-chevron-down"
            :disabled="!connected || !studio.paragraphs.value.length"
            @click="stepParagraph(1)"
          >Párrafo<template #trailing><UKbd value="Av Pág" /></template></UButton>

          <UButton
            variant="subtle"
            block
            icon="i-lucide-rotate-ccw"
            :disabled="!connected"
            @click="transport.replayPrevious()"
          >Repetir la cita<template #trailing><UKbd value="P" /></template></UButton>

          <UButton
            variant="subtle"
            block
            :icon="state.running ? 'i-lucide-pause' : 'i-lucide-play'"
            :disabled="!connected"
            @click="transport.toggleRun()"
          >{{ state.running ? 'Pausar' : 'Correr' }}<template #trailing><UKbd value="espacio" /></template></UButton>

          <UButton
            variant="subtle"
            block
            icon="i-lucide-image"
            :color="state.plate ? 'primary' : 'neutral'"
            :disabled="!connected"
            @click="transport.togglePlate()"
          >Lámina<template #trailing><UKbd value="L" /></template></UButton>

          <UButton
            variant="subtle"
            block
            icon="i-lucide-arrow-right"
            :disabled="!connected"
            @click="transport.putArrow(0.4)"
          >Flecha<template #trailing><UKbd value="A" /></template></UButton>
        </div>

        <div class="shrink-0 rounded-lg border border-score-grid bg-score-stave p-3">
          <div class="flex items-center gap-1.5 text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
            <UIcon name="i-lucide-list" class="size-3" />
            Ir a una sección
          </div>
          <div class="grid gap-1">
            <button
              v-for="(s, i) in studio.sections.value"
              :key="s.linea"
              class="flex items-center gap-1.5 text-left text-[11.5px] px-2 py-1 rounded-[4px] transition-colors"
              :class="state.f >= s.f && state.f < (studio.sections.value[i + 1]?.f ?? 1)
                ? 'bg-white/[0.06] text-white'
                : 'text-score-dim hover:bg-white/[0.03] hover:text-white'"
              :disabled="!connected"
              @click="transport.goToSection(i)"
            >
              <UIcon
                :name="state.f >= s.f && state.f < (studio.sections.value[i + 1]?.f ?? 1)
                  ? 'i-lucide-play' : 'i-lucide-minus'"
                class="size-3 shrink-0 opacity-60"
              />
              <span class="truncate">{{ s.titulo }}</span>
            </button>
          </div>
        </div>

        <div class="shrink-0 rounded-lg border border-score-grid bg-score-stave p-3">
          <div class="flex items-center justify-between text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
            <span class="flex items-center gap-1.5"><UIcon name="i-lucide-gauge" class="size-3" />Velocidad</span>
            <span class="tabular">{{ state.speed }} px/s</span>
          </div>
          <USlider
            :model-value="state.speed"
            :min="8"
            :max="40"
            :step="1"
            size="xs"
            @update:model-value="v => transport.setSpeed(Number(v))"
          />
          <p class="text-[11px] text-score-faint mt-2">
            A esta velocidad la lectura dura {{ clock(projected) }}.
            <span v-if="overBudget" class="text-warning">Se pasa de 25 min.</span>
          </p>
        </div>

        <div class="mt-auto text-[11px] text-score-faint">
          {{ takes.length }} tomas · {{ reading?.paginas ?? 0 }} páginas
        </div>
      </aside>
    </section>
  </div>
</template>
