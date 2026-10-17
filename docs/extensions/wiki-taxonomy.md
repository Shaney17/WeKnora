# Wiki taxonomy extension

Lets a knowledge base follow a **folder layout the user defines** instead of the
folders the LLM invents. The user edits the layout in the KB settings before the
first ingest; ingest then creates those folders and files pages only into them.

This is a downstream extension of WeKnora, written so that pulling upstream
(`Tencent/WeKnora`) stays cheap: all of its logic lives in new files, and the
core is touched only by a handful of one-line hooks, each tagged
`[wiki-taxonomy-ext]`.

## Behaviour

- Per KB: **Free-form** (upstream behaviour, the default) or **Fixed structure**.
- A layout is a tree of folders (max 3 levels). Each folder has a name, a
  description (guides the classifier) and an **Allow sub-folders** switch,
  **off** by default. A folder that is off is closed: ingest never adds folders
  at or below it beyond what the layout lists.
- The layout also names where **per-document summary pages** go and where items
  that fit nowhere go.
- Whatever the LLM answers is conformed onto the layout in code
  (`WikiTaxonomy.Conform`), so the stored tree cannot drift from it.
- Only pages that have no folder yet are filed. Pages a user moved by hand are
  never touched. Renaming or moving a layout folder later does not move pages
  that are already filed: settle the layout before the first ingest.
- A layout always **replaces** the stored one, also through upstream's PATCH
  endpoint (`WikiTaxonomy.UnmarshalJSON`): nothing of the old layout survives
  in the new one. `"taxonomy": null` clears it.
- A KB update from a client that does not know about the extension (older UI,
  API, an assistant) keeps the stored layout; clearing it is explicit
  (`"taxonomy": {"roots": []}`).

## Automatic quality control

A KB that follows a layout also gets, with nothing to configure:

1. **Built-in guidance** (`internal/types/wiki_guidance.go`), merged into the two
   instruction channels the wiki pipeline already has. Extraction: skip people who
   own a document, metadata, URL fragments, sample identifiers, enum values and
   passing mentions; one canonical name, with acronyms and translations as
   aliases. Content: entity pages say what a thing is and does and link to the
   concept page that owns the flows; no document metadata; ignore working notes
   ("ok", "TBD") and say so when the source marks an item removed; label sample
   values as examples. The owner's own instructions, if any, follow these.
2. **A curation pass** (`wiki_ingest_curate.go`) over each batch's candidates,
   before any page is written. Rules drop URLs, host names, e-mail addresses and
   sample identifiers such as `order_1`. One model call per 60 candidates drops
   people/teams, metadata, enum values and passing mentions, and merges the same
   subject under two names (a term and its acronym, or two languages), including
   into pages that already exist. Guard rails: pages that already exist are never
   dropped or merged away; a merge needs a known target of the same kind and no
   cycle; the pass applies nothing if it would change more than half of a batch;
   if it cannot tell which pages exist it does nothing.
3. **A better folder planner:** sub-folders under an open folder are named after a
   business capability, never after a source document or a technical word, and
   APIs, incidents, configuration and services are classified by what they are.

## Files that belong to the extension

| Area | Files |
| --- | --- |
| Types, validation, merge | `internal/types/wiki_taxonomy.go` (+ `_test`) |
| Planner and curation prompts | `internal/agent/prompts_wiki_template.go`, `internal/agent/prompts_wiki_curate.go` |
| Built-in guidance | `internal/types/wiki_guidance.go` (+ `_test`) |
| Ingest planner, curation | `internal/application/service/wiki_ingest_taxonomy_template.go`, `wiki_ingest_curate.go` (+ `_test`s) |
| Template list endpoint | `internal/handler/wiki_taxonomy_presets.go` (+ `_test`), `internal/router/routes_wiki_taxonomy.go` |
| Frontend | `frontend/src/extensions/wikiTaxonomy/` (editor, settings panel, API client, form helpers, messages, tests) |
| This doc, the guard | `docs/extensions/wiki-taxonomy.md`, `scripts/check-wiki-taxonomy-ext.sh` |

UI messages are merged at runtime under `extensions.wikiTaxonomy.*`
(`vue-i18n` `mergeLocaleMessage`), so the core locale files are not edited.

## Hooks in core files

These are the only edits to upstream files. If a merge drops one, re-apply it.

| File | Hooks | What it does |
| --- | --- | --- |
| `internal/types/wiki_page.go` | 1 | Embeds `WikiTaxonomyConfig` in `WikiConfig` (JSON stays flat) |
| `internal/types/prompt_instructions.go` | 1 | Validates the layout wherever a KB is created or updated |
| `internal/application/service/wiki_ingest_taxonomy.go` | 1 | `planBatchTaxonomy` hands KBs with a layout to `planTemplateTaxonomy` |
| `internal/application/service/wiki_ingest_batch.go` | 2 | Files summary pages (their branch returns before the usual placement); adds the built-in guidance to the batch context |
| `internal/application/service/knowledgebase.go` | 1 | Keeps the stored layout when an update omits it (`MergeWikiTaxonomy`) |
| `internal/router/router.go` | 1 | Registers `GET /api/v1/wiki-taxonomy/presets` |
| `frontend/src/views/knowledge/KnowledgeBaseEditorModal.vue` | 7 | import, settings panel, initial state, load, adopt-preset, create and update payloads |

The extension also calls a few core helpers (`collectTaxonomyItems`,
`selectRelevantFolders`, `formatExistingTaxonomyForPrompt`,
`parseTaxonomyAssignments`, `generateWithTemplate`, `FindOrCreateFolderPath`).
If upstream renames one, `wiki_ingest_taxonomy_template.go` stops compiling:
adapt it there.

## Syncing with upstream

```bash
git fetch upstream
git merge upstream/main          # or: git rebase upstream/main
scripts/check-wiki-taxonomy-ext.sh --test
```

`check-wiki-taxonomy-ext.sh` verifies that every hook is still present and that
the extension files exist; `--test` also runs the extension's Go and frontend
tests. It also lists any file that differs from upstream but is neither an
extension file nor a hook file, which is where an accidental edit would hide.

Conflicts, when they happen, are confined to the hook lines above. Enabling
`git config rerere.enabled true` makes git remember each resolution.
