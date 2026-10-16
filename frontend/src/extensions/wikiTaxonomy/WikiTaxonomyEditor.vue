<template>
  <div class="wiki-taxonomy-editor">
    <div class="wte-toolbar">
      <t-select
        v-if="presetOptions.length"
        class="wte-template"
        :value="''"
        :options="presetOptions"
        :placeholder="$t('extensions.wikiTaxonomy.loadTemplate')"
        size="small"
        @change="onPickPreset"
      />
      <t-button size="small" variant="outline" @click="addRoot">
        <template #icon><t-icon name="add" /></template>
        {{ $t('extensions.wikiTaxonomy.addRoot') }}
      </t-button>
    </div>

    <p v-if="!rows.length" class="wte-empty">{{ $t('extensions.wikiTaxonomy.empty') }}</p>

    <div v-else class="wte-tree">
      <div v-for="row in rows" :key="row.key" class="wte-row" :style="{ paddingLeft: `${(row.depth - 1) * 20}px` }">
        <t-input
          class="wte-name"
          size="small"
          :value="row.node.name"
          :maxlength="64"
          :status="row.node.name.trim() ? 'default' : 'error'"
          :placeholder="$t('extensions.wikiTaxonomy.namePlaceholder')"
          @change="(v: string) => rename(row, v)"
        />
        <t-input
          class="wte-desc"
          size="small"
          :value="row.node.description || ''"
          :maxlength="300"
          :placeholder="$t('extensions.wikiTaxonomy.descPlaceholder')"
          @change="(v: string) => (row.node.description = v)"
        />
        <t-tooltip :content="$t('extensions.wikiTaxonomy.openTip')">
          <label class="wte-open" :class="{ 'wte-open--disabled': row.depth >= MAX_DEPTH }">
            <t-switch
              size="small"
              :value="!!row.node.open"
              :disabled="row.depth >= MAX_DEPTH"
              @change="(v: boolean) => (row.node.open = v)"
            />
            <span>{{ $t('extensions.wikiTaxonomy.openLabel') }}</span>
          </label>
        </t-tooltip>
        <div class="wte-actions">
          <t-button
            size="small" variant="text" shape="square"
            :disabled="row.depth >= MAX_DEPTH"
            :title="$t('extensions.wikiTaxonomy.addChild')"
            @click="addChild(row)"
          >
            <t-icon name="add" />
          </t-button>
          <t-button
            size="small" variant="text" shape="square"
            :disabled="row.index === 0"
            :title="$t('extensions.wikiTaxonomy.moveUp')"
            @click="move(row, -1)"
          >
            <t-icon name="arrow-up" />
          </t-button>
          <t-button
            size="small" variant="text" shape="square"
            :disabled="row.index === row.siblings.length - 1"
            :title="$t('extensions.wikiTaxonomy.moveDown')"
            @click="move(row, 1)"
          >
            <t-icon name="arrow-down" />
          </t-button>
          <t-button
            size="small" variant="text" shape="square" theme="danger"
            :title="$t('extensions.wikiTaxonomy.remove')"
            @click="remove(row)"
          >
            <t-icon name="delete" />
          </t-button>
        </div>
      </div>
    </div>

    <p v-if="issue" class="wte-issue">{{ issue }}</p>

    <div v-if="rows.length" class="wte-targets">
      <div class="wte-target">
        <label>{{ $t('extensions.wikiTaxonomy.summaryTarget') }}</label>
        <t-select
          size="small"
          clearable
          :value="pathValue(draft.summary_path)"
          :options="pathOptions"
          :placeholder="$t('extensions.wikiTaxonomy.rootOption')"
          @change="(v: string) => setTarget('summary_path', v)"
        />
      </div>
      <div class="wte-target">
        <label>{{ $t('extensions.wikiTaxonomy.fallbackTarget') }}</label>
        <t-select
          size="small"
          clearable
          :value="pathValue(draft.fallback_path)"
          :options="pathOptions"
          :placeholder="$t('extensions.wikiTaxonomy.rootOption')"
          @change="(v: string) => setTarget('fallback_path', v)"
        />
      </div>
    </div>

    <p v-if="rows.length" class="wte-note">{{ $t('extensions.wikiTaxonomy.lockedNote') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin } from 'tdesign-vue-next'
import {
  fetchWikiTaxonomyPresets,
  type WikiTaxonomy,
  type WikiTaxonomyNode,
  type WikiTaxonomyPreset,
} from './api'

// Mirrors the backend cap (types.WikiCategoryMaxDepth): deeper folders are
// rejected on save, and a folder at this depth cannot be opened for sub-folders.
const MAX_DEPTH = 3

interface Row {
  key: string
  path: number[]
  depth: number
  node: WikiTaxonomyNode
  siblings: WikiTaxonomyNode[]
  index: number
}

const props = defineProps<{ modelValue: WikiTaxonomy | undefined | null }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: WikiTaxonomy): void }>()
const { t, te } = useI18n()

