<script setup lang="ts">
/**
 * The background track (lineamiento 2.j: «música de fondo con un volumen
 * apropiado para que no nuble la voz de quien lee»). Optional. The tracks
 * live in the studio's musica/ folder, shared by every reading and kept out
 * of git; the reading names one in lectura.toml's [musica], with how far under
 * the voice it sits. The render lays it under the whole video, looped, with a
 * fade at each end.
 */
interface Track { nombre: string; bytes: number; dur: number }

const props = defineProps<{ config: any }>()

const tracks = ref<Track[]>([])
const error = ref('')
const uploading = ref(false)

async function load() {
  tracks.value = await $fetch<Track[]>('/api/musica').catch(() => [])
}
onMounted(load)

const music = computed(() => props.config.musica ?? {})
const NONE = '—'
const items = computed(() => [
  { label: 'Sin música', value: NONE },
  ...tracks.value.map(t => ({ label: t.nombre, value: t.nombre, description: clock(t.dur) })),
])

function choose(name: string) {
  if (name === NONE) {
    delete props.config.musica
    return
  }
  props.config.musica = { nivel: 14, atenuar: true, ...props.config.musica, archivo: name }
}
function set(key: string, value: unknown) {
  props.config.musica = { ...props.config.musica, [key]: value }
}

async function upload(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  uploading.value = true
  error.value = ''
  try {
    const form = new FormData()
    form.append('file', file, file.name)
    const t = await $fetch<Track>('/api/musica', { method: 'POST', body: form })
    await load()
    choose(t.nombre)
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
  } finally {
    uploading.value = false
  }
}

const playing = ref(false)
let audio: HTMLAudioElement | null = null
function preview() {
  if (playing.value) {
    audio?.pause()
    playing.value = false
    return
  }
  audio = new Audio(`/api/musica/${encodeURIComponent(music.value.archivo)}`)
  audio.volume = 0.5
  audio.onended = () => { playing.value = false }
  audio.play().catch((e) => { error.value = String(e) })
  playing.value = true
}
onBeforeUnmount(() => audio?.pause())
watch(() => music.value.archivo, () => { audio?.pause(); playing.value = false })

const clock = (s: number) => `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}`
</script>

<template>
  <div class="flex flex-col gap-2 text-[12px]">
    <div class="flex items-center gap-1.5">
      <USelectMenu :model-value="music.archivo ?? NONE" :items="items" value-key="value" size="xs"
                   class="flex-1 min-w-0" @update:model-value="choose" />
      <UButton v-if="music.archivo" size="xs" variant="ghost"
               :icon="playing ? 'i-lucide-square' : 'i-lucide-play'" title="Escuchar" @click="preview" />
      <label class="shrink-0 cursor-pointer text-score-dim hover:text-white px-1" title="Subir una pista">
        <UIcon :name="uploading ? 'i-lucide-loader-circle' : 'i-lucide-upload'" class="size-4"
               :class="uploading && 'animate-spin'" />
        <input type="file" accept="audio/*" class="hidden" :disabled="uploading" @change="upload">
      </label>
    </div>
    <template v-if="music.archivo">
      <div class="flex items-center gap-2">
        <span class="text-score-dim w-[92px] shrink-0">Bajo la voz</span>
        <USlider :model-value="music.nivel ?? 14" :min="8" :max="28" :step="1" size="xs" class="flex-1"
                 @update:model-value="(v: number | number[] | undefined) => set('nivel', Array.isArray(v) ? v[0] : v)" />
        <span class="tabular w-[44px] text-right">{{ music.nivel ?? 14 }} dB</span>
      </div>
      <label class="flex items-center gap-2">
        <USwitch :model-value="music.atenuar ?? true" size="xs" @update:model-value="(v: boolean) => set('atenuar', v)" />
        <span class="text-score-dim">Bajarla un poco más mientras se lee</span>
      </label>
      <div class="text-[11px] text-score-faint">
        Se repite hasta el final, entra y sale con un fundido. 14 dB se oye detrás de la voz sin taparla; más la aleja, menos la acerca.
      </div>
    </template>
    <div v-else class="text-[11px] text-score-faint">
      Opcional (2.j). Las pistas quedan en <code>musica/</code>, fuera de git, para todas las lecturas.
    </div>
    <UAlert v-if="error" color="error" variant="subtle" :title="error" />
  </div>
</template>
