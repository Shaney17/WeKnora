package agent

// WikiTemplateTaxonomyPlanPrompt files the entity / concept pages of ONE ingest
// batch into a directory template the user designed (the wiki-taxonomy
// extension, docs/extensions/wiki-taxonomy.md). It is a sibling of
// WikiTaxonomyPlanPrompt rather than an edit of it: the core prompt keeps
// serving free-form KBs, and this one only runs for KBs that carry a template.
//
// The template block is pasted in as data ({{.TaxonomyTemplate}}); the service
// also conforms whatever the model answers onto the template, so the prompt can
// ask for the right thing without being the only line of defence.
const WikiTemplateTaxonomyPlanPrompt = `You are filing wiki pages into a directory template that the user designed. Assign each item below to a path in that template.

{{.TaxonomyTemplate}}
<existing_folders>
{{.ExistingTaxonomy}}
</existing_folders>

<items>
{{.Items}}
</items>

<instructions>
For every item, output a category path: an array of folder labels from broad to narrow (at most {{.MaxLevels}} levels). The category classifies WHAT the item fundamentally IS and which side of the template it belongs to, never the role it plays in one document. The "source" of an item is the document it came from; use it as a hint, but classify the item itself.

- Follow the template rules above exactly. Copy template labels character-for-character.
- Prefer the deepest template folder that fits. Do not stop at a parent that has sub-folders.
- Folders listed in <existing_folders> that are NOT in the template were created below an [open] folder earlier: reuse their EXACT label when one fits, never a synonym.
- Group items of the SAME kind under the SAME folder at the SAME depth.
- Do NOT use the item type ("entity"/"concept") as a folder. Do NOT put slashes inside a single label.
- Every item slug in <items> MUST appear exactly once in the output.
- Write labels that you create yourself in {{.Language}}.

### JSON Formatting Rules
- Output ONLY valid JSON, no preamble.
- Do NOT use literal newlines inside JSON string values.
</instructions>

Output format:
{
  "assignments": [
    {"slug": "entity/zhang-san", "path": ["Technical", "Services"]},
    {"slug": "concept/spring-festival", "path": ["Product", "Festival"]}
  ]
}`