const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value))
const blank = (): WikiTaxonomy => ({ roots: [] })

// The editor works on its own copy and emits on every change, so the parent form
// is never mutated in place. `lastEmitted` stops the two watchers echoing each other.
const draft = ref<WikiTaxonomy>(props.modelValue ? clone(props.modelValue) : blank())
let lastEmitted = JSON.stringify(draft.value)

watch(
  () => props.modelValue,
  (value) => {
    const next = value ? clone(value) : blank()
    const serialized = JSON.stringify(next)
    if (serialized === lastEmitted) return
    draft.value = next
    lastEmitted = serialized
  },
  { deep: true },
)

watch(
  draft,
  (value) => {
    const serialized = JSON.stringify(value)
    if (serialized === lastEmitted) return
    lastEmitted = serialized
    emit('update:modelValue', clone(value))
  },
  { deep: true },
)

const rows = computed<Row[]>(() => {
  const out: Row[] = []
  const walk = (nodes: WikiTaxonomyNode[], prefix: number[]) => {
    nodes.forEach((node, index) => {
      const path = [...prefix, index]
      out.push({ key: path.join('-'), path, depth: path.length, node, siblings: nodes, index })
      if (node.children?.length) walk(node.children, path)
    })
  }
  walk(draft.value.roots, [])
  return out
})

// Names along an index path, e.g. [0, 2] -> ['Technical', 'Data'].
const namesAlong = (indexPath: number[]): string[] => {
  const names: string[] = []
  let nodes: WikiTaxonomyNode[] | undefined = draft.value.roots
  for (const i of indexPath) {
    const node: WikiTaxonomyNode | undefined = nodes?.[i]
    if (!node) break
    names.push(node.name)
    nodes = node.children
  }
  return names
}

const startsWith = (path: string[], prefix: string[]) =>
  prefix.length > 0 && prefix.length <= path.length && prefix.every((p, i) => p === path[i])

type TargetKey = 'summary_path' | 'fallback_path'
const targetKeys: TargetKey[] = ['summary_path', 'fallback_path']

const uniqueName = (siblings: WikiTaxonomyNode[]) => {
  const base = t('extensions.wikiTaxonomy.newFolder')
  const taken = new Set(siblings.map((s) => s.name.trim().toLowerCase()))
  let name = base
  for (let n = 2; taken.has(name.toLowerCase()); n++) name = `${base} ${n}`
  return name
}

const addRoot = () => {
  draft.value.roots.push({ name: uniqueName(draft.value.roots) })
}

const addChild = (row: Row) => {
  if (row.depth >= MAX_DEPTH) return
  if (!row.node.children) row.node.children = []
  row.node.children.push({ name: uniqueName(row.node.children) })
}

