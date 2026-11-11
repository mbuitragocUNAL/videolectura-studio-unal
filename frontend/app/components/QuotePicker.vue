<script setup lang="ts">
/**
 * The nineteen quotes, and which one `N` will sound.
 *
 * A quote lives in a paragraph and a take is a paragraph, so choosing the
 * quote and choosing the paragraph are the same act — but the dialogues make
 * that hard to see from the passage alone: five consecutive turns of Sócrates
 * and Glaucón are five paragraphs that look alike in a list of seventy-nine.
 * Here they are as themselves, with the character's colour and whether they
 * have been recorded yet, so picking one is aiming rather than counting.
 */
const studio = useStudio()
const transport = useTransport()
const { quotes, firedQuotes } = studio

const emit = defineEmits<{ select: [paragraphIndex: number] }>()

/** Each quote with the paragraph it belongs to, so choosing one can go there. */
const rows = computed(() =>
  quotes.value.map((quote) => {
    const paragraph = studio.paragraphs.value.findIndex(
      p => quote.linea >= (studio.lines.value[p.from]?.n ?? 0)
        && quote.linea <= (studio.lines.value[p.to]?.n ?? 0),
    )
    return {
      quote,
      paragraph,
      fired: firedQuotes.value.has(quote.clip),
      take: paragraph >= 0 ? studio.takeOfParagraph(paragraph) : null,
    }
  }),
)

const current = computed(() => transport.currentQuote.value)
const done = computed(() => rows.value.filter(r => r.fired).length)

const list = useTemplateRef<HTMLElement>('list')

watch(current, async () => {
  await nextTick()
  list.value?.querySelector('[data-current="true"]')
    ?.scrollIntoView({ block: 'nearest' })
})
</script>

<template>
  <div class="shrink-0 rounded-lg border border-score-grid bg-score-stave overflow-hidden">
    <div class="flex items-baseline gap-2 px-3 pt-2.5 pb-1.5 text-[9.5px] uppercase tracking-[0.12em] text-score-faint">
      <UIcon name="i-lucide-quote" class="size-3" />
      <span>Citas</span>
      <span class="tabular text-score-dim">{{ done }} de {{ rows.length }}</span>
    </div>

    <div ref="list" class="max-h-[188px] overflow-y-auto px-1.5 pb-1.5">
      <button
        v-for="row in rows"
        :key="row.quote.clip"
        :data-current="current?.clip === row.quote.clip"
        class="w-full flex items-center gap-2 px-1.5 py-1 rounded-[4px] text-left transition-colors"
        :class="current?.clip === row.quote.clip
          ? 'bg-white/[0.08]'
          : 'hover:bg-white/[0.04]'"
        :disabled="row.paragraph < 0"
        :title="row.quote.texto"
        @click="row.paragraph >= 0 && emit('select', row.paragraph)"
      >
        <span
          class="w-[18px] shrink-0 text-center text-[9px] font-bold rounded-[3px] text-score-bg tabular"
          :style="{ background: studio.colourOf(row.quote.personaje) }"
        >{{ row.quote.clip.slice(0, 2) }}</span>

        <span
          class="flex-1 min-w-0 truncate text-[11.5px]"
          :class="current?.clip === row.quote.clip ? 'text-white' : 'text-score-dim'"
        >{{ row.quote.personaje }}</span>

        <span class="tabular text-[10px] text-score-faint shrink-0">
          ¶{{ row.paragraph + 1 }} · {{ row.quote.dur.toFixed(1) }}s
        </span>

        <UIcon
          v-if="row.fired"
          name="i-lucide-check"
          class="size-3 shrink-0 text-[var(--color-voice-glaucon)]"
          title="ya suena dentro de una toma"
        />
        <span v-else class="w-3 shrink-0" />
      </button>
    </div>

    <div v-if="current" class="px-3 py-2 border-t border-score-grid">
      <p class="text-[11px] text-score-dim line-clamp-3">{{ current.texto }}</p>
    </div>
  </div>
</template>
