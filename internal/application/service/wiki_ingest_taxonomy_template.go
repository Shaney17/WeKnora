package service

// Wiki-taxonomy extension: ingest side.
//
// When a KB carries a directory template (types.WikiTaxonomyOf), the batch
// taxonomy plan is taken over by planTemplateTaxonomy instead of the free-form
// planner in wiki_ingest_taxonomy.go. Everything that is specific to the
// extension lives in this file; the core files only carry one-line hooks marked
// "[wiki-taxonomy-ext]". See docs/extensions/wiki-taxonomy.md.
//
// The extension leans on a few core helpers (collectTaxonomyItems,
// selectRelevantFolders, formatExistingTaxonomyForPrompt, parseTaxonomyAssignments,
// generateWithTemplate). If upstream renames one, this file stops compiling,
// which is the signal to adapt it.

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// planTemplateTaxonomy is the extension's replacement for planBatchTaxonomy. The
// second result reports whether the KB has a template at all; when it is false
// the caller falls through to the free-form planner untouched.
//
// It (1) makes sure the template's fixed folders exist, (2) asks the planner to
// file the batch's entities and concepts into the template, conforming every
// answer onto it, and (3) files the per-document summary pages on the template's
// summary shelf. Like the core planner it only returns slug -> path; creating the
// folders for those paths and applying them to unfiled pages stays with the core
// (resolvePlannedFolders and the reduce phase).
func (s *wikiIngestService) planTemplateTaxonomy(
	ctx context.Context,
	chatModel chat.Chat,
	kb *types.KnowledgeBase,
	slugUpdates map[string][]SlugUpdate,
	lang string,
) (map[string][]string, bool) {
	if kb == nil {
		return nil, false
	}
	tax := types.WikiTaxonomyOf(kb.WikiConfig)
	if tax == nil {
		return nil, false
	}

	// Existing folders anchor reuse under [open] folders. A failed lookup only
	// means the planner sees the template alone.
	var pool [][]string
	if s.wikiService != nil {
		paths, err := s.wikiService.ListDistinctCategoryPaths(ctx, kb.ID, wikiTaxonomyFolderPoolMax)
		if err != nil {
			logger.Warnf(ctx, "wiki ingest: list category paths for template plan failed: %v", err)
		} else {
			pool = paths
		}
	}
	pool = s.seedTaxonomyFolders(ctx, kb, tax, pool)

	result := make(map[string][]string)
	if items := collectTaxonomyItems(slugUpdates); len(items) > 0 {
		existing := s.selectRelevantFolders(ctx, kb, items, pool)
		s.planTemplateChunks(ctx, chatModel, items, templateDocTitles(slugUpdates), existing, lang, tax, result)
	}
	fileSummaryPages(result, slugUpdates, tax)
	return result, true
}

// planTemplateChunks asks the planner for a path per item, a chunk at a time,
// and records the conformed answers in result. An item the planner never answered
// (failed call, missing from the JSON) is left out, so it stays unfiled and is
// planned again on a later batch instead of being pinned to a fallback folder.
func (s *wikiIngestService) planTemplateChunks(
	ctx context.Context,
	chatModel chat.Chat,
	items []wikiTaxonomyItem,
	docTitles map[string]string,
	existing [][]string,
	lang string,
	tax *types.WikiTaxonomy,
	result map[string][]string,
) {
	maxLevels := fmt.Sprint(tax.MaxDepth())
	templateBlock := formatTaxonomyTemplateForPrompt(tax)

	for start := 0; start < len(items); start += wikiTaxonomyPlanChunkSize {
		end := start + wikiTaxonomyPlanChunkSize
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]

		tree := formatExistingTaxonomyForPrompt(existing)
		if strings.TrimSpace(tree) == "" {
			tree = wikiTaxonomyEmptyTreeHint
		}

		var itemsBlock strings.Builder
		for _, it := range chunk {
			fmt.Fprintf(&itemsBlock, "- slug: %s | title: %s | type: %s | about: %s",
				it.slug, it.title, it.pageType, previewText(it.about, 120))
			// The source document's title is a strong hint for which side of a
			// template (business vs engineering) an item belongs to.
			if doc := strings.TrimSpace(docTitles[it.slug]); doc != "" {
				fmt.Fprintf(&itemsBlock, " | source: %s", previewText(doc, 80))
			}
			itemsBlock.WriteString("\n")
		}

		raw, err := s.generateWithTemplate(ctx, chatModel, agent.WikiTemplateTaxonomyPlanPrompt, map[string]string{
			"ExistingTaxonomy": tree,
			"Items":            itemsBlock.String(),
			"Language":         lang,
			"MaxLevels":        maxLevels,
			"TaxonomyTemplate": templateBlock,
		})
		if err != nil {
			logger.Warnf(ctx, "wiki ingest: template taxonomy plan call failed (%d items): %v", len(chunk), err)
			continue
		}

		for slug, path := range parseTaxonomyAssignments(raw) {
			clean := tax.Conform(types.CleanWikiCategoryPath(path))
			result[slug] = clean
			if len(clean) > 0 {
				existing = append(existing, clean) // feed forward so later chunks converge
			}
		}
	}
}