// Renaming keeps the summary/fallback targets pointing at the same folder.
const rename = (row: Row, value: string) => {
  const before = namesAlong(row.path)
  row.node.name = value
  const after = namesAlong(row.path)
  for (const key of targetKeys) {
    const current = draft.value[key]
    if (current && startsWith(current, before)) {
      draft.value[key] = [...after, ...current.slice(before.length)]
    }
  }
}

const remove = (row: Row) => {
  const gone = namesAlong(row.path)
  row.siblings.splice(row.index, 1)
  for (const key of targetKeys) {
    const current = draft.value[key]
    if (current && startsWith(current, gone)) delete draft.value[key]
  }
}

const move = (row: Row, delta: number) => {
  const to = row.index + delta
  if (to < 0 || to >= row.siblings.length) return
  const [node] = row.siblings.splice(row.index, 1)
  row.siblings.splice(to, 0, node)
}

const pathOptions = computed(() =>
  rows.value
    .filter((r) => r.node.name.trim())
    .map((r) => {
      const names = namesAlong(r.path)
      return { label: names.join(' / '), value: names.join('/') }
    }),
)

const pathValue = (path?: string[]) => (path && path.length ? path.join('/') : '')

const setTarget = (key: TargetKey, value: string) => {
  if (value) draft.value[key] = value.split('/')
  else delete draft.value[key]
}

// First problem worth telling the user about before they hit save; the backend
// enforces the same rules and rejects the save with a message.
const issue = computed(() => {
  const check = (nodes: WikiTaxonomyNode[]): string => {
    const seen = new Set<string>()
    for (const node of nodes) {
      const name = node.name.trim()
      if (!name) return t('extensions.wikiTaxonomy.issueEmpty')
      if (name.includes('/')) return t('extensions.wikiTaxonomy.issueSlash', { name })
      const key = name.toLowerCase()
      if (seen.has(key)) return t('extensions.wikiTaxonomy.issueDuplicate', { name })
      seen.add(key)
      const nested = check(node.children ?? [])
      if (nested) return nested
    }
    return ''
  }
  return check(draft.value.roots)
})

const presets = ref<WikiTaxonomyPreset[]>([])
onMounted(async () => {
  try {
    presets.value = await fetchWikiTaxonomyPresets()
  } catch {
    // Templates are a convenience; the tree can still be built by hand.
    presets.value = []
  }
})

const presetLabel = (id: string) => {
  const key = `extensions.wikiTaxonomy.templates.${id}`
  return te(key) ? t(key) : id
}
const presetOptions = computed(() => presets.value.map((p) => ({ label: presetLabel(p.id), value: p.id })))

const onPickPreset = (id: string) => {
  const preset = presets.value.find((p) => p.id === id)
  if (!preset) return
  const apply = () => {
    draft.value = clone(preset.taxonomy)
  }
  if (!rows.value.length) {
    apply()
    return
  }
  const dialog = DialogPlugin.confirm({
    header: t('extensions.wikiTaxonomy.replaceHeader'),
    body: t('extensions.wikiTaxonomy.replaceBody'),
    onConfirm: () => {
      apply()
      dialog.destroy()
    },
    onClose: () => dialog.destroy(),
  })
}
</script>

<style scoped lang="less">
.wiki-taxonomy-editor {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}

.wte-toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.wte-template {
  width: 220px;
}

.wte-empty,
.wte-note {
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.wte-tree {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
}

.wte-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.wte-name {
  flex: 0 0 180px;
}

.wte-desc {
  flex: 1 1 auto;
  min-width: 0;
}

.wte-open {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;

  &--disabled {
    opacity: 0.5;
  }
}

.wte-actions {
  display: flex;
  flex: 0 0 auto;
}

.wte-issue {
  margin: 0;
  font-size: 12px;
  color: var(--td-error-color);
}

.wte-targets {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.wte-target {
  display: flex;
  flex-direction: column;
  gap: 4px;

  label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}
</style>
