<script setup lang="ts">
/**
 * The finished reading, watchable before it is rendered.
 *
 * It plays the takes in the order the passage runs, skipping what has been
 * cut, by pointing one video element at each take's media in turn. There is a
 * short gap at every take boundary while the next file opens — this is a
 * preview, not the delivery, and the render splices them properly. What it is
 * for is hearing whether a splice lands and seeing whether the reading holds
 * together, at the point where both are still cheap to change.
 */
import type { Segment } from '~/composables/useMontage'

const studio = useStudio()
const montage = useMontage()
const { segments, total, missing, superseded } = montage

const purging = ref(false)

/** Clear out the takes the video is not using, once you are happy with it. */
async function purge() {
  purging.value = true
  try {
    for (const take of [...superseded.value]) await studio.deleteTake(take.id)
  } finally { purging.value = false }
}

const emit = defineEmits<{ edit: [takeId: number] }>()

const player = useTemplateRef<HTMLVideoElement>('player')
const bar = useTemplateRef<HTMLElement>('bar')

/** Second of the finished video the head is on. */
const time = ref(0)
const playing = ref(false)
const current = ref<Segment | null>(null)

const mediaUrl = (takeId: number) => `/api/takes/${takeId}/media`

/** Put the head at a second of the finished video and load what plays there. */
async function seek(seconds: number, keepPlaying = playing.value) {
  const target = Math.min(Math.max(seconds, 0), Math.max(total.value - 0.05, 0))
  time.value = target
  const found = montage.sourceTime(target)
  const video = player.value
  if (!found || !video) return

  if (current.value?.takeId !== found.segment.takeId) {
    video.src = mediaUrl(found.segment.takeId)
    // Wait for the new file before seeking into it: setting currentTime on a
    // video that has not opened yet is silently dropped.
    await new Promise<void>((resolve) => {
      const ready = () => { video.removeEventListener('loadedmetadata', ready); resolve() }
      video.addEventListener('loadedmetadata', ready)
      setTimeout(ready, 4000)
    })
  }
  current.value = found.segment
  video.currentTime = found.at
  if (keepPlaying) void video.play()
}

/**
 * Follow the take's own clock and hand over at the end of each stretch. A
 * segment ends either because it was cut short or because the take did.
 */
let frame = 0
function follow() {
  frame = requestAnimationFrame(follow)
  const video = player.value
  const segment = current.value
  if (!video || !segment || video.paused) return

  if (video.currentTime >= segment.to - 0.02) {
    const next = segment.startsAt + (segment.to - segment.from) + 0.01
    if (next >= total.value) { video.pause(); playing.value = false; return }
    void seek(next, true)
    return
  }
  time.value = segment.startsAt + (video.currentTime - segment.from)
}
onMounted(() => { frame = requestAnimationFrame(follow); void seek(0, false) })
onUnmounted(() => cancelAnimationFrame(frame))

watch(total, () => { if (time.value > total.value) void seek(0, false) })

function toggle() {
  const video = player.value
  if (!video) return
  if (video.paused) { void video.play(); playing.value = true }
  else { video.pause(); playing.value = false }
}

function onBar(e: MouseEvent) {
  if (e.shiftKey || e.button === 1) {
    panning = { clientX: e.clientX, from: view.value.from }
    chosenView.value = true
    e.preventDefault()
    return
  }
  if (!total.value) return
  void seek(secondsAt(e.clientX))
}

/* ---------- the window ---------- */

/**
 * Fifty takes across a thousand pixels is twenty pixels each, which is not a
 * thing you can aim at. The bar shows a window of the finished video and the
 * wheel narrows it, in the same grammar as the score.
 */
const view = ref({ from: 0, to: 1 })
const span = computed(() => Math.max(view.value.to - view.value.from, 1e-6))
const zoomed = computed(() => span.value < total.value - 1e-6)

const { width: barWidth } = useElementSize(bar)

function setView(from: number, to: number) {
  const whole = Math.max(total.value, 1e-6)
  const width = Math.min(Math.max(to - from, 1.5), whole)
  const start = Math.min(Math.max(from, 0), whole - width)
  view.value = { from: Math.max(0, start), to: Math.min(whole, start + width) }
}

/**
 * Whether the window is where the viewer put it. Without this the bar opened
 * zoomed to a second and a half: the first run of the watch below happens
 * while the takes are still loading and the total is zero, and from then on
 * the window looked deliberately narrowed and was never widened again.
 */
const chosenView = ref(false)

function fitBar() {
  chosenView.value = false
  setView(0, Math.max(total.value, 1e-6))
}

