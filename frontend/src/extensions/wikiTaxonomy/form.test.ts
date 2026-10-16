import assert from 'node:assert/strict'
import test from 'node:test'
import {
  adoptLegacyTaxonomyPreset,
  wikiTaxonomyDefaults,
  wikiTaxonomyFromKb,
  wikiTaxonomyPayload,
} from './form.ts'

const tree = { roots: [{ name: 'Product', open: true }, { name: 'Sources' }], summary_path: ['Sources'] }

test('a new form starts free-form', () => {
  assert.deepEqual(wikiTaxonomyDefaults(), { taxonomyPreset: '', taxonomy: undefined })
})

test('a stored layout is loaded as an independent copy', () => {
  const stored = { taxonomy_preset: '', taxonomy: tree }
  const state = wikiTaxonomyFromKb(stored)
  assert.deepEqual(state.taxonomy, tree)
  state.taxonomy!.roots[0].name = 'Changed'
  assert.equal(tree.roots[0].name, 'Product', 'editing the form must not touch the loaded KB object')
})

test('a stored layout without folders shows as free-form', () => {
  assert.equal(wikiTaxonomyFromKb({ taxonomy: { roots: [] } }).taxonomy, undefined)
  assert.equal(wikiTaxonomyFromKb(undefined).taxonomy, undefined)
  assert.equal(wikiTaxonomyFromKb({ taxonomy: null }).taxonomy, undefined)
})

test('the stored preset name is kept until the layout is adopted', () => {
  assert.equal(wikiTaxonomyFromKb({ taxonomy_preset: 'it_product' }).taxonomyPreset, 'it_product')
})

test('free-form is sent as an explicit empty layout so it can clear a stored one', () => {
  assert.deepEqual(wikiTaxonomyPayload({ taxonomyPreset: '', taxonomy: undefined }), {
    taxonomy_preset: '',
    taxonomy: { roots: [] },
  })
  assert.deepEqual(wikiTaxonomyPayload(null), { taxonomy_preset: '', taxonomy: { roots: [] } })
})

test('a fixed layout is sent as is', () => {
  assert.deepEqual(wikiTaxonomyPayload({ taxonomyPreset: '', taxonomy: tree }).taxonomy, tree)
})

test('a preset-only KB becomes an editable copy of the template', async () => {
  const state = { taxonomyPreset: 'it_product', taxonomy: undefined as typeof tree | undefined }
  await adoptLegacyTaxonomyPreset(state, async () => [{ id: 'it_product', taxonomy: tree }])
  assert.deepEqual(state.taxonomy, tree)
  assert.equal(state.taxonomyPreset, '')
  state.taxonomy!.roots.push({ name: 'Extra' })
  assert.equal(tree.roots.length, 2, 'the adopted copy must be independent of the template')
})

test('adoption leaves the state alone when it has nothing to do or the fetch fails', async () => {
  const untouched = { taxonomyPreset: '', taxonomy: undefined }
  await adoptLegacyTaxonomyPreset(untouched, async () => assert.fail('must not fetch'))
  assert.deepEqual(untouched, { taxonomyPreset: '', taxonomy: undefined })

  const custom = { taxonomyPreset: 'it_product', taxonomy: tree }
  await adoptLegacyTaxonomyPreset(custom, async () => assert.fail('must not fetch'))
  assert.equal(custom.taxonomy, tree)

  const offline = { taxonomyPreset: 'it_product', taxonomy: undefined }
  const warn = console.warn
  console.warn = () => {}
  try {
    await adoptLegacyTaxonomyPreset(offline, async () => { throw new Error('offline') })
  } finally { console.warn = warn }
  assert.deepEqual(offline, { taxonomyPreset: 'it_product', taxonomy: undefined }, 'the preset name survives a failed fetch')

  const unknown = { taxonomyPreset: 'gone', taxonomy: undefined }
  await adoptLegacyTaxonomyPreset(unknown, async () => [{ id: 'it_product', taxonomy: tree }])
  assert.equal(unknown.taxonomyPreset, 'gone')
})
