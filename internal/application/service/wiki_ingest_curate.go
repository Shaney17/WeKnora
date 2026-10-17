package service

// Wiki-taxonomy extension: candidate curation.
//
// Between extraction and page writing, a KB with a directory layout gets a
// quality pass over the batch's candidate entities and concepts. It removes
// subjects that do not deserve a page (people who own a document, URL fragments,
// sample values such as order_1, enum values, passing mentions) and folds
// duplicates together (the same term in two languages, an acronym and its full
// name), so the wiki is not padded with noise and one idea has one page.
//
// It runs from planTemplateTaxonomy and edits the batch's slugUpdates map in
// place, which is the same map the reduce phase then iterates. Links that other
// pages already carry to a removed candidate are repaired by the core's dead-link
// cleanup. See docs/extensions/wiki-taxonomy.md.
//
// Two layers decide: reliable rules (heuristicJunkReason) and one LLM call per
// chunk of candidates. The model only proposes; applyCuration validates every
// decision and refuses to change more than a bounded share of the batch, so a
// misbehaving answer cannot gut a wiki.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// wikiCurationMaxChangeRatio bounds how much of a batch curation may drop or
	// merge. Past it the whole answer is discarded as untrustworthy.
	wikiCurationMaxChangeRatio = 0.5
	// wikiCurationExistingPerType is how many existing pages of each type the
	// model sees as merge targets.
	wikiCurationExistingPerType = 120
	// wikiCurationMaxHops bounds how far a chain of merges is followed.
	wikiCurationMaxHops = 5
)

const (
	curationKeep  = "keep"
	curationDrop  = "drop"
	curationMerge = "merge"
)

