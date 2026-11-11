<script setup lang="ts">
/**
 * The voices of the quotes (2.i): one per character, the same in every quote.
 *
 * A character's voice is designed from its description — three candidates to
 * listen to, one kept — or picked from the ElevenLabs account or from the
 * local voices made before. Then every clip is generated at once, at the
 * narration's loudness. A clip that was recorded or generated elsewhere can be
 * uploaded instead. Each clip remembers the text and voice it was made with,
 * so only the quotes or characters that changed are made again.
 *
 * The work is preparar/voz.py, behind the hub; this panel follows its job.
 */
interface Engines {
  elevenlabs: { clave: boolean; origen: string; final: string; modelo: string }
  qwen: { instalado: boolean; dispositivo: string; error?: string }
}
interface Character { nombre: string; motor: string; voz: string; vozNombre: string; clips: number; caracteres: number }
interface Clip {
  clip: string; personaje: string; texto: string; caracteres: number; dur: number
  estado: 'falta' | 'lista' | 'texto' | 'otra-voz'; motor: string
}
interface VoiceState { personajes: Character[]; clips: Clip[]; pendientes: number }
interface Voice { id: string; nombre: string; categoria: string; descripcion: string; muestra: string }
interface Design { motor: string; texto: string; carpeta: string; candidatas: { id: string; archivo: string; dur: number }[] }
interface Job {
  id: string; lectura: string; tipo: 'generar' | 'disenar'; personaje?: string
  terminado: boolean; paso: string; hecho: number; total: number; resultado?: any; error?: string
}

const props = defineProps<{
  readingId: string
  config: any
  speakers: string[]
  colour: (name: string) => string
  dirty: boolean
  save: () => Promise<boolean>
}>()

const error = ref('')
const failed = (e: any) => { error.value = e?.data?.error ?? e?.message ?? String(e) }

/* ---------- engines and the ElevenLabs key ---------- */

const engines = ref<Engines | null>(null)
const account = ref<{ nivel: string; usados: number; limite: number } | null>(null)
const keyDraft = ref('')
const editingKey = ref(false)

async function loadEngines() {
  engines.value = await $fetch<Engines>('/api/voces/motores').catch((e) => { failed(e); return null })
  account.value = null
  if (engines.value?.elevenlabs.clave) {
    account.value = await $fetch<any>('/api/voces/cuenta').catch((e) => { failed(e); return null })
  }
}

async function saveKey(clave: string) {
  error.value = ''
  try {
    engines.value = await $fetch<Engines>('/api/voces/clave', { method: 'PUT', body: { clave } })
    keyDraft.value = ''
    editingKey.value = false
    elevenVoices.value = null
    await loadEngines()
  } catch (e) { failed(e) }
}

/* ---------- voices to choose from ---------- */

const elevenVoices = ref<Voice[] | null>(null)
const localVoices = ref<Voice[] | null>(null)

async function loadVoices(motor: string) {
  try {
    if (motor === 'elevenlabs' && engines.value?.elevenlabs.clave && !elevenVoices.value) {
      elevenVoices.value = await $fetch<Voice[]>('/api/voces/lista?motor=elevenlabs')
    }
    if (motor === 'qwen' && !localVoices.value) {
      localVoices.value = await $fetch<Voice[]>('/api/voces/lista?motor=qwen')
    }
  } catch (e) { failed(e) }
}

function entry(name: string) {
  if (!props.config.voces[name]) props.config.voces[name] = {}
  return props.config.voces[name]
}
const motorOf = (name: string) => props.config.voces[name]?.motor || 'manual'

function setMotor(name: string, motor: string) {
  const v = entry(name)
  if (v.motor === motor) return
  v.motor = motor
  delete v.voz
  delete v.voz_nombre
  loadVoices(motor)
}

function voiceItems(name: string) {
  const list = motorOf(name) === 'qwen' ? localVoices.value : elevenVoices.value
  return (list ?? []).map(v => ({
    label: v.nombre, value: v.id,
    description: v.categoria === 'premade' ? 'de ElevenLabs' : (v.descripcion || v.categoria).slice(0, 60),
  }))
}

