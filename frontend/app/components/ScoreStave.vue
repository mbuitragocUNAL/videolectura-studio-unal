<script setup lang="ts">
/**
 * One stave of the score: a clef on the left naming the line, and a lane that
 * shares its x-axis with every other stave. Nothing here knows what it holds —
 * the caller draws into the lane.
 */
defineProps<{
  name: string
  meta?: string
  height: number
  /** A ruler has no lane background; it is drawn on the page itself. */
  bare?: boolean
}>()
</script>

<template>
  <div class="flex items-stretch mb-[3px]">
    <div class="w-[104px] shrink-0 pr-3 flex flex-col justify-center border-r border-score-grid">
      <div class="text-[9.5px] font-semibold uppercase tracking-[0.13em] text-score-dim leading-tight">
        {{ name }}
      </div>
      <div v-if="meta" class="text-[9px] text-score-faint tabular mt-0.5 leading-tight">
        {{ meta }}
      </div>
    </div>

    <div
      class="flex-1 relative rounded-[2px]"
      :class="bare ? '' : 'bg-score-stave'"
      :style="{ height: `${height}px` }"
    >
      <slot />
    </div>
  </div>
</template>
