<script setup lang="ts">
/**
 * The render scope. ffmpeg reports its own progress, which the studio streams
 * as server-sent events, so this never polls and never opens a modal — the job
 * runs while the rest of the console stays usable.
 */
const studio = useStudio()
const { takes, keptSeconds, overBudget } = studio

interface Job {
  id: string
  state: 'queued' | 'running' | 'done' | 'failed'
  progress: number
  step: string
  error?: string
  duration?: number
}

const job = ref<Job | null>(null)
const starting = ref(false)
let stream: EventSource | null = null

onUnmounted(() => stream?.close())

async function start() {
  starting.value = true
  try {
    const started = await $fetch<Job>('/api/renders', { method: 'POST' })
    job.value = started
    watchJob(started.id)
  } catch (e: any) {
    job.value = {
      id: '', state: 'failed', progress: 0, step: '',
      error: e?.data?.error ?? e?.message ?? 'no pude arrancar el montaje',
    }
  } finally {
    starting.value = false
  }
}

function watchJob(id: string) {
  stream?.close()
  stream = new EventSource(`/api/renders/${id}/events`)
  stream.onmessage = (e) => {
    job.value = JSON.parse(e.data)
    if (job.value?.state === 'done' || job.value?.state === 'failed') stream?.close()
  }
  stream.onerror = () => stream?.close()
}

const running = computed(() => job.value?.state === 'queued' || job.value?.state === 'running')
</script>

<template>
  <div class="flex flex-col gap-4 min-h-0 overflow-auto">
    <div class="rounded-lg border border-score-grid bg-score-stave p-4">
      <div class="text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-3">Lo que se va a montar</div>

      <div class="grid grid-cols-4 gap-4">
        <div>
          <div class="text-[22px] font-medium tabular">{{ takes.length }}</div>
          <div class="text-[9px] uppercase tracking-[0.1em] text-score-faint">tomas</div>
        </div>
        <div>
          <div class="text-[22px] font-medium tabular" :class="overBudget ? 'text-warning' : ''">
            {{ clock(keptSeconds) }}
          </div>
          <div class="text-[9px] uppercase tracking-[0.1em] text-score-faint">tras cortes</div>
        </div>
        <div>
          <div class="text-[22px] font-medium tabular">
            {{ takes.reduce((s, t) => s + (t.cues?.length ?? 0), 0) }}
          </div>
          <div class="text-[9px] uppercase tracking-[0.1em] text-score-faint">citas colocadas</div>
        </div>
        <div>
          <div class="text-[22px] font-medium tabular">
            {{ takes.reduce((s, t) => s + (t.cuts?.length ?? 0), 0) }}
          </div>
          <div class="text-[9px] uppercase tracking-[0.1em] text-score-faint">cortes</div>
        </div>
      </div>

      <UAlert
        v-if="overBudget"
        color="warning"
        variant="subtle"
        class="mt-4"
        title="Se pasa del rango"
        :description="`El montaje dura ${clock(keptSeconds)} y el lineamiento 2.m pide entre 05 y 25 min.`"
      />
    </div>

    <div class="rounded-lg border border-score-grid bg-score-stave p-4">
      <div class="flex items-center gap-3">
        <UButton
          color="neutral"
          variant="solid"
          icon="i-lucide-clapperboard"
          :loading="starting || running"
          :disabled="!takes.length || running"
          @click="start"
        >Montar el video</UButton>

        <span v-if="job && running" class="text-[12px] text-score-dim">
          {{ job.step || job.state }} · {{ (job.progress * 100).toFixed(0) }}%
        </span>

        <a
          v-if="job?.state === 'done'"
          :href="`/api/renders/${job.id}/output`"
          class="flex items-center gap-1.5 text-[12px] text-signal hover:underline ml-auto"
          download
        >
          <UIcon name="i-lucide-download" class="size-3.5" />
          Descargar final.mp4 ({{ clock(job.duration ?? 0) }})
        </a>
      </div>

      <UProgress
        v-if="job && running"
        :model-value="job.progress * 100"
        color="neutral"
        size="sm"
        class="mt-3"
      />

      <UAlert
        v-if="job?.state === 'failed'"
        color="error"
        variant="subtle"
        class="mt-3"
        title="El montaje falló"
        :description="job.error"
      />

      <p v-if="!takes.length" class="text-[11px] text-score-faint mt-3">
        No hay nada que montar todavía.
      </p>
    </div>
  </div>
</template>