function setVoice(name: string, id: string) {
  const v = entry(name)
  v.voz = id
  v.voz_nombre = voiceItems(name).find(i => i.value === id)?.label ?? ''
}

const MOTORS = [
  { label: 'ElevenLabs', value: 'elevenlabs' },
  { label: 'Qwen (local)', value: 'qwen' },
  { label: 'Audio propio', value: 'manual' },
]

/* ---------- the reading's clips ---------- */

const state = ref<VoiceState | null>(null)
async function loadState() {
  if (!props.readingId) return
  state.value = await $fetch<VoiceState>(`/api/lecturas/${props.readingId}/voces`).catch(() => null)
}
const clipsOf = (name: string) => state.value?.clips.filter(c => c.personaje === name) ?? []
const pending = computed(() => (state.value?.clips ?? []).filter(c => c.estado !== 'lista'))
const toGenerate = computed(() => pending.value.filter(c => {
  const v = props.config.voces[c.personaje]
  return v?.motor && v.motor !== 'manual' && v.voz
}))
const elevenChars = computed(() => toGenerate.value
  .filter(c => props.config.voces[c.personaje]?.motor === 'elevenlabs')
  .reduce((n, c) => n + c.caracteres, 0))

const STATE_LABEL: Record<Clip['estado'], string> = {
  'lista': 'lista', 'falta': 'falta', 'texto': 'la cita cambió', 'otra-voz': 'otra voz',
}
const STATE_COLOUR: Record<Clip['estado'], string> = {
  'lista': 'var(--ui-success)', 'falta': 'var(--color-score-faint)',
  'texto': 'var(--ui-warning)', 'otra-voz': 'var(--ui-warning)',
}

/* ---------- listening ---------- */

const playing = ref('')
let audio: HTMLAudioElement | null = null
function play(url: string) {
  audio?.pause()
  if (playing.value === url) { playing.value = ''; return }
  audio = new Audio(`${url}${url.includes('?') ? '&' : '?'}t=${Date.now()}`)
  audio.onended = () => { playing.value = '' }
  audio.play().catch(failed)
  playing.value = url
}
onBeforeUnmount(() => audio?.pause())
const clipUrl = (clip: string) => `/api/lecturas/${props.readingId}/voces/audio/${clip}`
const sampleUrl = (path: string) => path.startsWith('http') ? path : `/api/voces/archivo/${path}`

/* ---------- jobs: designing and generating ---------- */

const job = ref<Job | null>(null)
const designs = reactive<Record<string, Design>>({})
const running = computed(() => !!job.value && !job.value.terminado)
let timer: ReturnType<typeof setTimeout> | undefined

async function follow() {
  clearTimeout(timer)
  const j = await $fetch<Job | null>('/api/voces/trabajo').catch(() => null)
  job.value = j
  if (!j) return
  if (!j.terminado) { timer = setTimeout(follow, 700); return }
  if (j.error) error.value = j.error
  if (j.tipo === 'disenar' && j.personaje && j.resultado) designs[j.personaje] = j.resultado
  if (j.tipo === 'generar') {
    const errs = (j.resultado?.errores ?? []).filter((e: any) => e.error !== 'se sube a mano')
    if (errs.length) error.value = errs.map((e: any) => `${e.clip}: ${e.error}`).join(' · ')
    await loadState()
  }
}
onBeforeUnmount(() => clearTimeout(timer))

/** Jobs work on the saved reading, so unsaved changes go first. */
async function ensureSaved() {
  if (!props.dirty) return true
  const ok = await props.save()
  if (ok) await loadState()
  return ok
}

async function start(path: string, body: object) {
  error.value = ''
  if (!(await ensureSaved())) return
  try {
    job.value = await $fetch<Job>(`/api/lecturas/${props.readingId}/voces/${path}`, { method: 'POST', body })
    follow()
  } catch (e) { failed(e) }
}