watch(total, () => { if (!chosenView.value) setView(0, Math.max(total.value, 1e-6)) }, { immediate: true })

const pct = (seconds: number) => ((seconds - view.value.from) / span.value) * 100
const widthPct = (seconds: number) => (seconds / span.value) * 100

function secondsAt(clientX: number) {
  const el = bar.value
  if (!el) return 0
  const rect = el.getBoundingClientRect()
  const local = (clientX - rect.left) / Math.max(rect.width, 1)
  return Math.min(total.value, Math.max(0, view.value.from + local * span.value))
}

let panning: { clientX: number; from: number } | null = null

function onBarWheel(e: WheelEvent) {
  e.preventDefault()
  chosenView.value = true
  if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) {
    const step = (e.deltaX / Math.max(barWidth.value, 1)) * span.value
    setView(view.value.from + step, view.value.to + step)
    return
  }
  const anchor = secondsAt(e.clientX)
  const held = (anchor - view.value.from) / span.value
  const next = span.value * (e.deltaY > 0 ? 1.25 : 1 / 1.25)
  setView(anchor - held * next, anchor - held * next + next)
}

useEventListener(window, 'mouseup', () => { panning = null })
useEventListener(window, 'mousemove', (e: MouseEvent) => {
  if (!panning) return
  const moved = (e.clientX - panning.clientX) / Math.max(barWidth.value, 1)
  setView(panning.from - moved * span.value, panning.from - moved * span.value + span.value)
})

/** Keep the head in the window while it plays, unless it is being panned. */
watch(time, (t) => {
  if (!zoomed.value || panning) return
  if (t >= view.value.from + span.value * 0.05 && t <= view.value.to - span.value * 0.05) return
  setView(t - span.value / 2, t + span.value / 2)
})

/* ---------- stepping take by take ---------- */

/** Zoom to one take and put the head at its start: the way to reach ¶47. */
function show(takeId: number) {
  const block = blocks.value.find(b => b.takeId === takeId)
  if (!block) return
  chosenView.value = true
  const pad = Math.max((block.to - block.from) * 0.6, 2)
  setView(block.from - pad, block.to + pad)
  void seek(block.from + 0.01)
}

function stepTake(direction: 1 | -1) {
  const list = blocks.value
  if (!list.length) return
  const here = list.findIndex(b => b.takeId === current.value?.takeId)
  const next = list[Math.min(Math.max((here < 0 ? 0 : here) + direction, 0), list.length - 1)]
  if (next) show(next.takeId)
}

/** One block per take, so the reading reads as a row of paragraphs. */
const blocks = computed(() => {
  const byTake = new Map<number, { takeId: number; label: string; from: number; to: number }>()
  for (const s of segments.value) {
    const seen = byTake.get(s.takeId)
    const end = s.startsAt + (s.to - s.from)
    if (seen) seen.to = end
    else byTake.set(s.takeId, { takeId: s.takeId, label: s.label, from: s.startsAt, to: end })
  }
  return [...byTake.values()]
})

/** Only what the window can show, so fifty blocks are not all laid out at once. */
const visibleBlocks = computed(() =>
  blocks.value.filter(b => b.to >= view.value.from && b.from <= view.value.to),
)

/** Picking a paragraph by name, for when the block is a sliver on the bar. */
const jump = ref<number | undefined>(undefined)
watch(jump, (takeId) => {
  if (takeId === undefined) return
  show(takeId)
  jump.value = undefined
})

const overBudget = computed(() => total.value > 25 * 60)
</script>