type curationDecision struct {
	Slug   string `json:"slug"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	Into   string `json:"into"`
}

// curationReport says what curation did, for the log and for tests.
type curationReport struct {
	Dropped   []string
	Merged    map[string]string // slug -> final target
	Rejected  int               // decisions that failed validation
	CappedOut bool              // the change budget was exceeded, nothing was applied
}

// curationCaller asks the model for decisions. nil means "no model available":
// only the rules run.
type curationCaller func(ctx context.Context, data map[string]string) (string, error)

// curationCandidate is one extracted entity/concept as the model sees it.
type curationCandidate struct {
	slug        string
	kind        string // entity | concept
	name        string
	description string
	aliases     []string
	chunks      int
	docs        []string
}

var (
	curationHostRe = regexp.MustCompile(`(?i)^(www\.)?[a-z0-9][a-z0-9-]*(\.[a-z0-9-]+)*\.(com|net|org|io|dev|app|vn|cn|co|ai)(\.[a-z]{2})?(/\S*)?$`)
	curationIDRe   = regexp.MustCompile(`(?i)^(order|ord|trade|txn|tx|user|usr|acc|account|sub|id|ref)[ _-]?\d+$`)
	curationLongID = regexp.MustCompile(`^[A-Za-z]{0,3}\d{8,}[A-Za-z0-9]*$`)
	curationEmail  = regexp.MustCompile(`(?i)^[^@\s]+@[^@\s]+\.[a-z]{2,}$`)
)

// heuristicJunkReason recognizes names that can never be a page-worthy subject
// without asking a model: URLs and host names, e-mail addresses, sample
// identifiers like order_1 or a long numeric ID, and names with no letters.
// It returns "" when the name is fine. It is deliberately narrow: a wrong drop
// costs a page, so anything ambiguous is left to the model's judgement.
func heuristicJunkReason(name string) string {
	n := strings.TrimSpace(name)
	switch {
	case n == "":
		return "empty"
	case strings.Contains(n, "://") || curationHostRe.MatchString(n):
		return "url_fragment"
	case curationEmail.MatchString(n):
		return "sample_value"
	case curationIDRe.MatchString(n) || curationLongID.MatchString(n):
		return "sample_value"
	}
	hasLetter := false
	for _, r := range n {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return "sample_value"
	}
	return ""
}

func slugKind(slug string) string {
	if i := strings.Index(slug, "/"); i >= 0 {
		return slug[:i]
	}
	return ""
}

// curatable reports whether a slug's updates are plain additions of one
// entity/concept. A slug that also carries a retraction is part of a document
// being re-ingested; it is left strictly alone.
func curatable(updates []SlugUpdate) bool {
	has := false
	for _, u := range updates {
		switch u.Type {
		case types.WikiPageTypeEntity, types.WikiPageTypeConcept:
			has = true
		default:
			return false
		}
	}
	return has
}

func collectCurationCandidates(slugUpdates map[string][]SlugUpdate) []curationCandidate {
	slugs := make([]string, 0, len(slugUpdates))
	for slug, updates := range slugUpdates {
		if curatable(updates) {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)

	out := make([]curationCandidate, 0, len(slugs))
	for _, slug := range slugs {
		c := curationCandidate{slug: slug}
		seenDoc := map[string]struct{}{}
		for _, u := range slugUpdates[slug] {
			if c.name == "" {
				c.kind = u.Type
				c.name = strings.TrimSpace(u.Item.Name)
				c.description = strings.TrimSpace(u.Item.Description)
			}
			c.aliases = appendUniqueFold(c.aliases, u.Item.Aliases...)
			c.chunks += len(u.SourceChunks)
			if t := strings.TrimSpace(u.DocTitle); t != "" {
				if _, ok := seenDoc[t]; !ok {
					seenDoc[t] = struct{}{}
					c.docs = append(c.docs, t)
				}
			}
		}
		if c.name == "" {
			c.name = slug
		}
		out = append(out, c)
	}
	return out
}

func appendUniqueFold(dst []string, add ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(add))
	for _, s := range dst {
		seen[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		dst = append(dst, s)
	}
	return dst
}

func formatCurationCandidates(cs []curationCandidate) string {
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "- slug: %s | type: %s | name: %s", c.slug, c.kind, c.name)
		if len(c.aliases) > 0 {
			fmt.Fprintf(&b, " | aliases: %s", strings.Join(c.aliases, ", "))
		}
		fmt.Fprintf(&b, " | cited_chunks: %d | about: %s", c.chunks, previewText(c.description, 140))
		if len(c.docs) > 0 {
			fmt.Fprintf(&b, " | source: %s", previewText(strings.Join(c.docs, "; "), 80))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatExistingPagesForCuration(existing map[string]string) string {
	if len(existing) == 0 {
		return "(none yet)"
	}
	slugs := make([]string, 0, len(existing))
	for slug := range existing {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var b strings.Builder
	for _, slug := range slugs {
		fmt.Fprintf(&b, "- %s | %s\n", slug, previewText(existing[slug], 80))
	}
	return b.String()
}

func parseCurationDecisions(raw string) []curationDecision {
	raw = cleanLLMJSON(raw)
	if raw == "" {
		return nil
	}
	var parsed struct {
		Decisions []curationDecision `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	out := parsed.Decisions[:0]
	for _, d := range parsed.Decisions {
		d.Slug = strings.TrimSpace(d.Slug)
		d.Action = strings.ToLower(strings.TrimSpace(d.Action))
		d.Into = strings.TrimSpace(d.Into)
		d.Reason = strings.TrimSpace(d.Reason)
		if d.Slug == "" || (d.Action != curationDrop && d.Action != curationMerge) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// applyCuration validates decisions and applies the valid ones to slugUpdates.
//
//   - protected slugs (pages that already exist in the KB) are never dropped or
//     merged away: they carry earlier documents' contributions. They can be a
//     merge target.
//   - a merge needs a target that is a surviving candidate or an existing page,
//     of the same kind (entity/concept), reached without a cycle.
//   - if the valid changes exceed wikiCurationMaxChangeRatio of the curatable
//     candidates, nothing is applied.
//
// A merge re-slugs the candidate's updates onto the target and adds the
// candidate's name to their aliases, so the target page keeps the evidence and
// the other name stays searchable.
func applyCuration(
	slugUpdates map[string][]SlugUpdate,
	decisions []curationDecision,
	protected map[string]bool,
	existing map[string]string,
) curationReport {
	report := curationReport{Merged: map[string]string{}}

	eligible := map[string]bool{}
	for slug, updates := range slugUpdates {
		if curatable(updates) {
			eligible[slug] = true
		}
	}

	plan := map[string]curationDecision{}
	for _, d := range decisions {
		if _, dup := plan[d.Slug]; dup {
			continue
		}
		if !eligible[d.Slug] || protected[d.Slug] {
			report.Rejected++
			continue
		}
		plan[d.Slug] = d
	}

	// Resolve every merge to its final target against the full plan first, then
	// reject the invalid ones, so the outcome does not depend on map order.
	final := map[string]string{}
	invalid := map[string]bool{}
	for slug, d := range plan {
		if d.Action != curationMerge {
			continue
		}
		target, ok := resolveMergeTarget(slug, plan)
		if ok {
			_, known := existing[target]
			if !eligible[target] && !known && !protected[target] {
				ok = false
			}
		}
		if !ok {
			invalid[slug] = true
			continue
		}
		final[slug] = target
	}
	for slug := range invalid {
		delete(plan, slug)
		report.Rejected++
	}

	changes := len(plan)
	limit := int(float64(len(eligible)) * wikiCurationMaxChangeRatio)
	if limit < 1 {
		limit = 1
	}
	if changes > limit {
		report.CappedOut = true
		return report
	}

	slugs := make([]string, 0, len(plan))
	for slug := range plan {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		d := plan[slug]
		if d.Action == curationDrop {
			delete(slugUpdates, slug)
			report.Dropped = append(report.Dropped, slug)
		}
	}
	for _, slug := range slugs {
		target, merge := final[slug]
		if !merge {
			continue
		}
		moved := slugUpdates[slug]
		delete(slugUpdates, slug)
		for _, u := range moved {
			u.Item.Aliases = appendUniqueFold(append([]string(nil), u.Item.Aliases...), u.Item.Name)
			u.Slug = target
			u.Item.Slug = target
			slugUpdates[target] = append(slugUpdates[target], u)
		}
		report.Merged[slug] = target
	}
	return report
}

// resolveMergeTarget follows a merge decision to the slug it finally lands on.
// A chain (A into B, B into C) resolves to C. It fails on an empty target, a
// target of another kind, a cycle, a chain longer than wikiCurationMaxHops, or a
// target that is itself dropped.
func resolveMergeTarget(slug string, plan map[string]curationDecision) (string, bool) {
	kind := slugKind(slug)
	target := plan[slug].Into
	seen := map[string]bool{slug: true}
	for hop := 0; hop <= wikiCurationMaxHops; hop++ {
		if target == "" || seen[target] || slugKind(target) != kind {
			return "", false
		}
		seen[target] = true
		next, chained := plan[target]
		if !chained {
			return target, true
		}
		if next.Action != curationMerge {
			return "", false
		}
		target = next.Into
	}
	return "", false
}

// curateCandidates is the extension's quality pass over one batch. It edits
// slugUpdates in place and returns what it did.
func (s *wikiIngestService) curateCandidates(
	ctx context.Context,
	kb *types.KnowledgeBase,
	slugUpdates map[string][]SlugUpdate,
	call curationCaller,
) curationReport {
	cands := collectCurationCandidates(slugUpdates)
	if len(cands) == 0 || s.wikiService == nil {
		return curationReport{}
	}

	slugs := make([]string, 0, len(cands))
	for _, c := range cands {
		slugs = append(slugs, c.slug)
	}
	// Pages that already exist hold earlier documents' contributions. If we
	// cannot tell which ones they are, curating could discard those, so do nothing.
	live, err := s.wikiService.ExistsSlugs(ctx, kb.ID, slugs)
	if err != nil {
		logger.Warnf(ctx, "wiki curation skipped, cannot check existing pages: %v", err)
		return curationReport{}
	}
	protected := make(map[string]bool, len(live))
	for slug, ok := range live {
		if ok {
			protected[slug] = true
		}
	}

	existing := map[string]string{}
	for _, pageType := range []string{types.WikiPageTypeEntity, types.WikiPageTypeConcept} {
		entries, err := s.wikiService.ListByTypeRecent(ctx, kb.ID, pageType, wikiCurationExistingPerType)
		if err != nil {
			logger.Warnf(ctx, "wiki curation: list existing %s pages failed: %v", pageType, err)
			continue
		}
		for _, e := range entries {
			existing[e.Slug] = e.Title
		}
	}

	var decisions []curationDecision
	var undecided []curationCandidate
	for _, c := range cands {
		if protected[c.slug] {
			continue
		}
		if reason := heuristicJunkReason(c.name); reason != "" {
			decisions = append(decisions, curationDecision{Slug: c.slug, Action: curationDrop, Reason: reason})
			continue
		}
		undecided = append(undecided, c)
	}

	if call != nil {
		existingText := formatExistingPagesForCuration(existing)
		for start := 0; start < len(undecided); start += wikiTaxonomyPlanChunkSize {
			end := start + wikiTaxonomyPlanChunkSize
			if end > len(undecided) {
				end = len(undecided)
			}
			raw, err := call(ctx, map[string]string{
				"ExistingPages": existingText,
				"Candidates":    formatCurationCandidates(undecided[start:end]),
			})
			if err != nil {
				logger.Warnf(ctx, "wiki curation: model call failed (%d candidates): %v", end-start, err)
				continue
			}
			decisions = append(decisions, parseCurationDecisions(raw)...)
		}
	}

	report := applyCuration(slugUpdates, decisions, protected, existing)
	switch {
	case report.CappedOut:
		logger.Warnf(ctx, "wiki curation: %d proposed changes exceed %.0f%% of %d candidates, none applied",
			len(decisions), wikiCurationMaxChangeRatio*100, len(cands))
	case len(report.Dropped) > 0 || len(report.Merged) > 0:
		logger.Infof(ctx, "wiki curation: dropped %d %v, merged %d, rejected %d of %d candidates",
			len(report.Dropped), report.Dropped, len(report.Merged), report.Rejected, len(cands))
	}
	return report
}

// curationCallerFor binds a chat model to the curation prompt.
func (s *wikiIngestService) curationCallerFor(chatModel chat.Chat) curationCaller {
	if chatModel == nil {
		return nil
	}
	return func(ctx context.Context, data map[string]string) (string, error) {
		return s.generateWithTemplate(ctx, chatModel, agent.WikiCurationPrompt, data)
	}
}
