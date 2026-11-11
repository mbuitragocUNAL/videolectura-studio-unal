<script setup lang="ts">
/**
 * Preparing a reading, by looking at the book.
 *
 * Everything lectura.toml says is chosen here against the pages themselves:
 * the chapter from the book's own index, where the reading starts and stops by
 * pointing at the line, which figures are shown and where they stay fixed, and
 * which quotations get a synthetic voice. Every change is re-analysed by
 * preparar/lector.py and drawn over the page, so what will happen is visible
 * before anything is written. Saving writes lectura.toml and prepares the
 * reading; the file stays the source of truth and can still be edited by hand.
 */
import type { PDFDocumentProxy } from 'pdfjs-dist'
import type { PageLine } from '~/components/PdfPage.vue'
import { type Chapter, chaptersOf, cropOf, loadPdf } from '~/composables/usePdf'

interface Linea { i: number; pag: number; t: string; tipo: string; p: number; x0: number; y0: number; x1: number; y1: number }
interface Figura {
  id: string; nombre: string; pagina: number | null; encontrada: boolean; pie: string
  omitida: boolean; persistente: boolean; menciones: number[]; enLaTira: boolean
  img?: number[]; imgPie?: number[]
}
interface Analisis {
  titulo: string; libro: string; desde: number; hasta: number
  todas: Linea[]
  secciones: { i: number; t: string; pag: number }[]
  figuras: Figura[]
  eventos: { figura: string; modo: 'pie' | 'breve'; i: number }[]
  persistencias: { figura: string; desde: number; hasta: number; manual?: boolean }[]
  citas: { k: number; personaje: string; texto?: string; i?: number; iFin?: number; error?: string }[]
  candidatas: { i: number; texto: string }[]
  duracion: Record<string, number>
  avisos: string[]
}

const route = useRoute()
const id = ref(String(route.query.id ?? ''))

/* ---------- configuration: the same shape as lectura.toml ---------- */

const config = reactive<any>({
  lectura: { curso: 'ME', grupo: 1, numero: 0, titulo: '', libro: '', paginas: '' },
  pantalla: { dpi: 200, ancho_figura: 0.38, segundos_figura: 3 },
  figuras: { detectar: true, persistente: true },
  figura: [] as any[],
  fija: [] as any[],
  cita: [] as any[],
  voces: {} as Record<string, { color?: string; descripcion?: string; motor?: string; voz?: string; voz_nombre?: string }>,
})

const shelf = ref<{ nombre: string; bytes: number }[]>([])
const bookUrl = ref('')
const bookName = ref('')
const doc = shallowRef<PDFDocumentProxy | null>(null)
const chapters = ref<Chapter[]>([])
const error = ref('')
const loading = ref(true)

async function loadShelf() {
  shelf.value = await $fetch('/api/libros')
}

async function openBook(url: string, name: string) {
  bookUrl.value = url
  bookName.value = name
  doc.value = null
  chapters.value = []
  try {
    const d = await loadPdf(url)
    doc.value = d
    chapters.value = await chaptersOf(d)
  } catch (e: any) {
    error.value = `No pude abrir el libro: ${e?.message ?? e}`
  }
}

function chooseShelfBook(name: string) {
  config.lectura.libro = name
  resetStretch()
  openBook(`/api/libros/${encodeURIComponent(name)}/archivo`, name)
}

const uploading = ref(false)
async function upload(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploading.value = true
  error.value = ''
  try {
    const form = new FormData()
    form.append('file', file, file.name)
    await $fetch('/api/libros', { method: 'POST', body: form })
    await loadShelf()
    chooseShelfBook(file.name)
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
  } finally {
    uploading.value = false
  }
}

onMounted(async () => {
  try {
    await loadShelf()
    if (id.value) {
      const r = await $fetch<{ config: any; libroNombre: string; libroRuta: string }>(
        `/api/lecturas/${id.value}/config`)
      for (const key of Object.keys(r.config)) {
        if (Array.isArray(r.config[key])) config[key] = r.config[key]
        else config[key] = { ...(config[key] ?? {}), ...r.config[key] }
      }
      lastSaved.value = request.value
      const onShelf = shelf.value.find(b => b.nombre === r.libroNombre)
      if (onShelf) await openBook(`/api/libros/${encodeURIComponent(onShelf.nombre)}/archivo`, onShelf.nombre)
      else if (r.libroRuta) await openBook(`/api/lecturas/${id.value}/libro.pdf`, r.libroNombre)
    }
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
  } finally {
    loading.value = false
  }
})