<template>
  <div class="flex flex-col gap-2.5 min-h-0">
    <div class="flex items-baseline gap-2 text-[9.5px] uppercase tracking-[0.12em] text-score-faint">
      <UIcon name="i-lucide-clapperboard" class="size-3" />
      <span>Montaje</span>
      <span class="tabular text-score-dim">
        {{ blocks.length }} {{ blocks.length === 1 ? 'toma' : 'tomas' }} · {{ clock(total) }}
      </span>
      <span v-if="overBudget" class="tabular text-warning normal-case tracking-normal">
        se pasa de 25 min
      </span>
      <span v-if="missing.length" class="ml-auto normal-case tracking-normal text-[10.5px] text-warning">
        {{ missing.length === 1 ? 'falta 1 párrafo' : `faltan ${missing.length} párrafos` }} por grabar
      </span>
      <!-- Re-reading a paragraph leaves the earlier attempt on disk and out of
           the video. Saying so beats letting the folder quietly fill up. -->
      <button
        v-if="superseded.length"
        class="normal-case tracking-normal text-[10.5px] text-score-dim hover:text-white underline underline-offset-2"
        :class="missing.length ? 'ml-2' : 'ml-auto'"
        :disabled="purging"
        :title="superseded.map(t => t.label).join('\n')"
        @click="purge"
      >{{ superseded.length }}
        {{ superseded.length === 1 ? 'toma descartada' : 'tomas descartadas' }} · borrarlas</button>
    </div>

    <div class="flex gap-2.5 min-h-0">
      <video
        ref="player"
        class="w-[280px] aspect-video bg-black rounded-[3px] border border-score-grid object-contain shrink-0"
        preload="auto"
        @pause="playing = false"
        @play="playing = true"
      />

      <div class="flex-1 min-w-0 flex flex-col gap-2">
        <!-- the assembled reading, one block per take -->
        <div
          ref="bar"
          class="relative h-[42px] rounded-[3px] border border-score-grid bg-[#0d0f12] overflow-hidden cursor-col-resize"
          @mousedown="onBar"
          @wheel="onBarWheel"
        >
          <div
            v-for="b in visibleBlocks"
            :key="b.takeId"
            class="absolute top-[3px] bottom-[3px] rounded-[2px] border overflow-hidden cursor-pointer"
            :class="current?.takeId === b.takeId
              ? 'bg-[#26303c] border-signal'
              : 'bg-[#1d232b] border-[#2a323c] hover:border-score-faint'"
            :style="{ left: `${pct(b.from)}%`, width: `${Math.max(widthPct(b.to - b.from), 0.4)}%` }"
            :title="`${b.label} · ${clock(b.to - b.from)}`"
            @mousedown.stop="show(b.takeId)"
            @dblclick.stop="emit('edit', b.takeId)"
          >
            <span
              v-if="widthPct(b.to - b.from) > 2.5"
              class="absolute inset-x-1 top-[3px] text-[9px] tabular text-score-dim truncate"
            >{{ b.label }}</span>
          </div>

          <div
            class="absolute inset-y-0 w-px bg-signal pointer-events-none"
            style="box-shadow: 0 0 10px rgba(245,247,73,.6)"
            :style="{ left: `${pct(time)}%` }"
          />

          <div
            v-if="!blocks.length"
            class="absolute inset-0 grid place-items-center text-[11px] text-score-faint"
          >Todavía no hay nada montado.</div>
        </div>

        <div class="flex items-center gap-1.5">
          <UButton
            size="xs"
            variant="subtle"
            :icon="playing ? 'i-lucide-pause' : 'i-lucide-play'"
            :disabled="!blocks.length"
            @click="toggle"
          />
          <UButton
            size="xs"
            variant="ghost"
            icon="i-lucide-chevron-left"
            :disabled="!blocks.length"
            title="la toma anterior"
            @click="stepTake(-1)"
          />
          <UButton
            size="xs"
            variant="ghost"
            icon="i-lucide-chevron-right"
            :disabled="!blocks.length"
            title="la toma siguiente"
            @click="stepTake(1)"
          />
          <span class="tabular text-[12px] text-white">{{ clock(time) }}</span>
          <span class="tabular text-[11px] text-score-faint">/ {{ clock(total) }}</span>
          <span v-if="current" class="text-[11px] text-score-dim truncate">
            {{ current.label }}
          </span>
          <div class="ml-auto shrink-0 flex items-center gap-1">
            <USelectMenu
              v-model="jump"
              :items="blocks"
              label-key="label"
              value-key="takeId"
              size="xs"
              placeholder="Ir a un párrafo…"
              class="w-[190px]"
              :search-input="{ placeholder: 'buscar ¶…' }"
            />
            <UButton
              size="xs"
              variant="ghost"
              icon="i-lucide-minimize-2"
              :disabled="!zoomed"
              title="ver el montaje entero"
              @click="fitBar()"
            />
            <UButton
              v-if="current"
              size="xs"
              variant="ghost"
              icon="i-lucide-scissors"
              @click="emit('edit', current.takeId)"
            >Editar</UButton>
          </div>
        </div>

        <p class="text-[10.5px] text-score-faint">
          Rueda para acercar · <b>shift</b> y arrastrar para desplazar · clic en
          un bloque para ir a él, doble clic para editarlo.
          <span v-if="zoomed" class="text-score-dim">
            Viendo {{ clock(view.from) }}–{{ clock(view.to) }} de {{ clock(total) }}.
          </span>
          Hay un salto breve al cambiar de toma mientras se abre el archivo
          siguiente; el render las empalma de verdad.
        </p>
      </div>
    </div>
  </div>
</template>
