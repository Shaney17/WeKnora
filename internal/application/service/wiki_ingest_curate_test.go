package service

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestHeuristicJunkReason(t *testing.T) {
	junk := map[string]string{
		"https://x.example/ws":        "url_fragment",
		"xobniws-sit.tcex.com.vn":     "url_fragment",
		"www.example.com":             "url_fragment",
		"order_1":                     "sample_value",
		"Order 22":                    "sample_value",
		"C01770621995764122000040302": "sample_value",
		"user@example.com":            "sample_value",
		"2026-02-09":                  "sample_value",
		"  ":                          "empty",
	}
	for name, want := range junk {
		if got := heuristicJunkReason(name); got != want {
			t.Errorf("heuristicJunkReason(%q) = %q, want %q", name, got, want)
		}
	}
	// Names that look technical but are real subjects must pass.
	for _, name := range []string{
		"Matching Engine", "Lệnh giới hạn", "ORDER_MATCHED (PARTIAL_MATCHED)", "Node.js", "Redis Lock",
		"Trading Orchestrator", "BTC_VND", "Cấu hình thuế", "X.Ops", "Kafka", "TCI3", "iXu", "Step size",
	} {
		if got := heuristicJunkReason(name); got != "" {
			t.Errorf("heuristicJunkReason(%q) = %q, want it kept", name, got)
		}
	}
}

func TestParseCurationDecisions(t *testing.T) {
	raw := "```json\n{\"decisions\":[" +
		"{\"slug\":\" entity/a \",\"action\":\"DROP\",\"reason\":\"person_or_team\"}," +
		"{\"slug\":\"concept/b\",\"action\":\"merge\",\"into\":\"concept/c\"}," +
		"{\"slug\":\"concept/d\",\"action\":\"keep\"}," +
		"{\"slug\":\"\",\"action\":\"drop\"}," +
		"{\"slug\":\"concept/e\",\"action\":\"explode\"}" +
		"]}\n```"
	got := parseCurationDecisions(raw)
	if len(got) != 2 {
		t.Fatalf("want the 2 actionable decisions, got %+v", got)
	}
	if got[0].Slug != "entity/a" || got[0].Action != "drop" {
		t.Fatalf("decision 0 not normalised: %+v", got[0])
	}
	if got[1].Into != "concept/c" {
		t.Fatalf("merge target lost: %+v", got[1])
	}
	if parseCurationDecisions("not json") != nil || parseCurationDecisions("") != nil {
		t.Fatal("garbage must yield no decisions")
	}
}

func addition(slug, name, doc string, chunks ...string) SlugUpdate {
	kind := types.WikiPageTypeConcept
	if strings.HasPrefix(slug, "entity/") {
		kind = types.WikiPageTypeEntity
	}
	return SlugUpdate{
		Slug: slug, Type: kind, DocTitle: doc, KnowledgeID: "k-" + doc,
		Item:         extractedItem{Slug: slug, Name: name, Description: name + " description", SourceChunks: chunks},
		SourceChunks: chunks,
	}
}

func batch(updates ...SlugUpdate) map[string][]SlugUpdate {
	m := map[string][]SlugUpdate{}
	for _, u := range updates {
		m[u.Slug] = append(m[u.Slug], u)
	}
	return m
}

