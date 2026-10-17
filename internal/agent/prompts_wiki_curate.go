package agent

// WikiCurationPrompt is the wiki-taxonomy extension's quality pass over the
// candidate entities and concepts of ONE ingest batch, run after extraction and
// before any page is written. It removes subjects that do not deserve a page and
// folds duplicates together, so the wiki is not padded with people, URL
// fragments, sample values or the same idea under three names.
//
// The model only proposes; the service validates every decision (known slugs,
// same type, no cycles, a cap on how much may change) before applying it. See
// docs/extensions/wiki-taxonomy.md.
const WikiCurationPrompt = `You are the quality editor of a wiki. It will be read by people who review it and by an assistant that answers questions from it. Below are CANDIDATE pages that were just extracted from documents. Decide which candidates should not become their own page.

<existing_pages>
{{.ExistingPages}}
</existing_pages>

<candidates>
{{.Candidates}}
</candidates>

<instructions>
For each candidate you may output a decision. Candidates you do not mention are kept. When unsure, KEEP.

"drop": the candidate does not deserve a page. Use it ONLY for these reasons:
- person_or_team: a person, team or role that owns or wrote a document, not a subject people look up.
- metadata: document status, version, epic, change history or a link to a design tool.
- url_fragment: a URL, a host name or a piece of one.
- sample_value: a placeholder or sample identifier, value or instance used as an example (for example order_1, an order number, a timestamp).
- field_or_enum: a status value, enum value, field or column name. It belongs inside its parent concept.
- passing_mention: named only in passing, with nothing substantive to say (cited_chunks is 1 or less and the description adds nothing).

"merge": the candidate is the SAME subject as another candidate or as an existing page: the same term in two languages, an acronym and its full name, singular and plural, a spelling variant. Set "into" to the slug of that other candidate or existing page. Both must have the same type. Prefer merging into an existing page. Do not merge subjects that are merely related, or a whole into one of its parts.

Never drop a candidate whose description shows that it is a system, service, API, message, business rule, policy, configuration or product capability.

### JSON Formatting Rules
- Output ONLY valid JSON, no preamble.
- Do NOT use literal newlines inside JSON string values.
</instructions>

Output format:
{
  "decisions": [
    {"slug": "entity/order-1", "action": "drop", "reason": "sample_value"},
    {"slug": "concept/lenh-lo", "action": "merge", "into": "concept/limit-order", "reason": "duplicate"}
  ]
}`
