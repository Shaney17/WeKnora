// Wiki-taxonomy extension: API types and client. Kept out of api/knowledge-base
// so the core API module stays untouched. See docs/extensions/wiki-taxonomy.md.
import { get } from '@/utils/request'

// Wiki directory template. Mirrors backend types.WikiTaxonomy (snake_case JSON).
// A folder is "closed" unless `open` is set: ingest files pages into closed
// folders but never adds folders under them.
export interface WikiTaxonomyNode {
  name: string
  description?: string
  children?: WikiTaxonomyNode[]
  open?: boolean
}

export interface WikiTaxonomy {
  roots: WikiTaxonomyNode[]
  summary_path?: string[]
  fallback_path?: string[]
}

export interface WikiTaxonomyPreset {
  id: string
  taxonomy: WikiTaxonomy
}

// Built-in directory templates the editor offers as starting points. The user
// copies one into the KB and edits it; the saved copy is what ingest follows.
export async function fetchWikiTaxonomyPresets(): Promise<WikiTaxonomyPreset[]> {
  const res = await get<{ success: boolean; data: { presets: WikiTaxonomyPreset[] } }>(
    '/api/v1/wiki-taxonomy/presets',
  )
  return res.data?.presets ?? []
}