function design(name: string) {
  const motor = motorOf(name)
  delete designs[name]
  start('disenar', { personaje: name, motor, descripcion: props.config.voces[name]?.descripcion ?? '' })
}

const choosing = ref('')
async function choose(name: string, candidato: string) {
  error.value = ''
  choosing.value = `${name}/${candidato}`
  try {
    const r = await $fetch<{ motor: string; voz: string; vozNombre: string }>(
      `/api/lecturas/${props.readingId}/voces/elegir`, { method: 'POST', body: { personaje: name, candidato } })
    Object.assign(entry(name), { motor: r.motor, voz: r.voz, voz_nombre: r.vozNombre })
    delete designs[name]
    if (r.motor === 'qwen') localVoices.value = null
    else elevenVoices.value = null
    await loadVoices(r.motor)
  } catch (e) { failed(e) } finally { choosing.value = '' }
}

function generate(clips?: string[]) {
  start('generar', clips ? { clips } : {})
}
async function cancel() {
  await $fetch('/api/voces/trabajo', { method: 'DELETE' }).catch(failed)
}

/* ---------- uploading a clip made elsewhere ---------- */

const uploadingClip = ref('')
async function upload(clip: string, e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  error.value = ''
  uploadingClip.value = clip
  try {
    const form = new FormData()
    form.append('file', file, file.name)
    await $fetch(`/api/lecturas/${props.readingId}/voces/subir/${clip}`, { method: 'POST', body: form })
    await loadState()
  } catch (e) { failed(e) } finally { uploadingClip.value = '' }
}

/* ---------- start ---------- */

onMounted(async () => {
  await loadEngines()
  await Promise.all([loadState(), follow(),
    ...[...new Set(props.speakers.map(motorOf))].map(loadVoices)])
})
watch(() => props.readingId, loadState)
// After «Guardar y preparar» the clips may be new or renumbered.
watch(() => props.dirty, (d, was) => { if (was && !d) loadState() })