func slugsOf(m map[string][]SlugUpdate) []string {
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestApplyCurationDropsAndMerges(t *testing.T) {
	m := batch(
		addition("entity/order-1", "order_1", "D1", "c1"),
		addition("entity/matching-engine", "Matching Engine", "D1", "c2", "c3"),
		addition("concept/limit-order", "Limit Order (LO)", "D1", "c4"),
		addition("concept/lenh-gioi-han", "Lệnh giới hạn", "D2", "c5"),
		addition("concept/other-a", "Other A", "D1", "c6"),
		addition("concept/other-b", "Other B", "D1", "c7"),
	)
	decisions := []curationDecision{
		{Slug: "entity/order-1", Action: curationDrop, Reason: "sample_value"},
		{Slug: "concept/lenh-gioi-han", Action: curationMerge, Into: "concept/limit-order"},
	}
	rep := applyCuration(m, decisions, nil, nil)

	if !reflect.DeepEqual(rep.Dropped, []string{"entity/order-1"}) {
		t.Fatalf("dropped = %v", rep.Dropped)
	}
	if rep.Merged["concept/lenh-gioi-han"] != "concept/limit-order" {
		t.Fatalf("merged = %v", rep.Merged)
	}
	if _, still := m["entity/order-1"]; still {
		t.Fatal("dropped candidate must leave the batch")
	}
	if _, still := m["concept/lenh-gioi-han"]; still {
		t.Fatal("merged candidate must leave the batch under its own slug")
	}
	into := m["concept/limit-order"]
	if len(into) != 2 {
		t.Fatalf("target should now carry both contributions, got %d", len(into))
	}
	if into[0].Item.Name != "Limit Order (LO)" {
		t.Fatalf("target's own update must stay first (it names the page), got %q", into[0].Item.Name)
	}
	moved := into[1]
	if moved.Slug != "concept/limit-order" || moved.Item.Slug != "concept/limit-order" {
		t.Fatalf("moved update was not re-slugged: %+v", moved)
	}
	if moved.KnowledgeID != "k-D2" || len(moved.SourceChunks) != 1 || moved.SourceChunks[0] != "c5" {
		t.Fatalf("moved update lost its evidence: %+v", moved)
	}
	if !containsFold(moved.Item.Aliases, "Lệnh giới hạn") {
		t.Fatalf("the merged name must become an alias, got %v", moved.Item.Aliases)
	}
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func TestApplyCurationMergeIntoAnExistingPage(t *testing.T) {
	m := batch(
		addition("concept/lenh-lo", "Lệnh LO", "D1", "c1"),
		addition("concept/x", "X", "D1"), addition("concept/y", "Y", "D1"), addition("concept/z", "Z", "D1"),
	)
	existing := map[string]string{"concept/limit-order": "Limit Order"}
	rep := applyCuration(m, []curationDecision{{Slug: "concept/lenh-lo", Action: curationMerge, Into: "concept/limit-order"}}, nil, existing)
	if rep.Merged["concept/lenh-lo"] != "concept/limit-order" {
		t.Fatalf("merge into an existing page was refused: %+v", rep)
	}
	if got := m["concept/limit-order"]; len(got) != 1 || got[0].Slug != "concept/limit-order" {
		t.Fatalf("existing page should receive the contribution: %+v", got)
	}
}

func TestApplyCurationGuardRails(t *testing.T) {
	newBatch := func() map[string][]SlugUpdate {
		return batch(
			addition("entity/a", "A", "D"), addition("entity/b", "B", "D"),
			addition("entity/c", "C", "D"), addition("entity/d", "D", "D"),
			addition("concept/k", "K", "D"), addition("concept/m", "M", "D"),
		)
	}
	cases := []struct {
		name      string
		decisions []curationDecision
		protected map[string]bool
		existing  map[string]string
		wantGone  []string // slugs that must be removed from the batch
		wantKept  []string
	}{
		{"unknown slug is ignored", []curationDecision{{Slug: "entity/zzz", Action: curationDrop}}, nil, nil, nil, []string{"entity/a"}},
		{"protected page is never dropped", []curationDecision{{Slug: "entity/a", Action: curationDrop}}, map[string]bool{"entity/a": true}, nil, nil, []string{"entity/a"}},
		{"protected page is never merged away", []curationDecision{{Slug: "entity/a", Action: curationMerge, Into: "entity/b"}}, map[string]bool{"entity/a": true}, nil, nil, []string{"entity/a"}},
		{"merge into itself is refused", []curationDecision{{Slug: "entity/a", Action: curationMerge, Into: "entity/a"}}, nil, nil, nil, []string{"entity/a"}},
		{"merge across kinds is refused", []curationDecision{{Slug: "entity/a", Action: curationMerge, Into: "concept/k"}}, nil, nil, nil, []string{"entity/a", "concept/k"}},
		{"merge into an unknown target is refused", []curationDecision{{Slug: "entity/a", Action: curationMerge, Into: "entity/nowhere"}}, nil, nil, nil, []string{"entity/a"}},
		{"merge into a dropped candidate is refused", []curationDecision{
			{Slug: "entity/a", Action: curationMerge, Into: "entity/b"}, {Slug: "entity/b", Action: curationDrop},
		}, nil, nil, []string{"entity/b"}, []string{"entity/a"}},
		{"a cycle is refused", []curationDecision{
			{Slug: "entity/a", Action: curationMerge, Into: "entity/b"}, {Slug: "entity/b", Action: curationMerge, Into: "entity/a"},
		}, nil, nil, nil, []string{"entity/a", "entity/b"}},
		{"a chain resolves to its end", []curationDecision{
			{Slug: "entity/a", Action: curationMerge, Into: "entity/b"}, {Slug: "entity/b", Action: curationMerge, Into: "entity/c"},
		}, nil, nil, []string{"entity/a", "entity/b"}, []string{"entity/c"}},
		{"first decision for a slug wins", []curationDecision{
			{Slug: "entity/a", Action: curationDrop}, {Slug: "entity/a", Action: curationMerge, Into: "entity/b"},
		}, nil, nil, []string{"entity/a"}, []string{"entity/b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newBatch()
			applyCuration(m, tc.decisions, tc.protected, tc.existing)
			for _, g := range tc.wantGone {
				if _, ok := m[g]; ok {
					t.Errorf("%s should have left the batch: %v", g, slugsOf(m))
				}
			}
			for _, k := range tc.wantKept {
				if _, ok := m[k]; !ok {
					t.Errorf("%s should still be in the batch: %v", k, slugsOf(m))
				}
			}
		})
	}
}

func TestApplyCurationChangeBudget(t *testing.T) {
	m := batch(
		addition("entity/a", "A", "D"), addition("entity/b", "B", "D"),
		addition("entity/c", "C", "D"), addition("entity/d", "D", "D"),
	)
	// Dropping 3 of 4 is beyond the 50% budget: the model is not to be trusted.
	rep := applyCuration(m, []curationDecision{
		{Slug: "entity/a", Action: curationDrop}, {Slug: "entity/b", Action: curationDrop}, {Slug: "entity/c", Action: curationDrop},
	}, nil, nil)
	if !rep.CappedOut || len(m) != 4 {
		t.Fatalf("an over-budget answer must change nothing: capped=%v batch=%v", rep.CappedOut, slugsOf(m))
	}
	// Dropping exactly half is allowed.
	rep = applyCuration(m, []curationDecision{
		{Slug: "entity/a", Action: curationDrop}, {Slug: "entity/b", Action: curationDrop},
	}, nil, nil)
	if rep.CappedOut || len(m) != 2 {
		t.Fatalf("half of the batch is within budget: capped=%v batch=%v", rep.CappedOut, slugsOf(m))
	}
}

func TestApplyCurationLeavesRetractionsAlone(t *testing.T) {
	m := batch(addition("entity/a", "A", "D"), addition("entity/b", "B", "D"), addition("entity/c", "C", "D"))
	m["entity/a"] = append(m["entity/a"], SlugUpdate{Slug: "entity/a", Type: "retract", KnowledgeID: "old"})
	applyCuration(m, []curationDecision{{Slug: "entity/a", Action: curationDrop}}, nil, nil)
	if _, ok := m["entity/a"]; !ok {
		t.Fatal("a slug that carries a retraction belongs to a document being re-ingested and must not be curated")
	}
}

func TestApplyCurationDoesNotShareAliasSlices(t *testing.T) {
	src := addition("concept/b", "B", "D")
	src.Item.Aliases = []string{"b1"}
	m := batch(src, addition("concept/a", "A", "D"), addition("concept/c", "C", "D"), addition("concept/d", "D", "D"))
	applyCuration(m, []curationDecision{{Slug: "concept/b", Action: curationMerge, Into: "concept/a"}}, nil, nil)
	if len(src.Item.Aliases) != 1 {
		t.Fatalf("the caller's alias slice was mutated: %v", src.Item.Aliases)
	}
}

func newCurationService(rec *folderRecorder) *wikiIngestService {
	return &wikiIngestService{wikiService: rec}
}

func TestCurateCandidatesEndToEnd(t *testing.T) {
	rec := &folderRecorder{
		live:     map[string]bool{"entity/matching-engine": true},
		existing: map[string][]types.WikiIndexEntry{types.WikiPageTypeConcept: {{Slug: "concept/limit-order", Title: "Limit Order"}}},
	}
	svc := newCurationService(rec)
	m := batch(
		addition("entity/order-1", "order_1", "D1", "c1"),                       // rule: sample value
		addition("entity/nguyen-van-a", "Nguyễn Văn A", "D1"),                   // model: person
		addition("concept/lenh-lo", "Lệnh LO", "D1", "c2"),                      // model: merge into an existing page
		addition("entity/matching-engine", "Matching Engine", "D1", "c3", "c4"), // protected, already in the wiki
		addition("concept/hold", "Hold tiền", "D1", "c5", "c6"),
		addition("concept/unhold", "Unhold", "D1", "c7"),
		addition("concept/tick", "Tick size", "D1", "c8"),
		addition("concept/step", "Step size", "D1", "c9"),
	)
	var seenPrompt map[string]string
	caller := func(_ context.Context, data map[string]string) (string, error) {
		seenPrompt = data
		return `{"decisions":[
			{"slug":"entity/nguyen-van-a","action":"drop","reason":"person_or_team"},
			{"slug":"concept/lenh-lo","action":"merge","into":"concept/limit-order","reason":"duplicate"},
			{"slug":"entity/matching-engine","action":"drop","reason":"passing_mention"}
		]}`, nil
	}
	rep := svc.curateCandidates(context.Background(), &types.KnowledgeBase{ID: "kb"}, m, caller)

	if _, ok := m["entity/order-1"]; ok {
		t.Error("the rule should have dropped order_1")
	}
	if _, ok := m["entity/nguyen-van-a"]; ok {
		t.Error("the model's drop of a person should have been applied")
	}
	if _, ok := m["entity/matching-engine"]; !ok {
		t.Error("an existing page must survive even if the model proposes dropping it")
	}
	if got := m["concept/limit-order"]; len(got) != 1 || got[0].Item.Name != "Lệnh LO" {
		t.Errorf("Lệnh LO should now feed the existing Limit Order page: %+v", got)
	}
	if len(rep.Dropped) != 2 || len(rep.Merged) != 1 {
		t.Errorf("report = %+v", rep)
	}

	// The model sees what it needs, and not what the rules already settled.
	if !strings.Contains(seenPrompt["ExistingPages"], "concept/limit-order | Limit Order") {
		t.Errorf("existing pages missing from the prompt: %q", seenPrompt["ExistingPages"])
	}
	if strings.Contains(seenPrompt["Candidates"], "order_1") {
		t.Error("candidates removed by the rules should not be sent to the model")
	}
	if strings.Contains(seenPrompt["Candidates"], "entity/matching-engine") {
		t.Error("protected pages are not curatable and should not be offered to the model")
	}
	if !strings.Contains(seenPrompt["Candidates"], "cited_chunks: 1") {
		t.Errorf("the model needs the evidence count: %q", seenPrompt["Candidates"])
	}
}

func TestCurateCandidatesFailsSafe(t *testing.T) {
	build := func() map[string][]SlugUpdate {
		return batch(addition("entity/order-1", "order_1", "D"), addition("entity/a", "A", "D"), addition("entity/b", "B", "D"))
	}

	// Cannot tell which pages exist: curating could discard earlier documents'
	// contributions, so it does nothing, not even the rules.
	m := build()
	newCurationService(&folderRecorder{existsErr: errors.New("db down")}).
		curateCandidates(context.Background(), &types.KnowledgeBase{ID: "kb"}, m, nil)
	if len(m) != 3 {
		t.Fatalf("a failed existence check must not curate: %v", slugsOf(m))
	}

	// No model: the rules still run.
	m = build()
	newCurationService(&folderRecorder{}).curateCandidates(context.Background(), &types.KnowledgeBase{ID: "kb"}, m, nil)
	if _, ok := m["entity/order-1"]; ok || len(m) != 2 {
		t.Fatalf("rules should run without a model: %v", slugsOf(m))
	}

	// A model that errors leaves the rest of the batch intact.
	m = build()
	failing := func(context.Context, map[string]string) (string, error) { return "", errors.New("429") }
	newCurationService(&folderRecorder{}).curateCandidates(context.Background(), &types.KnowledgeBase{ID: "kb"}, m, failing)
	if len(m) != 2 {
		t.Fatalf("a model failure must not lose candidates: %v", slugsOf(m))
	}

	// Nothing to curate.
	rep := newCurationService(&folderRecorder{}).curateCandidates(context.Background(), &types.KnowledgeBase{ID: "kb"}, map[string][]SlugUpdate{}, nil)
	if len(rep.Dropped) != 0 || len(rep.Merged) != 0 {
		t.Fatalf("empty batch: %+v", rep)
	}
}

// Contract with the core: the core's planBatchTaxonomy must hand a KB that has a
// layout to the extension. If a merge from upstream drops the hook, this fails.
func TestCoreTaxonomyHookDelegatesToTheExtension(t *testing.T) {
	svc := &wikiIngestService{wikiService: &folderRecorder{}}
	kb := &types.KnowledgeBase{ID: "kb", WikiConfig: &types.WikiConfig{
		WikiTaxonomyConfig: types.WikiTaxonomyConfig{TaxonomyPreset: types.WikiTaxonomyPresetITProduct},
	}}
	m := batch(
		SlugUpdate{Slug: "summary/1", Type: types.WikiPageTypeSummary},
		addition("entity/order-1", "order_1", "D"), // junk, curated away: no model is needed
	)
	plan := svc.planBatchTaxonomy(context.Background(), nil, kb, m, "English")

	if got := strings.Join(plan["summary/1"], "/"); got != "Sources" {
		t.Fatalf("the core planner did not delegate to the extension: plan = %v", plan)
	}
	if _, ok := m["entity/order-1"]; ok {
		t.Fatal("the curation pass did not edit the batch in place; the reduce phase would still write that page")
	}
}

// Contract with the core: the batch context must carry the built-in guidance for
// a KB with a layout, and only for such a KB.
func TestCoreBatchContextHookAddsGuidance(t *testing.T) {
	svc := &wikiIngestService{}
	withLayout := &types.WikiConfig{
		ContentInstructions: "Mine.",
		WikiTaxonomyConfig:  types.WikiTaxonomyConfig{TaxonomyPreset: types.WikiTaxonomyPresetITProduct},
	}
	ctx := svc.newWikiBatchContext("kb", withLayout)
	if !strings.HasPrefix(ctx.ContentInstructions, types.WikiContentGuidance) || !strings.HasSuffix(ctx.ContentInstructions, "Mine.") {
		t.Fatalf("content instructions = %q", ctx.ContentInstructions)
	}
	if !strings.HasPrefix(ctx.ExtractionInstructions, types.WikiExtractionGuidance) {
		t.Fatalf("extraction instructions = %q", ctx.ExtractionInstructions)
	}

	plain := svc.newWikiBatchContext("kb", &types.WikiConfig{ContentInstructions: "Mine."})
	if plain.ContentInstructions != "Mine." || plain.ExtractionInstructions != "" {
		t.Fatalf("a KB without a layout must keep exactly its own instructions: %+v", plain)
	}
}