/* ---------- pages ---------- */

function pagesOf(spec: string): number[] {
  const out: number[] = []
  for (const part of String(spec).replace(/\s/g, '').split(',')) {
    if (!part) continue
    const [a, b] = part.split('-').map(Number)
    if (!a) continue
    for (let p = a; p <= (b || a); p++) out.push(p)
  }
  return out
}
const pages = computed(() => pagesOf(config.lectura.paginas))

function resetStretch() {
  for (const k of ['desde', 'hasta', 'desde_i', 'hasta_i']) delete config.lectura[k]
  config.fija = []
}

function chooseChapter(c: Chapter) {
  config.lectura.paginas = c.hasta > c.pagina ? `${c.pagina}-${c.hasta}` : String(c.pagina)
  if (!config.lectura.titulo || chapters.value.some(x => x.titulo === config.lectura.titulo)) {
    config.lectura.titulo = c.titulo
  }
  resetStretch()
}
const chosenChapter = computed(() =>
  chapters.value.find(c => config.lectura.paginas === (c.hasta > c.pagina ? `${c.pagina}-${c.hasta}` : String(c.pagina))))

/* ---------- analysis, on every change ---------- */

const analysis = ref<Analisis | null>(null)
const analysing = ref(false)
const analysisError = ref('')

const request = computed(() => JSON.stringify({ config }))
/** What lectura.toml says now, to know whether there is anything unsaved. */
const lastSaved = ref('')
const dirty = computed(() => !id.value || request.value !== lastSaved.value)
let ticket = 0
async function analyse() {
  if (!config.lectura.libro || !pages.value.length) return
  const mine = ++ticket
  analysing.value = true
  try {
    const r = await $fetch<Analisis>(`/api/analizar${id.value ? `?id=${id.value}` : ''}`, {
      method: 'POST', body: JSON.parse(request.value),
    })
    if (mine !== ticket) return
    analysis.value = r
    analysisError.value = ''
  } catch (e: any) {
    if (mine === ticket) analysisError.value = e?.data?.error ?? e?.message ?? String(e)
  } finally {
    if (mine === ticket) analysing.value = false
  }
}
watchDebounced(request, analyse, { debounce: 650 })
watch(() => loading.value, v => { if (!v) analyse() })

const lines = computed(() => analysis.value?.todas ?? [])
const figureById = computed(() => new Map((analysis.value?.figuras ?? []).map(f => [f.id, f])))

/* ---------- colours: figures and voices ---------- */

const PALETTE = ['#4fb0ff', '#ff9f45', '#45d98a', '#a98bff', '#ff5470', '#2ec4c4', '#f06bc4', '#c8c84a']
function figureColour(figId: string) {
  const i = (analysis.value?.figuras ?? []).findIndex(f => f.id === figId)
  return PALETTE[(i < 0 ? 0 : i) % PALETTE.length]!
}
const speakers = computed(() => [...new Set(config.cita.map((c: any) => (c.personaje ?? '').trim()).filter(Boolean))] as string[])
function voiceColour(name: string) {
  return config.voces[name]?.color || PALETTE[(speakers.value.indexOf(name) + 1) % PALETTE.length]!
}

/* ---------- what each line is, for the pages and the text ---------- */

const lineState = computed(() => {
  const a = analysis.value
  const map = new Map<number, { fija?: string; mencion?: boolean; cita?: string }>()
  if (!a) return map
  for (const p of a.persistencias) {
    for (let i = p.desde; i <= p.hasta; i++) map.set(i, { ...map.get(i), fija: figureColour(p.figura) })
  }
  for (const f of a.figuras) for (const i of f.menciones) map.set(i, { ...map.get(i), mencion: true })
  for (const c of a.citas) {
    if (c.i === undefined || c.iFin === undefined) continue
    for (let i = c.i; i <= c.iFin; i++) map.set(i, { ...map.get(i), cita: voiceColour(c.personaje) })
  }
  return map
})

function pageLines(page: number): PageLine[] {
  const a = analysis.value
  if (!a) return []
  return a.todas.filter(l => l.pag === page).map(l => ({
    ...l,
    estado: l.i >= a.desde && l.i <= a.hasta ? 'dentro' : 'fuera',
    ...lineState.value.get(l.i),
  }))
}

/* ---------- choosing by pointing ---------- */

const menu = ref<{ x: number; y: number; line: Linea } | null>(null)
const menuEl = useTemplateRef<HTMLElement>('menuEl')
onClickOutside(menuEl, () => { menu.value = null })

