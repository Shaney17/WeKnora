package types

import "strings"

// Wiki-taxonomy extension: built-in quality guidance.
//
// A KB that follows a directory layout also gets these rules, merged into the
// two instruction channels the wiki pipeline already has (extraction and page
// content). They are the part of "what good wiki pages look like" that the
// user should not have to know to write down. The user's own instructions, if
// any, are appended after them and may refine them.
// See docs/extensions/wiki-taxonomy.md.

// WikiExtractionGuidance steers what the extraction pass treats as a page-worthy
// subject. Kept short: it is added to every extraction and citation call.
const WikiExtractionGuidance = `What to extract for this wiki:
- Extract subjects that a reader would look up on their own: product capabilities and their rules, domain terms, systems and services, APIs / events / messages as contracts, configuration parameters and calculation rules.
- Do NOT extract: people, teams or roles who merely own or wrote a document; document metadata (status, version, epic, change history); URLs, host names or fragments of them; placeholder or sample identifiers and values (order_1, IDs, timestamps); the individual sample objects of one incident (describe the incident once, as a single concept); status values, enum values and field names (cover them inside their parent concept); anything that is only mentioned in passing.
- Use one canonical name per subject. Put abbreviations, translations and alternate spellings in aliases (a term and its acronym, or the same term in two languages).`

// WikiContentGuidance steers how pages and summaries are written.
const WikiContentGuidance = `Quality rules for this wiki. It is read by people who review it and by an assistant that answers questions from it:
- State only what the source supports. Never guess what an unexplained name stands for.
- An entity page says what the thing is, what it is responsible for and how it relates to other entities, in a few short paragraphs. Do not reproduce step-by-step flows, state tables or payloads there; link to the concept page that owns them.
- A concept page defines one idea with its rules, conditions and examples. Keep a table only when the table is the definition.
- Never repeat document metadata (status, owner, epic, version, change history, design-tool links) in the text.
- Ignore working notes in the source such as "ok", "TBD", "done" or strike-through remarks. If the source marks an item as removed or deprecated, say so explicitly instead of listing it as current.
- Reference prices, balances and sample values are examples at one point in time: label them as such or leave them out.
- Do not copy sample identifiers (order numbers, IDs, timestamps) unless the page is about that exact incident.`

// WithWikiTaxonomyGuidance returns the content and extraction instructions the
// wiki pipeline should use for a KB. A KB without a directory layout keeps the
// user's instructions untouched; one with a layout gets the built-in guidance
// first and the user's instructions after it.
func WithWikiTaxonomyGuidance(c *WikiConfig, content, extraction string) (string, string) {
	if WikiTaxonomyOf(c) == nil {
		return content, extraction
	}
	return joinGuidance(WikiContentGuidance, content), joinGuidance(WikiExtractionGuidance, extraction)
}

func joinGuidance(builtIn, user string) string {
	user = strings.TrimSpace(user)
	if user == "" {
		return builtIn
	}
	return builtIn + "\n\nAdditional instructions from the knowledge base owner:\n" + user
}
