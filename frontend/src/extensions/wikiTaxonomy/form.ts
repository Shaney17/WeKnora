// Wiki-taxonomy extension: the pure form-state helpers behind the KB editor
// hooks. No Vue, no network: they are unit-tested on their own, and index.ts
// wires in the real template fetcher. See docs/extensions/wiki-taxonomy.md.
import type { WikiTaxonomy, WikiTaxonomyPreset } from './api'

/** The extension's slice of the form's wikiConfig. */
export interface WikiTaxonomyFormState {
  taxonomyPreset: string
  taxonomy: WikiTaxonomy | undefined
}

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

/** Initial form state: free-form, no layout. */
export function wikiTaxonomyDefaults(): WikiTaxonomyFormState {
  return { taxonomyPreset: '', taxonomy: undefined }
}

/**
 * Form state from a KB's stored `wiki_config`. A layout without folders means
 * free-form, so it is shown as such.
 */
export function wikiTaxonomyFromKb(wikiConfig?: {
  taxonomy_preset?: string
  taxonomy?: WikiTaxonomy | null
}): WikiTaxonomyFormState {
  const taxonomy = wikiConfig?.taxonomy
  return {
    taxonomyPreset: wikiConfig?.taxonomy_preset || '',
    taxonomy: taxonomy && taxonomy.roots?.length ? clone(taxonomy) : undefined,
  }
}

/**
 * Fields to spread into the `wiki_config` of a create/update request. Free-form
 * sends an empty layout rather than nothing: the backend keeps the stored layout
 * when a request omits it (so clients that never heard of this extension cannot
 * wipe it), which means clearing has to be explicit.
 */
export function wikiTaxonomyPayload(state?: Partial<WikiTaxonomyFormState> | null) {
  return {
    taxonomy_preset: state?.taxonomyPreset || '',
    taxonomy: state?.taxonomy ?? { roots: [] },
  }
}

/**
 * A KB whose layout was set by template name through the API becomes an editable
 * copy of that template, so the user sees exactly what ingest will follow. If the
 * templates cannot be fetched the name is left alone and sent back unchanged.
 */
export async function adoptLegacyTaxonomyPreset(
  state: Partial<WikiTaxonomyFormState> | null | undefined,
  fetchPresets: () => Promise<WikiTaxonomyPreset[]>,
) {
  if (!state || state.taxonomy || !state.taxonomyPreset) return
  try {
    const hit = (await fetchPresets()).find((p) => p.id === state.taxonomyPreset)
    if (hit) {
      state.taxonomy = clone(hit.taxonomy)
      state.taxonomyPreset = ''
    }
  } catch (error) {
    console.warn('Failed to load wiki folder templates:', error)
  }
}
