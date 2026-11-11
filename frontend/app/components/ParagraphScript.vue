<script setup lang="ts">
/**
 * The passage as a list of takes.
 *
 * A reading is seventy-nine paragraphs and each one is recorded on its own, so
 * the question the console has to answer is not "where is the scroll" but
 * "which paragraph am I doing, and which ones are done". Choosing is explicit:
 * the scroll follows the choice rather than the choice being guessed from the
 * scroll, which is what made the first two paragraphs unreachable — at the top
 * of the strip the page cannot scroll far enough to put them under the eye
 * line, so the reported position clamped and always landed on the third.
 */
const props = defineProps<{ selected: number }>()

const emit = defineEmits<{ select: [index: number]; record: [index: number] }>()

const studio = useStudio()
const transport = useTransport()
const { paragraphs, quotes } = studio
const { connected, armed, busy } = transport

/** Quote id keyed by the line it starts on, so a turn shows whose voice it is. */
const quoteByLine = computed(() => {
  const map = new Map<number, (typeof quotes.value)[number]>()
  for (const q of quotes.value) map.set(q.linea, q)
  return map
})

const deleting = ref(false)

/** Discard a take. The paragraph then falls back to an earlier one, or empties. */
async function discard(id: number) {
  deleting.value = true
  try { await studio.deleteTake(id) } finally { deleting.value = false }
}

const rows = computed(() =>
  paragraphs.value.map((p) => {
    const all = studio.takesOfParagraph(p.index)
    const take = studio.takeOfParagraph(p.index)
    const quote = studio.lines.value
      .slice(p.from, p.to + 1)
      .map(l => quoteByLine.value.get(l.n))
      .find(Boolean)
    return {
      paragraph: p,
      take,
      quote,
      // Figures go right after the paragraph whose sentence names them: the
      // first mention as a take of its own, the rest as a note.
      figures: studio.figureEvents.value.filter(e => e.paragraph === p.index),
      // 2.f: the render keeps this figure on the right while the paragraph is
      // read. The presentation does not show it, so the list says so.
      fixed: [...new Set((studio.reading.value?.persistencias ?? [])
        .filter(f => f.nDesde <= (studio.lines.value[p.to]?.n ?? 0) && f.nHasta >= (studio.lines.value[p.from]?.n ?? 0))
        .map(f => studio.figureName(f.figura)))],
      // The printed name of whoever speaks this turn. The reader says it in
      // his own voice, then fires the clip.
      label: studio.speakerLabel(p.index),
      saysName: !!take && !!quote && studio.saysTheName(take, quote.clip),
      attempts: all.length,
      newest: all[all.length - 1] ?? null,
      // The tail of a quote is the same clip continuing: it belongs to the
      // take of the paragraph the quote starts in, and asking for one of its
      // own is asking for a recording that should not exist.
      tail: studio.quoteTails.value.has(p.index),
    }
  }),
)

function figureChosen(id: string) {
  return transport.plate.value && transport.figure.value === id
}

/** Choosing a figure parks on the paragraph that names it. */
function chooseFigure(paragraph: number, id: string) {
  emit('select', paragraph)
  transport.plate.value = true
  transport.figure.value = id
}

function recordFigure(paragraph: number, id: string) {
  chooseFigure(paragraph, id)
  transport.startTake()
}

/** Turns whose name is missing from the recording. */
const toRedo = computed(() => studio.turnsToRedo.value)

const recordable = computed(() => rows.value.filter(r => !r.tail))
const done = computed(() => recordable.value.filter(r => r.take).length)

/** Net seconds of everything recorded, which is the number the rules care about. */
const recorded = computed(() => studio.keptSeconds.value)

const list = useTemplateRef<HTMLElement>('list')

/** Keep the chosen paragraph in view when it is chosen with the keyboard. */
watch(() => props.selected, async () => {
  await nextTick()
  list.value?.querySelector('[data-selected="true"]')
    ?.scrollIntoView({ block: 'center', behavior: 'smooth' })
})
</script>