/** While set, a click fixes this figure over the paragraph clicked (2.f). */
const fixing = ref<string | null>(null)

function onLine(line: Linea, e: MouseEvent) {
  if (fixing.value) {
    fixParagraph(fixing.value, line)
    fixing.value = null
    return
  }
  menu.value = { x: e.clientX, y: e.clientY, line }
}

/** In the text view a click is also how a selection ends; only a plain click picks. */
function onTextLine(line: Linea, e: MouseEvent) {
  if (window.getSelection()?.toString()) return
  onLine(line, e)
}

function startHere(line: Linea) {
  config.lectura.desde_i = line.i
  delete config.lectura.desde
  menu.value = null
}
function endHere(line: Linea) {
  config.lectura.hasta_i = line.i
  delete config.lectura.hasta
  menu.value = null
}
function startAtSection(i: number) {
  config.lectura.desde_i = i
  delete config.lectura.desde
}
/** The reading ends with the line before this heading. */
function endBeforeSection(i: number) {
  config.lectura.hasta_i = Math.max(0, i - 1)
  delete config.lectura.hasta
}
function wholePages() {
  for (const k of ['desde', 'hasta', 'desde_i', 'hasta_i']) delete config.lectura[k]
}

function paragraphRange(line: Linea) {
  const same = lines.value.filter(l => l.p === line.p)
  return { desde: same[0]?.i ?? line.i, hasta: same.at(-1)?.i ?? line.i }
}

/* ---------- figures ---------- */

function figureOverride(nombre: string) {
  let entry = config.figura.find((f: any) => f.nombre === nombre)
  if (!entry) {
    entry = { nombre }
    config.figura.push(entry)
  }
  return entry
}
function setFigure(nombre: string, key: string, value: unknown) {
  const entry = figureOverride(nombre)
  if (value === undefined || value === null || value === '') delete entry[key]
  else entry[key] = value
  if (Object.keys(entry).length === 1) config.figura = config.figura.filter((f: any) => f !== entry)
}
function fixParagraph(nombre: string, line: Linea) {
  const r = paragraphRange(line)
  config.fija.push({ figura: nombre, desde_i: r.desde, hasta_i: r.hasta })
}
function fixedOf(nombre: string) {
  return config.fija
    .map((f: any, index: number) => ({ ...f, index }))
    .filter((f: any) => f.figura === nombre)
}
function fixedLabel(f: any) {
  const t = f.desde_i !== undefined ? lines.value[f.desde_i]?.t : f.desde
  return (t ?? '').slice(0, 48)
}

const crops = reactive<Record<string, string>>({})
watch([doc, () => analysis.value?.figuras], async () => {
  const d = doc.value
  if (!d) return
  for (const f of analysis.value?.figuras ?? []) {
    const key = `${f.id}@${f.pagina}`
    if (!f.encontrada || !f.pagina || !f.imgPie || crops[key]) continue
    crops[key] = await cropOf(d, f.pagina, f.imgPie, 320).catch(() => '')
  }
}, { immediate: true })

/* ---------- quotations ---------- */

function addQuote(texto: string) {
  config.cita.push({ personaje: '', texto: texto.trim() })
}
const selection = ref('')
function readSelection() {
  const s = window.getSelection()?.toString().trim() ?? ''
  selection.value = s.split(/\s+/).length >= 2 ? s : ''
}
function quoteError(k: number) {
  return analysis.value?.citas.find(c => c.k === k)?.error
}
function voice(name: string) {
  if (!config.voces[name]) config.voces[name] = {}
  return config.voces[name]
}

/* ---------- saving ---------- */

