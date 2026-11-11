<script setup lang="ts">
/**
 * Everything about capturing a take, in the order you actually need it: grant
 * the microphone, see which one is live and how loud you are, then the
 * transport.
 *
 * The permission is asked for *here*, in the focused window, even though the
 * presentation window is the one that records. A prompt raised by a window
 * sitting behind this one is a prompt nobody sees.
 */
const transport = useTransport()
const mic = useMicrophone()
const { state, connected, level, dbfs, recording, paused, armed, busy, uploading } = transport

const selectedDevice = computed({
  get: () => state.value.deviceId ?? undefined,
  set: (id: string | undefined) => transport.armMic(id ?? null),
})

const deviceOptions = computed(() =>
  mic.devices.value.map(d => ({ label: d.label, value: d.id })),
)

const currentLabel = computed(() =>
  mic.devices.value.find(d => d.id === state.value.deviceId)?.label ?? null,
)

/** Ask here, then hand the chosen device to the window that records. */
async function open() {
  const ok = await mic.request()
  if (ok) transport.armMic(mic.devices.value[0]?.id ?? null)
}

const verdict = computed(() => {
  if (!armed.value) return { label: 'sin abrir', tone: 'text-score-faint' }
  if (state.value.muted) return { label: 'silenciado', tone: 'text-warning' }
  if (level.value.peak >= 0.99) return { label: 'satura', tone: 'text-[var(--color-voice-zenon)]' }
  if (dbfs.value > -6) return { label: 'muy alto', tone: 'text-warning' }
  if (dbfs.value < -30) return { label: 'muy bajo', tone: 'text-warning' }
  return { label: 'holgado', tone: 'text-[var(--color-voice-glaucon)]' }
})

const HISTORY = 56
const history = ref<number[]>(Array(HISTORY).fill(0))
watch(() => level.value.rms, (v) => {
  history.value = [...history.value.slice(1), v]
})
</script>

<template>
  <div class="shrink-0 rounded-lg border border-score-grid bg-score-stave overflow-hidden">
    <!-- microphone -->
    <div class="p-3 border-b border-score-grid">
      <div class="flex items-center justify-between text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
        <span class="flex items-center gap-1.5"><UIcon name="i-lucide-mic" class="size-3" />Micrófono</span>
        <span :class="verdict.tone">{{ verdict.label }}</span>
      </div>

      <template v-if="!armed">
        <UButton
          block
          size="sm"
          variant="subtle"
          icon="i-lucide-mic"
          :loading="mic.asking.value"
          :disabled="!connected || mic.asking.value"
          @click="open"
        >Abrir el micrófono</UButton>

        <p v-if="mic.problem.value" class="flex gap-1.5 text-[11px] text-[var(--color-voice-zenon)] mt-2">
          <UIcon name="i-lucide-triangle-alert" class="size-3.5 shrink-0 mt-0.5" />
          <span>{{ mic.problem.value }}</span>
        </p>
        <p v-else-if="!connected" class="text-[11px] text-score-faint mt-2">
          Abre primero la ventana de presentación.
        </p>
        <p v-else class="text-[11px] text-score-faint mt-2">
          Chrome pedirá el permiso en esta ventana.
        </p>
      </template>

      <template v-else>
        <div class="flex items-center gap-1.5">
          <USelectMenu
            v-model="selectedDevice"
            :items="deviceOptions"
            value-key="value"
            size="sm"
            class="flex-1 min-w-0"
            :disabled="busy"
            :placeholder="currentLabel || 'Elegir micrófono'"
          />
          <UButton
            :icon="state.muted ? 'i-lucide-mic-off' : 'i-lucide-mic'"
            :color="state.muted ? 'warning' : 'neutral'"
            variant="subtle"
            size="sm"
            :title="state.muted ? 'Reactivar el micrófono (M)' : 'Silenciar el micrófono (M)'"
            @click="transport.toggleMute()"
          />
        </div>

        <div class="flex items-end gap-px h-7 mt-2.5">
          <span
            v-for="(v, i) in history"
            :key="i"
            class="flex-1 rounded-[1px] transition-[height] duration-75"
            :class="v > 0.5 ? 'bg-[var(--color-voice-zenon)]' : 'bg-[var(--color-voice-glaucon)]'"
            :style="{ height: `${Math.max(2, Math.min(1, v * 3) * 100)}%`, opacity: state.muted ? 0.2 : 1 }"
          />
        </div>
        <div class="flex justify-between text-[9px] tabular text-score-faint mt-1">
          <span>−40</span>
          <span class="text-score-dim">{{ dbfs > -98 ? `${dbfs.toFixed(0)} dB` : '—' }}</span>
          <span>0</span>
        </div>

        <p v-if="state.muted" class="text-[11px] text-warning mt-2">
          Silenciado: la grabación sigue, pero graba silencio.
        </p>
      </template>
    </div>

    <!-- transport -->
    <div class="p-3">
      <div class="flex items-center justify-between text-[9.5px] uppercase tracking-[0.12em] text-score-faint mb-2">
        <span class="flex items-center gap-1.5"><UIcon name="i-lucide-video" class="size-3" />Toma</span>
        <span v-if="busy" class="tabular" :class="paused ? 'text-warning' : 'text-[var(--color-voice-zenon)]'">
          {{ paused ? '⏸' : '●' }} {{ clock(state.recorded) }}
        </span>
      </div>

      <div v-if="!busy" class="grid gap-1.5">
        <!-- Its own red, not the error colour. Recording is not a failure, and
             the error token is tuned to be read as text on a near-black page,
             which makes a washed-out fill. This is the studio's red, the same
             one the counter and the cuts use. -->
        <UButton
          block
          color="neutral"
          variant="solid"
          icon="i-lucide-circle"
          class="!text-score-bg font-semibold"
          :style="{ background: 'var(--color-voice-zenon)' }"
          :loading="uploading"
          :disabled="!connected || !armed || uploading"
          @click="transport.startTake()"
        >
          Grabar
          <template #trailing><UKbd value="R" /></template>
        </UButton>
        <p v-if="!armed" class="text-[11px] text-score-faint">
          Abre el micrófono primero, así ves tu nivel antes de comprometerte.
        </p>
        <p v-else class="text-[11px] text-score-faint">
          Chrome preguntará qué compartir: elige la pestaña de la presentación.
        </p>
      </div>

      <div v-else class="grid grid-cols-2 gap-1.5">
        <UButton
          v-if="recording"
          block
          variant="subtle"
          icon="i-lucide-pause"
          @click="transport.pauseTake()"
        >Pausar</UButton>
        <UButton
          v-else
          block
          variant="solid"
          color="neutral"
          icon="i-lucide-play"
          @click="transport.resumeTake()"
        >Reanudar</UButton>

        <UButton
          block
          color="error"
          variant="subtle"
          icon="i-lucide-square"
          @click="transport.stopTake()"
        >
          Terminar
          <template #trailing><UKbd value="S" /></template>
        </UButton>

        <p v-if="paused" class="col-span-2 text-[11px] text-warning">
          En pausa. El texto también se detuvo: al reanudar sigues donde lo dejaste.
        </p>
      </div>
    </div>
  </div>
</template>
