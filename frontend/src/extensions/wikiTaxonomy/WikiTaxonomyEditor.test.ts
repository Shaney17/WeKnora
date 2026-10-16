import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { runInNewContext } from 'node:vm'
import test from 'node:test'
import { compileScript, parse } from '@vue/compiler-sfc'
import ts from 'typescript'
import { createRenderer, h, nextTick, reactive, ref } from 'vue'

const require = createRequire(import.meta.url)
const filename = fileURLToPath(new URL('./WikiTaxonomyEditor.vue', import.meta.url))
const { descriptor } = parse(readFileSync(filename, 'utf8'), { filename })
const script = compileScript(descriptor, { id: 'wiki-taxonomy-editor-test' }).content
  .replace('__expose();', '')
  .replace('return __returned__', '__expose(__returned__); return __returned__')
const compiled = ts.transpileModule(script, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText

const NEW_FOLDER = 'extensions.wikiTaxonomy.newFolder'

type Node = { name: string; description?: string; children?: Node[]; open?: boolean }
type Taxonomy = { roots: Node[]; summary_path?: string[]; fallback_path?: string[] }

const sample = (): Taxonomy => ({
  roots: [
    { name: 'Product', open: true },
    { name: 'Technical', children: [{ name: 'Services' }, { name: 'Data' }] },
    { name: 'Sources' },
  ],
  summary_path: ['Sources'],
  fallback_path: ['Technical', 'Data'],
})

async function fixture(initial: Taxonomy | null = sample(), presets: Array<{ id: string; taxonomy: Taxonomy }> = []) {
  const emitted: Taxonomy[] = []
  const dialogs: any[] = []
  const exports: any = {}
  runInNewContext(compiled, {
    exports,
    require(name: string) {
      if (name === 'vue') return require('vue')
      if (name === 'vue-i18n') return { useI18n: () => ({ t: (key: string) => key, te: () => false }) }
      if (name === 'tdesign-vue-next') {
        return { DialogPlugin: { confirm(options: any) { dialogs.push(options); return { destroy() {} } } } }
      }
      if (name === './api') return { fetchWikiTaxonomyPresets: async () => presets }
      return { default: {} }
    },
    URL, console, JSON,
  })
  const component = exports.default
  component.render = () => null
  const renderer = createRenderer<any, any>({
    createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    insert() {}, remove() {}, setElementText() {}, setText() {}, patchProp() {},
    parentNode: () => null, nextSibling: () => null,
  })
  const props = reactive<{ modelValue: Taxonomy | null }>({ modelValue: initial })
  const instance = ref<any>()
  const app = renderer.createApp({
    render: () => h(component, {
      modelValue: props.modelValue,
      'onUpdate:modelValue': (value: Taxonomy) => emitted.push(value),
      ref: instance,
    }),
  })
  app.mount({})
  await nextTick()
  await nextTick()
  const vm = instance.value
  const row = (name: string) => vm.rows.find((r: any) => r.node.name === name)
  return { vm, emitted, dialogs, props, row, last: () => emitted[emitted.length - 1], close: () => app.unmount() }
}

test('edits are emitted as a new object and never mutate the parent value', async () => {
  const parent = sample()
  const f = await fixture(parent)
  try {
    f.vm.addRoot()
    await nextTick()
    assert.equal(f.emitted.length, 1)
    assert.deepEqual(f.last().roots.map(r => r.name), ['Product', 'Technical', 'Sources', NEW_FOLDER])
    assert.equal(parent.roots.length, 3, 'the prop object must stay untouched')
  } finally { f.close() }
})

test('a parent update that matches the last emit does not echo back', async () => {
  const f = await fixture()
  try {
    f.vm.addRoot()
    await nextTick()
    f.props.modelValue = f.last()
    await nextTick()
    assert.equal(f.emitted.length, 1, 'no second emit for the value we just sent')
  } finally { f.close() }
})

test('new folders get unique names among their siblings', async () => {
  const f = await fixture({ roots: [] })
  try {
    f.vm.addRoot(); f.vm.addRoot(); f.vm.addRoot()
    await nextTick()
    assert.deepEqual(f.vm.draft.roots.map((r: Node) => r.name), [NEW_FOLDER, `${NEW_FOLDER} 2`, `${NEW_FOLDER} 3`])
  } finally { f.close() }
})

test('renaming a folder keeps the summary and fallback targets pointing at it', async () => {
  const f = await fixture()
  try {
    f.vm.rename(f.row('Technical'), 'Engineering')
    await nextTick()
    assert.deepEqual(f.last().fallback_path, ['Engineering', 'Data'], 'descendant target follows the parent rename')
    assert.deepEqual(f.last().summary_path, ['Sources'], 'unrelated target is untouched')

    f.vm.rename(f.row('Sources'), 'Docs')
    await nextTick()
    assert.deepEqual(f.last().summary_path, ['Docs'])
  } finally { f.close() }
})

test('removing a folder clears targets that pointed into it', async () => {
  const f = await fixture()
  try {
    f.vm.remove(f.row('Technical'))
    await nextTick()
    assert.equal(f.last().fallback_path, undefined)
    assert.deepEqual(f.last().summary_path, ['Sources'])
    assert.deepEqual(f.last().roots.map(r => r.name), ['Product', 'Sources'])
  } finally { f.close() }
})

test('sub-folders stop at the backend depth cap', async () => {
  const f = await fixture({ roots: [{ name: 'A', children: [{ name: 'B', children: [{ name: 'C' }] }] }] })
  try {
    f.vm.addChild(f.row('C'))
    await nextTick()
    assert.equal(f.row('C').node.children, undefined, 'a depth-3 folder cannot get children')
    f.vm.addChild(f.row('B'))
    await nextTick()
    assert.equal(f.row('B').node.children.length, 2)
  } finally { f.close() }
})

test('moving a folder reorders siblings and ignores the ends of the list', async () => {
  const f = await fixture()
  try {
    f.vm.move(f.row('Sources'), -1)
    await nextTick()
    assert.deepEqual(f.last().roots.map(r => r.name), ['Product', 'Sources', 'Technical'])
    const before = f.emitted.length
    f.vm.move(f.row('Product'), -1)
    await nextTick()
    assert.equal(f.emitted.length, before, 'moving the first folder up changes nothing')
  } finally { f.close() }
})

test('the editor reports empty, slashed and duplicate names', async () => {
  const f = await fixture({ roots: [{ name: 'A' }, { name: 'a' }] })
  try {
    assert.equal(f.vm.issue, 'extensions.wikiTaxonomy.issueDuplicate')
    f.vm.draft.roots[1].name = ' '
    await nextTick()
    assert.equal(f.vm.issue, 'extensions.wikiTaxonomy.issueEmpty')
    f.vm.draft.roots[1].name = 'x/y'
    await nextTick()
    assert.equal(f.vm.issue, 'extensions.wikiTaxonomy.issueSlash')
    f.vm.draft.roots[1].name = 'B'
    await nextTick()
    assert.equal(f.vm.issue, '')
  } finally { f.close() }
})

test('a template loads straight into an empty editor', async () => {
  const tpl = { id: 'it_product', taxonomy: sample() }
  const f = await fixture({ roots: [] }, [tpl])
  try {
    await new Promise(resolve => setTimeout(resolve, 0))
    f.vm.onPickPreset('it_product')
    await nextTick()
    assert.deepEqual(f.last().roots.map(r => r.name), ['Product', 'Technical', 'Sources'])
    assert.equal(f.dialogs.length, 0, 'nothing to overwrite, so no confirmation')
  } finally { f.close() }
})

test('loading a template over an existing structure asks first', async () => {
  const tpl = { id: 'it_product', taxonomy: sample() }
  const f = await fixture({ roots: [{ name: 'Mine' }] }, [tpl])
  try {
    await new Promise(resolve => setTimeout(resolve, 0))
    f.vm.onPickPreset('it_product')
    await nextTick()
    assert.equal(f.emitted.length, 0, 'nothing is replaced before the user confirms')
    assert.equal(f.dialogs.length, 1)
    f.dialogs[0].onConfirm()
    await nextTick()
    assert.deepEqual(f.last().roots.map(r => r.name), ['Product', 'Technical', 'Sources'])
  } finally { f.close() }
})
