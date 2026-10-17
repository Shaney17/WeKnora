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
For every item, output a category path: an array of folder labels from broad to narrow (at most {{.MaxLevels}} levels). The category classifies WHAT the item fundamentally IS and which side of the template it belongs to, never the role it plays in one document. The "source" of an item is the document it came from; use it only as a hint about the nature of the item, classify the item itself.

- Follow the template rules above exactly. Copy template labels character-for-character.
- Prefer the deepest template folder that fits. Do not stop at a parent that has sub-folders.
- The folder descriptions in the template are the criterion. Where they leave room, classify by what the item IS: an API, endpoint, stream topic, event, message or payload belongs in an interface or integration folder if the template has one; an incident, failure case, exception handling or runbook belongs in an operations folder; configuration parameters, rate tables and calculation or rounding rules belong in a configuration folder; a system, service or component belongs in a services folder; what the product does for its users, with its business rules, belongs on the business side.
- Name sub-folders that you create below an [open] folder after a business capability or subject area (for example "Order management"), never after a source document, a file name or a document type, and never after a technical word such as "API". Keep them few (at most about six per level) and reuse an existing one whenever it fits.
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
