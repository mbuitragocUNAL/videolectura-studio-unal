<script setup lang="ts">
/**
 * Every reading in lecturas/, and the way into one: record it, change how it
 * is prepared, or start a new one from a book.
 */
interface Entry {
  id: string
  numero: number
  titulo: string
  paginas: string
  preparada: boolean
  activa: boolean
  tomas: number
}

const lista = ref<Entry[]>([])
const carpetaActiva = ref('')
const error = ref('')
const abriendo = ref('')

async function cargar() {
  try {
    const r = await $fetch<{ lecturas: Entry[]; carpetaActiva: string }>('/api/lecturas')
    lista.value = r.lecturas
    carpetaActiva.value = r.carpetaActiva
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
  }
}
onMounted(cargar)

async function abrir(id: string) {
  abriendo.value = id
  error.value = ''
  try {
    await $fetch('/api/lecturas/activa', { method: 'POST', body: { id } })
    // The console holds the old reading in memory; a full load starts clean.
    window.location.href = '/'
  } catch (e: any) {
    error.value = e?.data?.error ?? e?.message ?? String(e)
    abriendo.value = ''
  }
}

const enLaLista = computed(() => lista.value.some(l => l.activa))
</script>

<template>
  <div class="min-h-screen px-8 py-8 max-w-[880px] mx-auto">
    <div class="flex items-center gap-3 mb-8">
      <NuxtLink to="/" class="text-score-dim hover:text-white text-sm flex items-center gap-1">
        <UIcon name="i-lucide-arrow-left" class="size-4" /> Estudio
      </NuxtLink>
      <div class="flex-1" />
      <UButton to="/preparar" icon="i-lucide-plus" color="primary" variant="solid">Nueva lectura</UButton>
    </div>

    <h1 class="text-2xl font-semibold mb-1">Lecturas</h1>
    <p class="text-score-dim text-sm mb-6">
      Cada lectura es una carpeta en <code>lecturas/</code> con su <code>lectura.toml</code>, sus
      páginas y sus tomas.
      <template v-if="!enLaLista && carpetaActiva">
        Ahora se graba <b class="text-white">{{ carpetaActiva }}</b>, que está fuera de esa carpeta.
      </template>
    </p>

    <UAlert v-if="error" color="error" variant="subtle" :title="error" class="mb-4" />

    <div v-if="!lista.length && !error" class="text-score-dim text-sm py-10 text-center border border-dashed border-score-edge rounded-md">
      Todavía no hay lecturas. Empieza con «Nueva lectura».
    </div>

    <div class="flex flex-col gap-2">
      <div
        v-for="l in lista"
        :key="l.id"
        class="flex items-center gap-4 px-4 py-3 rounded-md border"
        :class="l.activa ? 'border-signal/50 bg-score-stave' : 'border-score-grid hover:border-score-edge'"
      >
        <div class="text-2xl font-semibold tabular w-12" :class="l.activa ? 'text-signal' : 'text-score-dim'">
          {{ String(l.numero).padStart(2, '0') }}
        </div>
        <div class="flex-1 min-w-0">
          <div class="font-medium truncate">{{ l.titulo || l.id }}</div>
          <div class="text-[12px] text-score-dim">
            páginas {{ l.paginas || '—' }} ·
            {{ l.preparada ? `${l.tomas} toma${l.tomas === 1 ? '' : 's'}` : 'sin preparar' }}
            <span v-if="l.activa"> · en el estudio ahora</span>
          </div>
        </div>
        <UButton :to="`/preparar?id=${l.id}`" variant="ghost" icon="i-lucide-sliders-horizontal">Preparar</UButton>
        <UButton
          :disabled="!l.preparada"
          :loading="abriendo === l.id"
          :variant="l.activa ? 'subtle' : 'solid'"
          icon="i-lucide-mic"
          @click="l.activa ? navigateTo('/') : abrir(l.id)"
        >{{ l.activa ? 'Volver a grabar' : 'Grabar' }}</UButton>
      </div>
    </div>
  </div>
</template>
