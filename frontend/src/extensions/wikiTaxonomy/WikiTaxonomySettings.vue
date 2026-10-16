<template>
  <div class="wiki-taxonomy-settings">
    <label class="wts-label">{{ $t('extensions.wikiTaxonomy.label') }}</label>
    <p class="wts-tip">{{ $t('extensions.wikiTaxonomy.tip') }}</p>
    <t-radio-group v-model="mode" variant="default-filled">
      <t-radio-button value="free">
        {{ $t('extensions.wikiTaxonomy.modeFree') }}
      </t-radio-button>
      <t-radio-button value="fixed">
        {{ $t('extensions.wikiTaxonomy.modeFixed') }}
      </t-radio-button>
    </t-radio-group>
    <WikiTaxonomyEditor v-if="mode === 'fixed'" v-model="wikiConfig.taxonomy" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import WikiTaxonomyEditor from './WikiTaxonomyEditor.vue'
import type { WikiTaxonomyFormState } from './form'
import type { WikiTaxonomy } from './api'

// The form's wikiConfig object. It is the modal's own form state, so this
// component edits its taxonomy fields in place rather than emitting a copy.
const props = defineProps<{ wikiConfig: Partial<WikiTaxonomyFormState> }>()

// "free" lets ingest invent folders as before, "fixed" makes ingest follow the
// tree edited here and never add folders to it. Switching back to free keeps the
// edited tree aside, so a misclick loses nothing.
const stash = ref<WikiTaxonomy | undefined>()
const mode = computed<'free' | 'fixed'>({
  get: () => (props.wikiConfig.taxonomy ? 'fixed' : 'free'),
  set: (next) => {
    if (next === 'fixed') {
      props.wikiConfig.taxonomy = stash.value ?? { roots: [] }
    } else {
      stash.value = props.wikiConfig.taxonomy
      props.wikiConfig.taxonomy = undefined
      props.wikiConfig.taxonomyPreset = ''
    }
  },
})
</script>

<style scoped lang="less">
// Mirrors the KB editor's .form-label / .form-tip, which are scoped to the modal
// and so do not reach into this component.
.wts-label {
  display: block;
  margin-bottom: 8px;
  font-family: var(--app-font-family);
  font-size: var(--app-text-lg);
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.wts-tip {
  margin: 0 0 8px;
  font-size: var(--app-text-sm);
  color: var(--td-text-color-placeholder);
}
</style>
