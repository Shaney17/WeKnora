// Wiki-taxonomy extension: the surface the KB editor modal touches.
//
// The modal carries one-line hooks marked "[wiki-taxonomy-ext]" that call the
// helpers below; everything else (UI, API, messages) lives in this folder.
// See docs/extensions/wiki-taxonomy.md.
import { fetchWikiTaxonomyPresets } from './api'
import { adoptLegacyTaxonomyPreset as adopt, type WikiTaxonomyFormState } from './form'
import { registerWikiTaxonomyMessages } from './locales'

export { default as WikiTaxonomySettings } from './WikiTaxonomySettings.vue'
export { wikiTaxonomyDefaults, wikiTaxonomyFromKb, wikiTaxonomyPayload } from './form'
export type { WikiTaxonomyFormState } from './form'
export type { WikiTaxonomy } from './api'

// Messages must exist before the first render, so register on import.
registerWikiTaxonomyMessages()

/** See form.adoptLegacyTaxonomyPreset; this binds the real template fetcher. */
export function adoptLegacyTaxonomyPreset(state?: Partial<WikiTaxonomyFormState> | null) {
  return adopt(state, fetchWikiTaxonomyPresets)
}