function slug(s: string) {
  return s.normalize('NFKD').replace(/[̀-ͯ]/g, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
}
const pad = (n: number) => String(n || 0).padStart(2, '0')
const videoTitle = computed(() =>
  `${config.lectura.curso}0${config.lectura.grupo} Lectura ${pad(config.lectura.numero)} ${config.lectura.titulo}`)
const folder = computed(() => id.value || `${pad(config.lectura.numero)}-${slug(config.lectura.titulo)}`)

const saving = ref(false)
const saved = ref<{ titulo: string; lineas: number; figuras: number; citas: number; avisos: string[] } | null>(null)
const ready = computed(() => !!(config.lectura.libro && pages.value.length && config.lectura.numero && config.lectura.titulo))

async function save(): Promise<boolean> {
  saving.value = true
  error.value = ''
  saved.value = null
  try {
    // Voices nobody quotes any more are dropped rather than written.
    for (const name of Object.keys(config.voces)) if (!speakers.value.includes(name)) delete config.voces[name]
    saved.value = await $fetch(`/api/lecturas/${folder.value}`, { method: 'PUT', body: { config } })
    if (!id.value) {
      id.value = folder.value
      await navigateTo({ query: { id: folder.value } }, { replace: true })
    }
    // The anchors were rewritten as text on save; reload them so the next
    // analysis uses exactly what lectura.toml now says.
    const r = await $fetch<{ config: any }>(`/api/lecturas/${id.value}/config`)
    config.lectura = { ...config.lectura, ...r.config.lectura }
    for (const k of ['desde_i', 'hasta_i']) delete config.lectura[k]
    config.fija = r.config.fija ?? []
    lastSaved.value = request.value
    return true
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
    return false
  } finally {
    saving.value = false
  }
}

async function record() {
  await $fetch('/api/lecturas/activa', { method: 'POST', body: { id: id.value } })
  window.location.href = '/'
}

/* ---------- layout ---------- */

const view = ref<'paginas' | 'texto'>('paginas')
const right = useTemplateRef<HTMLElement>('right')
const { width: rightWidth } = useElementSize(right)
const pageWidth = computed(() => Math.max(280, Math.min(620, (rightWidth.value - 72) / 2)))
const clock = (s: number) => `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}`

const fueraDeRango = (s: number) => s < 300 || s > 1500
</script>

<template>
  <div class="h-screen flex flex-col overflow-hidden">
    <header class="flex items-center gap-4 px-5 py-3 border-b border-score-grid">
      <NuxtLink to="/lecturas" class="text-score-dim hover:text-white text-sm flex items-center gap-1">
        <UIcon name="i-lucide-arrow-left" class="size-4" /> Lecturas
      </NuxtLink>
      <div class="w-px h-5 bg-score-edge" />
      <div class="font-semibold">{{ id ? 'Preparar' : 'Nueva lectura' }}</div>
      <div class="text-score-dim text-sm truncate">{{ videoTitle }}</div>
      <div class="flex-1" />
      <span v-if="analysing" class="text-[11px] text-score-dim flex items-center gap-1">
        <UIcon name="i-lucide-loader-circle" class="size-3.5 animate-spin" /> analizando
      </span>
      <UButton :disabled="!ready" :loading="saving" color="primary" variant="solid" icon="i-lucide-save" @click="save">
        Guardar y preparar
      </UButton>
      <UButton v-if="saved && id" icon="i-lucide-mic" @click="record">Grabar</UButton>
    </header>

    <div class="flex-1 flex min-h-0">
      <!-- ================= controls ================= -->
      <aside class="w-[400px] shrink-0 border-r border-score-grid overflow-y-auto px-4 py-4 flex flex-col gap-5 text-sm">
        <UAlert v-if="error" color="error" variant="subtle" :title="error" />
        <UAlert v-if="saved" color="success" variant="subtle"
                :title="`Preparada: ${saved.lineas} líneas, ${saved.figuras} figuras, ${saved.citas} citas`"
                :description="saved.avisos.join(' · ') || 'lectura.toml escrito. Ya se puede grabar.'" />

        <!-- 1. Libro -->
        <section>
          <h2 class="paso"><span>1</span> Libro</h2>
          <div class="flex flex-col gap-1">
            <button v-for="b in shelf" :key="b.nombre"
                    class="text-left px-2.5 py-1.5 rounded-[4px] border text-[13px] truncate"
                    :class="bookName === b.nombre ? 'border-signal/60 bg-score-stave text-white' : 'border-score-grid text-score-dim hover:text-white'"
                    @click="chooseShelfBook(b.nombre)">
              <UIcon name="i-lucide-book" class="size-3.5 mr-1 align-[-2px]" />{{ b.nombre }}
            </button>
            <div v-if="bookName && !shelf.some(b => b.nombre === bookName)"
                 class="px-2.5 py-1.5 rounded-[4px] border border-signal/60 bg-score-stave text-[13px] truncate">
              <UIcon name="i-lucide-book" class="size-3.5 mr-1 align-[-2px]" />{{ bookName }}
              <span class="text-score-dim">(fuera del estante)</span>
            </div>
            <label class="mt-1 inline-flex items-center gap-2 text-[12px] text-score-dim hover:text-white cursor-pointer">
              <UIcon :name="uploading ? 'i-lucide-loader-circle' : 'i-lucide-upload'" class="size-4" :class="uploading && 'animate-spin'" />
              Subir un PDF
              <input type="file" accept="application/pdf" class="hidden" @change="upload">
            </label>
            <p class="text-[11px] text-score-faint">Los libros quedan en <code>libros/</code>, fuera de git.</p>
          </div>
        </section>

        <!-- 2. Páginas -->
        <section v-if="doc">
          <h2 class="paso"><span>2</span> Páginas</h2>
          <p class="text-[12px] text-score-dim mb-2">Elige el capítulo en el índice del libro, o escribe las páginas del PDF.</p>
          <div class="max-h-[220px] overflow-y-auto border border-score-grid rounded-[4px] mb-2">
            <button v-for="c in chapters" :key="`${c.titulo}${c.pagina}`"
                    class="w-full text-left px-2.5 py-1 text-[12.5px] flex gap-2 hover:bg-score-stave"
                    :class="chosenChapter === c ? 'bg-score-stave text-white' : 'text-score-dim'"
                    :style="{ paddingLeft: `${10 + (c.nivel - 1) * 14}px` }"
                    @click="chooseChapter(c)">
              <span class="flex-1 truncate">{{ c.titulo }}</span>
              <span class="tabular text-score-faint">{{ c.pagina }}–{{ c.hasta }}</span>
            </button>
            <div v-if="!chapters.length" class="px-2.5 py-2 text-[12px] text-score-faint">Este PDF no trae índice.</div>
          </div>
          <label class="flex items-center gap-2 text-[12px] text-score-dim">
            Páginas del PDF
            <UInput v-model="config.lectura.paginas" placeholder="207-218" class="flex-1" @change="resetStretch" />
          </label>
        </section>

        <!-- 3. Tramo -->
        <section v-if="analysis">
          <h2 class="paso"><span>3</span> Dónde empieza y termina</h2>
          <p class="text-[12px] text-score-dim mb-2">
            Haz clic en una línea de la página para empezar o terminar ahí, o usa las secciones.
          </p>
          <div class="flex flex-col gap-0.5 mb-2">
            <div v-for="s in analysis.secciones" :key="s.i" class="flex items-center gap-1 group">
              <span class="flex-1 truncate text-[12.5px]"
                    :class="s.i >= analysis.desde && s.i <= analysis.hasta ? 'text-white' : 'text-score-faint line-through'">
                {{ s.t }}
              </span>
              <UButton size="xs" variant="ghost" class="opacity-50 group-hover:opacity-100" title="empezar en esta sección"
                       icon="i-lucide-arrow-down-to-line" @click="startAtSection(s.i)" />
              <UButton size="xs" variant="ghost" class="opacity-50 group-hover:opacity-100" title="terminar antes de esta sección"
                       icon="i-lucide-arrow-up-to-line" @click="endBeforeSection(s.i)" />
            </div>
          </div>
          <div class="text-[12px] text-score-dim leading-relaxed">
            <div><b class="text-[#2ba36b]">Empieza:</b> «{{ lines[analysis.desde]?.t.slice(0, 60) }}» (p. {{ lines[analysis.desde]?.pag }})</div>
            <div><b class="text-[#d9364f]">Termina:</b> «{{ lines[analysis.hasta]?.t.slice(-60) }}» (p. {{ lines[analysis.hasta]?.pag }})</div>
          </div>
          <UButton size="xs" variant="link" class="px-0" @click="wholePages">Páginas completas</UButton>
        </section>

        <!-- 4. Figuras -->
        <section v-if="analysis">
          <h2 class="paso"><span>4</span> Figuras y láminas</h2>
          <p class="text-[12px] text-score-dim mb-2">
            Primera mención: con su pie. Las siguientes: sin pie, {{ config.pantalla.segundos_figura }} s.
            Siempre con flecha al volver. La barra de color en la página marca dónde queda fija a la derecha (2.f).
          </p>
          <div v-if="!analysis.figuras.length" class="text-[12px] text-score-faint">El texto no nombra figuras.</div>
          <div v-for="f in analysis.figuras" :key="f.id" class="border border-score-grid rounded-[5px] p-2.5 mb-2"
               :class="f.omitida && 'opacity-50'">
            <div class="flex items-center gap-2 mb-1.5">
              <span class="w-2.5 h-2.5 rounded-full" :style="{ background: figureColour(f.id) }" />
              <b class="flex-1">{{ f.nombre }}</b>
              <USwitch :model-value="!f.omitida" size="xs" @update:model-value="v => setFigure(f.nombre, 'omitir', v ? undefined : true)" />
            </div>
            <img v-if="crops[`${f.id}@${f.pagina}`]" :src="crops[`${f.id}@${f.pagina}`]" class="w-full rounded-[3px] mb-1.5 bg-white">
            <div class="text-[11.5px] text-score-dim mb-1.5">
              <template v-if="f.encontrada">
                p. {{ f.pagina }}{{ f.enLaTira ? '' : ', fuera del pasaje' }} ·
                nombrada {{ f.menciones.length }} {{ f.menciones.length === 1 ? 'vez' : 'veces' }}
              </template>
              <span v-else class="text-[var(--ui-error)]">No encontré su pie. Indica la página del PDF:</span>
            </div>
            <div class="flex items-center gap-2 text-[12px]">
              <UInput :model-value="config.figura.find((x: any) => x.nombre === f.nombre)?.pagina ?? ''"
                      size="xs" class="w-20" placeholder="página"
                      @change="(e: Event) => setFigure(f.nombre, 'pagina', Number((e.target as HTMLInputElement).value) || undefined)" />
              <label class="flex items-center gap-1.5 text-score-dim">
                <USwitch size="xs" :model-value="f.persistente"
                         @update:model-value="v => setFigure(f.nombre, 'persistente', v ? undefined : false)" />
                fija donde la nombran
              </label>
            </div>
            <div class="mt-1.5 flex flex-wrap gap-1">
              <span v-for="x in fixedOf(f.nombre)" :key="x.index"
                    class="text-[11px] px-1.5 py-0.5 rounded-[3px] bg-score-stave border border-score-edge flex items-center gap-1">
                «{{ fixedLabel(x) }}…»
                <button class="text-score-dim hover:text-white" @click="config.fija.splice(x.index, 1)">×</button>
              </span>
              <UButton size="xs" variant="ghost" :color="fixing === f.nombre ? 'warning' : 'neutral'" icon="i-lucide-pin"
                       @click="fixing = fixing === f.nombre ? null : f.nombre">
                {{ fixing === f.nombre ? 'Haz clic en un párrafo…' : 'Fijar en otro párrafo' }}
              </UButton>
            </div>
          </div>
          <div class="flex items-center gap-2 text-[12px] text-score-dim mt-1">
            sin pie
            <UInput v-model.number="config.pantalla.segundos_figura" size="xs" type="number" class="w-14" /> s ·
            figura fija ocupa
            <UInput v-model.number="config.pantalla.ancho_figura" size="xs" type="number" step="0.02" class="w-16" />
          </div>
        </section>

        <!-- 5. Citas -->
        <section v-if="analysis">
          <h2 class="paso"><span>5</span> Citas con voz sintética</h2>
          <p class="text-[12px] text-score-dim mb-2">
            Lo decide quien lee (2.i). Selecciona el texto en la vista «Texto» para agregarlo, o usa una sugerencia.
          </p>
          <div v-for="(c, k) in config.cita" :key="k" class="border border-score-grid rounded-[5px] p-2 mb-2">
            <div class="flex items-center gap-2 mb-1">
              <span class="w-2.5 h-2.5 rounded-full shrink-0" :style="{ background: voiceColour(c.personaje) }" />
              <UInput v-model="c.personaje" size="xs" placeholder="personaje" class="flex-1" />
              <UButton size="xs" variant="ghost" icon="i-lucide-trash-2" @click="config.cita.splice(k, 1)" />
            </div>
            <UTextarea v-model="c.texto" :rows="2" autoresize size="xs" class="w-full" />
            <div v-if="quoteError(k + 1)" class="text-[11px] text-[var(--ui-error)] mt-1">{{ quoteError(k + 1) }}</div>
          </div>
          <div v-if="analysis.candidatas.length" class="mb-2">
            <div class="text-[11px] uppercase tracking-wider text-score-faint mb-1">Sugerencias</div>
            <button v-for="c in analysis.candidatas" :key="c.i"
                    class="w-full text-left text-[12px] px-2 py-1 rounded-[4px] hover:bg-score-stave text-score-dim flex gap-2"
                    @click="addQuote(c.texto)">
              <UIcon name="i-lucide-plus" class="size-3.5 mt-0.5 shrink-0" /> {{ c.texto }}
            </button>
          </div>
          <VoiceCast v-if="id && speakers.length" :reading-id="id" :config="config" :speakers="speakers"
                     :colour="voiceColour" :dirty="dirty" :save="save" />
          <template v-else>
            <div v-for="name in speakers" :key="name" class="flex items-center gap-2 mb-1 text-[12px]">
              <input type="color" :value="voiceColour(name)" class="w-6 h-6 bg-transparent"
                     @input="(e: Event) => voice(name).color = (e.target as HTMLInputElement).value">
              <b class="w-24 truncate">{{ name }}</b>
              <UInput :model-value="config.voces[name]?.descripcion ?? ''" size="xs" class="flex-1"
                      placeholder="cómo suena: edad, oficio, ánimo"
                      @update:model-value="(v: string) => voice(name).descripcion = v" />
            </div>
            <div v-if="speakers.length" class="text-[11px] text-score-faint">
              Al guardar la lectura se pueden diseñar las voces y generar los clips aquí mismo.
            </div>
          </template>
        </section>

        <!-- 6. Datos -->
        <section v-if="doc">
          <h2 class="paso"><span>6</span> Datos del video</h2>
          <div class="grid grid-cols-[70px_1fr] gap-x-2 gap-y-1.5 items-center text-[12px]">
            <span class="text-score-dim">Curso</span>
            <USelect v-model="config.lectura.curso" :items="['ME', 'LP']" size="xs" class="w-24" />
            <span class="text-score-dim">Grupo</span>
            <UInput v-model.number="config.lectura.grupo" type="number" size="xs" class="w-24" />
            <span class="text-score-dim">Lectura</span>
            <UInput v-model.number="config.lectura.numero" type="number" size="xs" class="w-24" placeholder="22" />
            <span class="text-score-dim">Título</span>
            <UInput v-model="config.lectura.titulo" size="xs" class="w-full" />
          </div>
          <div class="mt-2 text-[12px] text-score-dim">
            Título en YouTube (2.k):<br><b class="text-white">{{ videoTitle }}</b>
          </div>
          <div v-if="analysis" class="mt-2 text-[12px] text-score-dim">
            Duración estimada:
            <span v-for="(s, ppm) in analysis.duracion" :key="ppm" class="mr-2"
                  :class="fueraDeRango(s) ? 'text-[var(--ui-warning)]' : 'text-white'">
              {{ clock(s) }} <span class="text-score-faint">a {{ ppm }}</span>
            </span>
            <div class="text-score-faint">El lineamiento pide entre 5 y 25 minutos.</div>
          </div>
          <div class="mt-2 text-[11px] text-score-faint">Carpeta: <code>lecturas/{{ folder }}</code></div>
        </section>

        <!-- 7. Música -->
        <section v-if="analysis">
          <h2 class="paso"><span>7</span> Música de fondo</h2>
          <MusicPicker :config="config" />
        </section>

        <section v-if="analysis?.avisos.length">
          <UAlert color="warning" variant="subtle" title="Avisos" :description="analysis.avisos.join(' · ')" />
        </section>
        <UAlert v-if="analysisError" color="error" variant="subtle" :title="analysisError" />
      </aside>

      <!-- ================= the book ================= -->
      <main ref="right" class="flex-1 min-w-0 flex flex-col">
        <div class="flex items-center gap-1 px-4 py-2 border-b border-score-grid text-[11px] uppercase tracking-wider">
          <button v-for="v in (['paginas', 'texto'] as const)" :key="v"
                  class="px-3 py-1 rounded-[3px]"
                  :class="view === v ? 'bg-white text-score-bg font-semibold' : 'text-score-dim hover:text-white'"
                  @click="view = v">{{ v === 'paginas' ? 'Páginas' : 'Texto' }}</button>
          <div class="flex-1" />
          <span class="normal-case tracking-normal text-score-faint">
            <span class="inline-block w-2 h-2 ring-2 ring-[#2ba36b] mr-1" />empieza
            <span class="inline-block w-2 h-2 ring-2 ring-[#d9364f] mx-1 ml-3" />termina
            <span class="inline-block w-2 h-2 bg-[#4fb0ff]/40 mx-1 ml-3" />nombra una figura
            <span class="inline-block w-2 h-2 bg-[#ff9f45]/50 mx-1 ml-3" />cita
          </span>
        </div>

        <div v-if="fixing" class="px-4 py-2 bg-[var(--ui-warning)]/15 text-[12.5px]">
          Haz clic en el párrafo donde <b>{{ fixing }}</b> debe quedar fija a la derecha.
          <button class="underline ml-2" @click="fixing = null">cancelar</button>
        </div>

        <div class="flex-1 overflow-y-auto px-6 py-8">
          <div v-if="loading" class="text-score-dim">Cargando…</div>
          <div v-else-if="!doc" class="text-score-dim max-w-md">
            Elige un libro del estante o sube el PDF. Después, el capítulo.
          </div>
          <div v-else-if="!pages.length" class="text-score-dim">Elige el capítulo o las páginas.</div>

          <div v-else-if="view === 'paginas'" class="flex flex-wrap gap-x-6 gap-y-10">
            <PdfPage v-for="p in pages" :key="p" :doc="doc" :page="p" :width="pageWidth"
                     :lines="pageLines(p)" :first="analysis?.desde" :last="analysis?.hasta"
                     @line="onLine" />
          </div>

          <div v-else class="max-w-[720px] font-serif text-[16px] leading-[1.6]" @mouseup="readSelection">
            <template v-for="l in lines" :key="l.i">
              <div v-if="l.i > 0 && l.pag !== lines[l.i - 1]?.pag"
                   class="font-sans text-[10px] text-score-faint my-3 border-t border-score-grid pt-1">p. {{ l.pag }}</div>
              <div class="relative pl-3 cursor-pointer rounded-[2px] hover:bg-white/[0.04]"
                   :class="[
                     analysis && (l.i < analysis.desde || l.i > analysis.hasta) ? 'text-score-faint' : 'text-[#d8dde3]',
                     l.tipo === 'titulo' && 'font-semibold text-white mt-3',
                     l.tipo === 'pie' && 'italic text-[13px] text-score-dim',
                     l.i === analysis?.desde && 'ring-1 ring-[#2ba36b]',
                     l.i === analysis?.hasta && 'ring-1 ring-[#d9364f]',
                     lineState.get(l.i)?.mencion && 'bg-[#4fb0ff]/10',
                     lineState.get(l.i)?.cita && 'bg-[#ff9f45]/15',
                   ]"
                   @click="(e: MouseEvent) => onTextLine(l, e)">
                <span v-if="lineState.get(l.i)?.fija" class="absolute left-0 top-0 bottom-0 w-[3px]"
                      :style="{ background: lineState.get(l.i)?.fija }" />
                {{ l.t }}
              </div>
            </template>
          </div>
        </div>

        <div v-if="selection" class="px-4 py-2 border-t border-score-grid flex items-center gap-3 text-[12.5px]">
          <span class="truncate flex-1 text-score-dim">«{{ selection }}»</span>
          <UButton size="xs" icon="i-lucide-quote" @click="addQuote(selection); selection = ''">Agregar como cita</UButton>
        </div>
      </main>
    </div>

    <!-- what a line can be: start, end, the paragraph of a figure -->
    <div v-if="menu" ref="menuEl"
         class="fixed z-50 bg-score-grid border border-score-edge rounded-[6px] shadow-2xl p-1 flex flex-col text-[12.5px] min-w-[220px]"
         :style="{ left: `${menu.x + 6}px`, top: `${menu.y + 6}px` }">
      <div class="px-2 py-1 text-[11px] text-score-dim truncate max-w-[300px]">«{{ menu.line.t }}»</div>
      <button class="menu-item" @click="startHere(menu.line)"><UIcon name="i-lucide-arrow-down-to-line" /> La lectura empieza aquí</button>
      <button class="menu-item" @click="endHere(menu.line)"><UIcon name="i-lucide-arrow-up-to-line" /> La lectura termina aquí</button>
      <template v-for="f in (analysis?.figuras ?? []).filter(x => x.encontrada && !x.omitida)" :key="f.id">
        <button class="menu-item" @click="fixParagraph(f.nombre, menu.line); menu = null">
          <UIcon name="i-lucide-pin" :style="{ color: figureColour(f.id) }" /> Fijar {{ f.nombre }} en este párrafo
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.paso {
  display: flex; align-items: center; gap: 8px;
  font-size: 11px; text-transform: uppercase; letter-spacing: .08em;
  color: var(--color-score-dim); font-weight: 600; margin-bottom: 8px;
}
.paso span {
  display: inline-grid; place-items: center; width: 18px; height: 18px; border-radius: 50%;
  background: var(--color-score-edge); color: #fff; font-size: 10px;
}
.menu-item {
  display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 4px; text-align: left;
}
.menu-item:hover { background: var(--color-score-edge); }
</style>