const jobOf = (name: string) => running.value && job.value?.tipo === 'disenar' && job.value.personaje === name
const busyClip = (clip: string) => running.value && job.value?.tipo === 'generar' && job.value.paso.startsWith(clip)
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Engines -->
    <div class="rounded-[5px] border border-score-grid px-2.5 py-2 text-[12px] flex flex-col gap-1.5">
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-key-round" class="size-3.5 text-score-dim shrink-0" />
        <b>ElevenLabs</b>
        <template v-if="engines?.elevenlabs.clave && !editingKey">
          <span class="text-score-dim">clave ····{{ engines.elevenlabs.final }}</span>
          <span v-if="account" class="text-score-faint tabular">
            {{ account.usados.toLocaleString('es') }} de {{ account.limite.toLocaleString('es') }} car.
          </span>
          <div class="flex-1" />
          <UButton v-if="engines.elevenlabs.origen === 'archivo'" size="xs" variant="ghost" @click="editingKey = true">
            Cambiar
          </UButton>
        </template>
        <template v-else-if="!editingKey">
          <span class="text-score-faint">sin clave</span>
          <div class="flex-1" />
          <UButton size="xs" variant="ghost" icon="i-lucide-plus" @click="editingKey = true">Agregar clave</UButton>
        </template>
      </div>
      <div v-if="editingKey" class="flex items-center gap-1.5">
        <UInput v-model="keyDraft" type="password" size="xs" class="flex-1" placeholder="sk_…" autocomplete="off"
                @keydown.enter="saveKey(keyDraft)" />
        <UButton size="xs" :disabled="!keyDraft.trim()" @click="saveKey(keyDraft)">Guardar</UButton>
        <UButton v-if="engines?.elevenlabs.clave" size="xs" variant="ghost" color="error" @click="saveKey('')">Quitar</UButton>
        <UButton size="xs" variant="ghost" icon="i-lucide-x" @click="editingKey = false" />
      </div>
      <div v-if="editingKey" class="text-[11px] text-score-faint">
        Se guarda en <code>voces/claves.json</code> de este computador, fuera de git; la consola no la vuelve a mostrar.
      </div>
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-cpu" class="size-3.5 text-score-dim shrink-0" />
        <b>Qwen3-TTS</b>
        <span v-if="engines?.qwen.instalado" class="text-score-dim">en este computador ({{ engines.qwen.dispositivo }})</span>
        <span v-else class="text-score-faint">no instalado aquí: <code>./estudio voces</code> en el Mac</span>
      </div>
    </div>

    <UAlert v-if="error" color="error" variant="subtle" :title="error" :close="true" @update:open="error = ''" />
    <div v-if="dirty" class="text-[11px] text-score-faint">
      Hay cambios sin guardar: se guardan antes de diseñar o generar.
    </div>

    <!-- Characters -->
    <div v-for="name in speakers" :key="name" class="rounded-[5px] border border-score-grid p-2 flex flex-col gap-1.5">
      <div class="flex items-center gap-2 text-[12px]">
        <input type="color" :value="colour(name)" class="w-5 h-5 bg-transparent shrink-0"
               @input="(e: Event) => entry(name).color = (e.target as HTMLInputElement).value">
        <b class="truncate">{{ name }}</b>
        <span class="text-score-faint tabular shrink-0">
          {{ clipsOf(name).length }} cita{{ clipsOf(name).length === 1 ? '' : 's' }}
        </span>
      </div>
      <UTextarea :model-value="config.voces[name]?.descripcion ?? ''" :rows="1" autoresize size="xs" class="w-full"
                 placeholder="cómo suena: edad, oficio, ánimo, acento"
                 @update:model-value="(v: string) => entry(name).descripcion = v" />

      <div class="flex items-center gap-1.5">
        <USelect :model-value="motorOf(name)" :items="MOTORS" size="xs" class="w-[150px] shrink-0"
                 @update:model-value="(v: string) => setMotor(name, v)" />
        <template v-if="motorOf(name) !== 'manual'">
          <USelectMenu :model-value="config.voces[name]?.voz" :items="voiceItems(name)" value-key="value"
                       size="xs" class="flex-1 min-w-0" placeholder="elegir una voz…"
                       :disabled="motorOf(name) === 'elevenlabs' && !engines?.elevenlabs.clave"
                       @update:model-value="(v: string) => setVoice(name, v)" />
          <UButton size="xs" variant="soft" icon="i-lucide-wand-sparkles" :loading="jobOf(name)"
                   :disabled="running || !config.voces[name]?.descripcion
                     || (motorOf(name) === 'elevenlabs' && !engines?.elevenlabs.clave)"
                   :title="config.voces[name]?.descripcion ? 'Tres voces candidatas a partir de la descripción' : 'Escribe primero cómo suena'"
                   @click="design(name)">
            Diseñar
          </UButton>
        </template>
      </div>
      <div v-if="motorOf(name) !== 'manual' && config.voces[name]?.voz && !config.voces[name]?.voz_nombre"
           class="text-[11px] text-score-faint truncate">voz {{ config.voces[name].voz }}</div>

      <!-- The three candidates -->
      <div v-if="jobOf(name)" class="text-[11px] text-score-dim flex items-center gap-1.5">
        <UIcon name="i-lucide-loader-circle" class="size-3 animate-spin" /> {{ job?.paso }}
      </div>
      <div v-if="designs[name]" class="rounded-[4px] bg-score-stave p-1.5 flex flex-col gap-1">
        <div class="text-[11px] text-score-faint">Escucha y quédate con una. Dice: «{{ designs[name].texto.slice(0, 90) }}…»</div>
        <div v-for="c in designs[name].candidatas" :key="c.id" class="flex items-center gap-1.5 text-[12px]">
          <UButton size="xs" variant="ghost"
                   :icon="playing === sampleUrl(`${designs[name].carpeta}/${c.archivo}`) ? 'i-lucide-square' : 'i-lucide-play'"
                   @click="play(sampleUrl(`${designs[name].carpeta}/${c.archivo}`))" />
          <span>Candidata {{ designs[name].candidatas.indexOf(c) + 1 }}</span>
          <span class="text-score-faint tabular">{{ c.dur }} s</span>
          <div class="flex-1" />
          <UButton size="xs" variant="soft" :loading="choosing === `${name}/${c.id}`" @click="choose(name, c.id)">
            Usar esta
          </UButton>
        </div>
      </div>

      <!-- Its clips -->
      <div v-for="c in clipsOf(name)" :key="c.clip" class="flex items-center gap-1.5 text-[11.5px]">
        <span class="w-2 h-2 rounded-full shrink-0" :style="{ background: STATE_COLOUR[c.estado] }"
              :title="STATE_LABEL[c.estado]" />
        <span class="tabular text-score-dim w-5 shrink-0">{{ c.clip.slice(0, 2) }}</span>
        <span class="flex-1 min-w-0 truncate text-score-dim" :title="c.texto">{{ c.texto }}</span>
        <span v-if="busyClip(c.clip)" class="shrink-0"><UIcon name="i-lucide-loader-circle" class="size-3 animate-spin" /></span>
        <span v-else-if="c.estado !== 'lista'" class="text-[10.5px] shrink-0" :style="{ color: STATE_COLOUR[c.estado] }">
          {{ STATE_LABEL[c.estado] }}
        </span>
        <span v-else class="tabular text-score-faint shrink-0">{{ c.dur ? `${c.dur.toFixed(1)} s` : '' }}</span>
        <UButton v-if="c.estado !== 'falta'" size="xs" variant="ghost" class="shrink-0"
                 :icon="playing === clipUrl(c.clip) ? 'i-lucide-square' : 'i-lucide-play'"
                 title="Escuchar" @click="play(clipUrl(c.clip))" />
        <UButton v-if="motorOf(name) !== 'manual' && config.voces[name]?.voz" size="xs" variant="ghost" class="shrink-0"
                 icon="i-lucide-refresh-cw" :disabled="running" title="Rehacer este clip" @click="generate([c.clip])" />
        <label class="shrink-0 cursor-pointer text-score-dim hover:text-white px-1" title="Subir un audio propio">
          <UIcon :name="uploadingClip === c.clip ? 'i-lucide-loader-circle' : 'i-lucide-upload'"
                 class="size-3.5" :class="uploadingClip === c.clip && 'animate-spin'" />
          <input type="file" accept="audio/*" class="hidden" :disabled="!!uploadingClip" @change="upload(c.clip, $event)">
        </label>
      </div>
      <div v-if="!clipsOf(name).length" class="text-[11px] text-score-faint">
        Guarda para ver sus clips.
      </div>
    </div>

    <!-- Generating -->
    <div v-if="state?.clips.length" class="flex flex-col gap-1.5">
      <div v-if="running && job?.tipo === 'generar'" class="flex flex-col gap-1">
        <div class="flex items-center gap-2 text-[11.5px] text-score-dim">
          <UIcon name="i-lucide-loader-circle" class="size-3.5 animate-spin" />
          <span class="flex-1 truncate">{{ job.paso }}</span>
          <UButton size="xs" variant="ghost" color="error" @click="cancel">Cancelar</UButton>
        </div>
        <UProgress :model-value="job.total ? (job.hecho / job.total) * 100 : null" size="xs" color="neutral" />
      </div>
      <UButton v-else block :disabled="running || !toGenerate.length" icon="i-lucide-audio-lines" @click="generate()">
        <template v-if="toGenerate.length">
          Generar {{ toGenerate.length }} clip{{ toGenerate.length === 1 ? '' : 's' }}
          <span v-if="elevenChars" class="opacity-70">· {{ elevenChars.toLocaleString('es') }} car. de ElevenLabs</span>
        </template>
        <template v-else-if="pending.length">Faltan {{ pending.length }}: elige voz o sube el audio</template>
        <template v-else>Todas las citas tienen voz</template>
      </UButton>
    </div>
  </div>
</template>