<template>
  <div class="flex flex-col min-h-0 overflow-hidden">
    <div class="shrink-0 flex items-baseline gap-2 text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2 px-1">
      <UIcon name="i-lucide-pilcrow" class="size-3" />
      <span>Párrafos</span>
      <span class="tabular text-score-dim">
        {{ done }} de {{ recordable.length }} · {{ clock(recorded) }}
      </span>
      <span
        v-if="toRedo.length"
        class="normal-case tracking-normal text-[10.5px] text-[var(--color-voice-zenon)]"
      >
        {{ toRedo.length }} turnos sin el nombre dicho
      </span>
      <span class="ml-auto normal-case tracking-normal text-[10.5px]">
        <b>Av Pág</b> / <b>Re Pág</b> para moverte · <b>R</b> graba el elegido
      </span>
    </div>

    <div ref="list" class="flex-1 min-h-0 overflow-y-auto pr-1">
      <template v-for="row in rows" :key="row.paragraph.index">
      <div
        :data-selected="row.paragraph.index === selected"
        class="group flex gap-2.5 px-2 py-1.5 rounded-[5px] cursor-pointer transition-colors"
        :class="row.paragraph.index === selected
          ? 'bg-white/[0.07]'
          : 'hover:bg-white/[0.03]'"
        @click="emit('select', row.paragraph.index)"
      >
        <!-- number and state: what is done, at a glance, down the whole list -->
        <div class="shrink-0 w-[52px] pt-[3px] flex items-start gap-1">
          <span
            class="tabular text-[10px]"
            :class="row.paragraph.index === selected ? 'text-signal' : 'text-score-faint'"
          >{{ row.paragraph.index + 1 }}</span>
          <UIcon
            v-if="row.take"
            name="i-lucide-check"
            class="size-3 mt-px text-[var(--color-voice-glaucon)]"
            :title="`grabado · ${clock(row.take.duration)}`"
          />
          <UIcon
            v-else-if="row.tail"
            name="i-lucide-corner-down-right"
            class="size-3 mt-px text-score-faint"
            title="sigue la cita de arriba: no lleva toma propia"
          />
        </div>

        <p
          class="flex-1 min-w-0 font-serif text-[15px] leading-[1.5]"
          :class="row.paragraph.index === selected
            ? 'text-white'
            : row.tail ? 'text-[#565e6c] italic'
              : row.take ? 'text-[#6b7484]' : 'text-[#98a2b0]'"
        >
          <span
            v-if="row.quote"
            class="inline-block font-sans text-[10px] font-bold px-[6px] py-px rounded-[4px] mr-1.5 align-middle text-score-bg"
            :style="{ background: studio.colourOf(row.quote.personaje) }"
          >{{ studio.firedQuotes.value.has(row.quote.clip) ? '✓' : '▶ N' }} ·
            {{ row.quote.personaje }}</span><span
            v-if="row.label && !row.saysName"
            class="inline-block font-sans text-[10px] font-bold px-[6px] py-px rounded-[4px] mr-1.5 align-middle bg-[var(--color-voice-zenon)] text-score-bg"
          >di «{{ row.label }}.» y luego N</span><span
            v-for="name in row.fixed"
            :key="name"
            class="inline-block font-sans text-[10px] font-semibold px-[6px] py-px rounded-[4px] mr-1.5 align-middle border border-[var(--color-voice-whitehead)] text-[var(--color-voice-whitehead)]"
            :title="`El video muestra ${name} fija a la derecha mientras se lee este párrafo (2.f)`"
          >{{ name }} fija</span>{{ row.paragraph.text }}
        </p>

        <div class="shrink-0 w-[96px] flex items-start justify-end gap-1 pt-px">
          <span
            v-if="row.attempts > 1"
            class="tabular text-[9.5px] text-score-faint"
            :title="`${row.attempts} tomas de este párrafo; se usa la última`"
          >{{ row.attempts }}×</span>
          <span v-if="row.take" class="tabular text-[10px] text-score-faint">
            {{ clock(row.take.duration) }}
          </span>
          <UButton
            v-if="row.newest"
            size="xs"
            variant="ghost"
            icon="i-lucide-trash-2"
            class="opacity-0 group-hover:opacity-100 !text-[var(--color-voice-zenon)]"
            :disabled="deleting || busy"
            :title="row.attempts > 1
              ? 'borrar la última toma y volver a la anterior'
              : 'borrar la toma de este párrafo'"
            @click.stop="discard(row.newest.id)"
          />
          <UButton
            v-if="!row.tail"
            size="xs"
            variant="ghost"
            icon="i-lucide-circle"
            class="opacity-0 group-hover:opacity-100 data-[on=true]:opacity-100 !text-[var(--color-voice-zenon)]"
            :data-on="row.paragraph.index === selected"
            :disabled="!connected || !armed || busy || deleting"
            :title="row.take ? 'volver a grabar este párrafo' : 'grabar este párrafo'"
            @click.stop="emit('record', row.paragraph.index)"
          />
        </div>
      </div>

      <!-- Lineamiento 3.2.3: primera mención, la figura entera con su pie (2.d);
           las siguientes, sin pie 2 o 3 s (2.e). Al volver, flecha. -->
      <template v-for="fig in row.figures" :key="`${fig.figura}-${fig.n}`">
        <div
          v-if="fig.modo === 'pie'"
          class="group flex gap-2.5 px-2 py-1.5 rounded-[5px] cursor-pointer transition-colors"
          :class="figureChosen(fig.figura) ? 'bg-white/[0.07]' : 'hover:bg-white/[0.03]'"
          @click="chooseFigure(row.paragraph.index, fig.figura)"
        >
          <div class="shrink-0 w-[52px] pt-[3px] flex items-start gap-1">
            <UIcon
              name="i-lucide-image"
              class="size-3 mt-px"
              :class="figureChosen(fig.figura) ? 'text-signal' : 'text-score-faint'"
            />
            <UIcon
              v-if="studio.figureTake(fig.figura)"
              name="i-lucide-check"
              class="size-3 mt-px text-[var(--color-voice-glaucon)]"
            />
          </div>

          <p
            class="flex-1 min-w-0 font-serif text-[15px] leading-[1.5] italic"
            :class="figureChosen(fig.figura) ? 'text-white' : 'text-[#98a2b0]'"
          >
            <span class="not-italic font-sans text-[10px] font-bold px-[6px] py-px rounded-[4px] mr-1.5 align-middle text-score-bg bg-[var(--color-voice-whitehead)] uppercase">{{ fig.nombre }}</span>
            <template v-if="studio.figureTake(fig.figura)">
              Primera mención: grabada aparte, a pantalla completa con su pie, después del
              párrafo. Al volver, el render pone la flecha.
            </template>
            <template v-else>
              Primera mención: lee el pie dentro del párrafo, justo después de nombrarla.
              El render la pone a pantalla completa mientras lo lees, la deja
              {{ studio.reading.value?.pantalla?.segundosFigura ?? 3 }} s y vuelve con la flecha.
              O grábalo aparte con <b>L</b>.
            </template>
          </p>

          <div class="shrink-0 w-[96px] flex items-start justify-end gap-1 pt-px">
            <span v-if="studio.figureTake(fig.figura)" class="tabular text-[10px] text-score-faint">
              {{ clock(studio.figureTake(fig.figura)!.duration) }}
            </span>
            <UButton
              v-if="studio.figureTake(fig.figura)"
              size="xs"
              variant="ghost"
              icon="i-lucide-trash-2"
              class="opacity-0 group-hover:opacity-100 !text-[var(--color-voice-zenon)]"
              :disabled="deleting"
              @click.stop="discard(studio.figureTake(fig.figura)!.id)"
            />
            <UButton
              size="xs"
              variant="ghost"
              icon="i-lucide-circle"
              class="opacity-0 group-hover:opacity-100 data-[on=true]:opacity-100 !text-[var(--color-voice-zenon)]"
              :data-on="figureChosen(fig.figura)"
              :disabled="!connected || !armed || busy || deleting"
              :title="`grabar ${fig.nombre}`"
              @click.stop="recordFigure(row.paragraph.index, fig.figura)"
            />
          </div>
        </div>

        <div v-else class="flex gap-2.5 px-2 py-1">
          <div class="shrink-0 w-[52px] pt-[3px]">
            <UIcon name="i-lucide-image" class="size-3 text-score-faint" />
          </div>
          <p class="flex-1 min-w-0 text-[12px] leading-[1.5] text-score-faint">
            <span class="font-bold uppercase">{{ fig.nombre }}</span>
            otra vez: al terminar la frase que la nombra, sin pie,
            {{ studio.reading.value?.pantalla?.segundosFigura ?? 3 }} s, y flecha al volver.
            No se graba; la pone el render.
          </p>
        </div>
      </template>
      </template>

      <div v-if="!rows.length" class="text-score-faint text-sm px-2.5 py-4">
        Cargando el guion…
      </div>
    </div>
  </div>
</template>
