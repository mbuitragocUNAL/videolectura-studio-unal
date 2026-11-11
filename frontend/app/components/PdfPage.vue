<script setup lang="ts">
/**
 * One page of the book with the analysis laid over it.
 *
 * The lines are the ones lector.py found, drawn where they sit on the page, so
 * choosing where the reading starts is pointing at the line itself rather
 * than picking a number from a list. Lines outside the reading are washed
 * out; headings, captions and the lines that name a figure are marked.
 */
import type { PDFDocumentProxy } from 'pdfjs-dist'
import { drawPage } from '~/composables/usePdf'

export interface PageLine {
  i: number
  pag: number
  /** Paragraph, as lector.py numbers them. */
  p: number
  t: string
  tipo: string
  x0: number
  y0: number
  x1: number
  y1: number
  /** dentro | fuera — inside the chosen stretch or not. */
  estado: 'dentro' | 'fuera'
  /** Colour of a fixed figure (2.f) covering this line, if any. */
  fija?: string
  /** A figure named on this line. */
  mencion?: boolean
  cita?: string
}

const props = defineProps<{
  doc: PDFDocumentProxy
  page: number
  width: number
  lines: PageLine[]
  first?: number
  last?: number
}>()
const emit = defineEmits<{ line: [line: PageLine, event: MouseEvent] }>()

const canvas = useTemplateRef<HTMLCanvasElement>('canvas')
const root = useTemplateRef<HTMLElement>('root')
const scale = ref(0)
const height = ref(props.width * 1.3)
const visible = ref(false)

useIntersectionObserver(root, ([entry]) => {
  if (entry?.isIntersecting) visible.value = true
}, { rootMargin: '400px' })

let drawing = 0
async function draw() {
  if (!canvas.value || !visible.value) return
  const ticket = ++drawing
  const out = await drawPage(props.doc, props.page, canvas.value, props.width)
  if (ticket !== drawing) return
  scale.value = out.scale
  height.value = out.height
}
watch([visible, () => props.page, () => props.width, () => props.doc], draw)
onMounted(draw)
</script>

<template>
  <div ref="root" class="relative bg-white rounded-[3px] shadow-[0_6px_30px_rgba(0,0,0,.45)]"
       :style="{ width: `${width}px`, height: `${height}px` }">
    <canvas ref="canvas" class="block" />
    <template v-if="scale">
      <button
        v-for="l in lines"
        :key="l.i"
        type="button"
        class="absolute rounded-[2px] transition-colors cursor-pointer"
        :class="[
          l.estado === 'fuera' ? 'bg-white/75 hover:bg-white/40' : 'hover:bg-[#f5f749]/30',
          l.i === first ? 'ring-2 ring-[#2ba36b]' : '',
          l.i === last ? 'ring-2 ring-[#d9364f]' : '',
          l.mencion ? 'bg-[#4fb0ff]/20' : '',
          l.cita ? 'bg-[#ff9f45]/25' : '',
        ]"
        :style="{
          left: `${(l.x0 - 3) * scale}px`,
          top: `${(l.y0 - 1) * scale}px`,
          width: `${(l.x1 - l.x0 + 6) * scale}px`,
          height: `${(l.y1 - l.y0 + 2) * scale}px`,
        }"
        :title="l.t"
        @click="emit('line', l, $event)"
      />
      <!-- 2.f: a bar in the margin where a figure stays fixed on the right. -->
      <div
        v-for="l in lines.filter(x => x.fija)"
        :key="`f${l.i}`"
        class="absolute w-[4px] pointer-events-none"
        :style="{
          left: `${(l.x1 + 10) * scale}px`,
          top: `${(l.y0 - 1) * scale}px`,
          height: `${(l.y1 - l.y0 + 4) * scale}px`,
          background: l.fija,
        }"
      />
    </template>
    <div class="absolute -top-5 left-0 text-[10px] text-score-dim tabular">p. {{ page }}</div>
  </div>
</template>