// templateDocTitles maps each entity/concept slug of the batch to the title of
// the document it came from.
func templateDocTitles(slugUpdates map[string][]SlugUpdate) map[string]string {
	titles := make(map[string]string, len(slugUpdates))
	for slug, updates := range slugUpdates {
		for _, u := range updates {
			if u.Type == types.WikiPageTypeEntity || u.Type == types.WikiPageTypeConcept {
				titles[slug] = u.DocTitle
				break
			}
		}
	}
	return titles
}

// seedTaxonomyFolders creates the template's fixed folders that the KB does not
// have yet, so the skeleton is in place from the first ingest and no one has to
// build it by hand. It is idempotent and returns pool extended with whatever it
// created. A folder that fails to create is logged and skipped: planning still
// works, and the next batch retries.
func (s *wikiIngestService) seedTaxonomyFolders(
	ctx context.Context, kb *types.KnowledgeBase, tax *types.WikiTaxonomy, pool [][]string,
) [][]string {
	if s.wikiService == nil || tax == nil {
		return pool
	}
	key := func(path []string) string { return strings.ToLower(strings.Join(path, "/")) }
	have := make(map[string]struct{}, len(pool))
	for _, p := range pool {
		have[key(p)] = struct{}{}
	}
	for _, path := range tax.FixedPaths() {
		if _, ok := have[key(path)]; ok {
			continue
		}
		if _, _, err := s.wikiService.FindOrCreateFolderPath(ctx, kb.ID, kb.TenantID, path); err != nil {
			logger.Warnf(ctx, "wiki ingest: seed taxonomy folder %q failed: %v", strings.Join(path, "/"), err)
			continue
		}
		have[key(path)] = struct{}{}
		pool = append(pool, path)
	}
	return pool
}

// fileSummaryPages assigns every per-document summary page in the batch to the
// template's summary shelf. Summaries are not entities or concepts, so the
// planner never sees them; without this they all pile up at the wiki root.
func fileSummaryPages(result map[string][]string, slugUpdates map[string][]SlugUpdate, tax *types.WikiTaxonomy) {
	if tax == nil || len(tax.SummaryPath) == 0 {
		return
	}
	for slug, updates := range slugUpdates {
		for _, u := range updates {
			if u.Type == types.WikiPageTypeSummary {
				result[slug] = append([]string(nil), tax.SummaryPath...)
				break
			}
		}
	}
}

// formatTaxonomyTemplateForPrompt renders a directory template as the
// <fixed_template> block of the planning prompt. Template labels are data
// pasted into the prompt, not part of the prompt template itself.
func formatTaxonomyTemplateForPrompt(tax *types.WikiTaxonomy) string {
	if tax == nil || len(tax.Roots) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<fixed_template>\n")

	var render func(nodes []types.WikiTaxonomyNode, depth int)
	render = func(nodes []types.WikiTaxonomyNode, depth int) {
		for _, n := range nodes {
			kind := "fixed"
			if n.Open {
				kind = "open"
			}
			fmt.Fprintf(&b, "%s- %s [%s]", strings.Repeat("  ", depth), n.Name, kind)
			if d := strings.TrimSpace(n.Description); d != "" {
				fmt.Fprintf(&b, ": %s", d)
			}
			b.WriteString("\n")
			render(n.Children, depth+1)
		}
	}
	render(tax.Roots, 0)

	b.WriteString("\nTemplate rules:\n")
	b.WriteString("- The first label of every path MUST be exactly one of the top-level folders above. " +
		"Copy template labels character-for-character; never translate, rename or re-case them.\n")
	b.WriteString("- A [fixed] folder accepts NO new sub-folders and no new siblings. If it lists sub-folders, " +
		"choose ONLY among them and pick one rather than stopping at the parent; if it lists none, " +
		"stop at that folder. Never add a level that is not in the template.\n")
	b.WriteString("- Below an [open] folder, reuse a sub-folder from <existing_folders> when one fits. " +
		"Otherwise create a broad, durable one (for example one per product, then one per feature area).\n")
	if len(tax.SummaryPath) > 0 {
		fmt.Fprintf(&b, "- Never file an item under %q: it is reserved for per-document summaries.\n",
			strings.Join(tax.SummaryPath, "/"))
	}
	if len(tax.FallbackPath) > 0 {
		fmt.Fprintf(&b, "- Use %q only when an item clearly fits nowhere else.\n",
			strings.Join(tax.FallbackPath, "/"))
	}
	b.WriteString("</fixed_template>\n")
	return b.String()
}

// applyPlannedFolder files a page that has no folder yet into the folder the
// batch plan chose for its slug. Pages that are already filed, including those a
// user moved by hand, keep their placement. The core reduce phase does the same
// inline for entity and concept pages; this is the hook for summary pages, whose
// branch returns before that code.
func applyPlannedFolder(page *types.WikiPage, batchCtx *WikiBatchContext, slug string) {
	if page.FolderID != "" || batchCtx == nil {
		return
	}
	if fid := batchCtx.PlannedFolderID[slug]; fid != "" {
		page.FolderID = fid
	}
}
