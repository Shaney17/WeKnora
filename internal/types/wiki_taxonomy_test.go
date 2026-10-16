package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestITProductPresetIsValid(t *testing.T) {
	tax, ok := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	if !ok {
		t.Fatalf("preset %q not found", WikiTaxonomyPresetITProduct)
	}
	if err := tax.Validate(); err != nil {
		t.Fatalf("built-in preset must validate: %v", err)
	}
	if got := tax.MaxDepth(); got != WikiCategoryMaxDepth {
		t.Fatalf("MaxDepth() = %d, want %d (Product is open)", got, WikiCategoryMaxDepth)
	}
}

func TestPresetReturnsIndependentCopies(t *testing.T) {
	a, _ := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	a.Roots[0].Name = "Mutated"
	b, _ := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	if b.Roots[0].Name != "Product" {
		t.Fatalf("mutating one preset copy leaked into the next: %q", b.Roots[0].Name)
	}
}

// cfg builds a WikiConfig carrying only the extension's settings.
func cfg(preset string, tax *WikiTaxonomy) *WikiConfig {
	return &WikiConfig{WikiTaxonomyConfig: WikiTaxonomyConfig{TaxonomyPreset: preset, Taxonomy: tax}}
}

func TestWikiTaxonomyOf(t *testing.T) {
	if WikiTaxonomyOf(nil) != nil {
		t.Fatal("nil config must resolve to no taxonomy")
	}
	if WikiTaxonomyOf(&WikiConfig{}) != nil {
		t.Fatal("config without preset or taxonomy must stay free-form")
	}
	if WikiTaxonomyOf(cfg("nope", nil)) != nil {
		t.Fatal("unknown preset must not resolve")
	}
	if WikiTaxonomyOf(cfg(WikiTaxonomyPresetITProduct, nil)) == nil {
		t.Fatal("known preset must resolve")
	}
	if WikiTaxonomyOf(cfg("", &WikiTaxonomy{})) != nil {
		t.Fatal("a taxonomy without folders means free-form")
	}

	custom := &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "Docs"}}}
	if got := WikiTaxonomyOf(cfg(WikiTaxonomyPresetITProduct, custom)); got != custom {
		t.Fatal("an explicit taxonomy must win over the preset")
	}
}

func TestMergeWikiTaxonomy(t *testing.T) {
	tax := &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "Docs"}}}
	prev := cfg("", tax)
	prev.MaxPagesPerIngest = 5

	// A client that never heard of the extension must not erase the layout.
	legacy := &WikiConfig{MaxPagesPerIngest: 9}
	got := MergeWikiTaxonomy(prev, legacy)
	if got.Taxonomy != tax || got.MaxPagesPerIngest != 9 {
		t.Fatalf("legacy save lost the layout or its own fields: %+v", got)
	}

	// Clearing is explicit: an empty taxonomy replaces the old one.
	cleared := MergeWikiTaxonomy(prev, cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{}}))
	if WikiTaxonomyOf(cleared) != nil {
		t.Fatal("an explicit empty taxonomy must switch the KB back to free-form")
	}

	// Naming a preset replaces the old custom layout.
	preset := MergeWikiTaxonomy(prev, cfg(WikiTaxonomyPresetITProduct, nil))
	if preset.Taxonomy != nil || preset.TaxonomyPreset != WikiTaxonomyPresetITProduct {
		t.Fatalf("a preset in the request must win over the stored layout: %+v", preset)
	}

	if MergeWikiTaxonomy(nil, legacy) != legacy || MergeWikiTaxonomy(prev, nil) != nil {
		t.Fatal("nil on either side must pass the other through")
	}
}

// The extension's fields must stay flat in the stored/served JSON, next to the
// core wiki_config keys, so the API and existing rows are unaffected by the
// embedding.
func TestWikiConfigJSONStaysFlat(t *testing.T) {
	in := cfg(WikiTaxonomyPresetITProduct, &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "Docs"}}})
	in.MaxPagesPerIngest = 3
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flat); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"taxonomy_preset", "taxonomy", "max_pages_per_ingest"} {
		if _, ok := flat[key]; !ok {
			t.Fatalf("expected a flat %q key in %s", key, raw)
		}
	}
	if _, nested := flat["WikiTaxonomyConfig"]; nested {
		t.Fatalf("the embedded struct leaked as a nested key: %s", raw)
	}

	var out WikiConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.TaxonomyPreset != WikiTaxonomyPresetITProduct || out.Taxonomy == nil || out.MaxPagesPerIngest != 3 {
		t.Fatalf("round trip lost data: %+v", out)
	}

	// Rows stored before the extension existed still load.
	var old WikiConfig
	if err := json.Unmarshal([]byte(`{"max_pages_per_ingest":7}`), &old); err != nil || old.MaxPagesPerIngest != 7 {
		t.Fatalf("pre-extension row failed to load: %v %+v", err, old)
	}
}

func TestValidateTaxonomy(t *testing.T) {
	tooDeep := &WikiTaxonomy{Roots: []WikiTaxonomyNode{
		{Name: "A", Children: []WikiTaxonomyNode{{Name: "B", Children: []WikiTaxonomyNode{{Name: "C", Children: []WikiTaxonomyNode{{Name: "D"}}}}}}},
	}}
	cases := []struct {
		name string
		cfg  *WikiConfig
		want string // substring of the error, "" = valid
	}{
		{"unknown preset", cfg("x", nil), "unknown wiki taxonomy preset"},
		{"valid preset", cfg(WikiTaxonomyPresetITProduct, nil), ""},
		{"empty name", cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: " "}}}), "must not be empty"},
		{"separator in name", cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "a/b"}}}), "not usable"},
		{"reserved type label", cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "Concepts"}}}), "not usable"},
		{"duplicate siblings", cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "A"}, {Name: "a"}}}), "two sibling"},
		{"too deep", cfg("", tooDeep), "deeper than"},
		{"open at max depth", cfg("", &WikiTaxonomy{Roots: []WikiTaxonomyNode{
			{Name: "A", Children: []WikiTaxonomyNode{{Name: "B", Children: []WikiTaxonomyNode{{Name: "C", Open: true}}}}},
		}}), "cannot be open"},
		{"summary path outside template", cfg("", &WikiTaxonomy{
			Roots: []WikiTaxonomyNode{{Name: "A"}}, SummaryPath: []string{"Missing"},
		}), "summary_path"},
		{"fallback path inside template", cfg("", &WikiTaxonomy{
			Roots: []WikiTaxonomyNode{{Name: "A"}, {Name: "B"}}, FallbackPath: []string{"b"},
		}), ""},
		{"nil config", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWikiTaxonomy(tc.cfg)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestKnowledgeBaseValidationCoversTaxonomy(t *testing.T) {
	kb := &KnowledgeBase{WikiConfig: cfg("typo", nil)}
	if err := ValidateKnowledgeBasePromptInstructions(kb); err == nil {
		t.Fatal("a KB with an unknown taxonomy preset must fail validation")
	}
}

func TestConform(t *testing.T) {
	tax, _ := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	cases := []struct {
		name string
		in   []string
		want string // "/"-joined; "<nil>" for nil
	}{
		{"fixed branch", []string{"Technical", "Services"}, "Technical/Services"},
		{"case is rewritten to template spelling", []string{"technical", "interfaces"}, "Technical/Interfaces"},
		{"unknown child of a fixed folder is cut back", []string{"Technical", "Random"}, "Technical"},
		{"open folder keeps planner labels", []string{"Product", "Trading", "Order management"}, "Product/Trading/Order management"},
		{"open folder depth is capped", []string{"Product", "A", "B", "C"}, "Product/A/B"},
		{"unknown root falls back", []string{"Misc"}, "Unsorted"},
		{"empty falls back", nil, "Unsorted"},
		{"only type labels fall back", []string{"entity"}, "Unsorted"},
		{"fixed root alone is kept", []string{"Sources"}, "Sources"},
		{"labels below a leaf fixed folder are dropped", []string{"Sources", "Doc"}, "Sources"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(tax.Conform(tc.in), "/")
			if got != tc.want {
				t.Fatalf("Conform(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestConformWithoutFallbackLeavesItemUnfiled(t *testing.T) {
	tax := &WikiTaxonomy{Roots: []WikiTaxonomyNode{{Name: "A"}}}
	if got := tax.Conform([]string{"Other"}); got != nil {
		t.Fatalf("Conform() = %v, want nil when there is no fallback", got)
	}
	if got := tax.Conform(nil); got != nil {
		t.Fatalf("Conform(nil) = %v, want nil when there is no fallback", got)
	}
}

func TestConformDoesNotAliasFallbackPath(t *testing.T) {
	tax, _ := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	got := tax.Conform(nil)
	got[0] = "Changed"
	if tax.FallbackPath[0] != "Unsorted" {
		t.Fatalf("Conform returned a slice aliasing FallbackPath: %v", tax.FallbackPath)
	}
}

func TestFixedPathsListParentsFirst(t *testing.T) {
	tax, _ := WikiTaxonomyPreset(WikiTaxonomyPresetITProduct)
	seen := map[string]bool{}
	for _, p := range tax.FixedPaths() {
		if len(p) > 1 && !seen[strings.Join(p[:len(p)-1], "/")] {
			t.Fatalf("path %v listed before its parent", p)
		}
		seen[strings.Join(p, "/")] = true
	}
	for _, want := range []string{"Product", "Technical", "Technical/Services", "Technical/Operations", "Sources", "Unsorted"} {
		if !seen[want] {
			t.Fatalf("FixedPaths() is missing %q", want)
		}
	}
	if seen["Product/Trading"] {
		t.Fatal("open folders must not get invented children")
	}
}

// Upstream's PATCH endpoint decodes request sections on top of the stored value.
// A layout must replace, not overlay: nothing from the old one may survive.
func TestWikiTaxonomyDecodesWithReplaceSemantics(t *testing.T) {
	stored := &WikiConfig{MaxPagesPerIngest: 4}
	stored.Taxonomy = &WikiTaxonomy{
		Roots:        []WikiTaxonomyNode{{Name: "Product", Open: true, Description: "old"}, {Name: "Sources"}},
		SummaryPath:  []string{"Sources"},
		FallbackPath: []string{"Product"},
	}
	shared := stored.Taxonomy

	// A patch that sends a new layout without summary/fallback targets or flags.
	patch := `{"taxonomy":{"roots":[{"name":"Docs"}]}}`
	if err := json.Unmarshal([]byte(patch), stored); err != nil {
		t.Fatal(err)
	}
	got := stored.Taxonomy
	if len(got.Roots) != 1 || got.Roots[0].Name != "Docs" || got.Roots[0].Open || got.Roots[0].Description != "" {
		t.Fatalf("new layout inherited old node data: %+v", got.Roots)
	}
	if got.SummaryPath != nil || got.FallbackPath != nil {
		t.Fatalf("omitted targets must not survive from the old layout: %+v", got)
	}
	if stored.MaxPagesPerIngest != 4 {
		t.Fatal("a patch that only carries a layout must leave the other wiki fields alone")
	}
	_ = shared

	// A patch that does not mention the layout leaves it alone.
	if err := json.Unmarshal([]byte(`{"max_pages_per_ingest":9}`), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Taxonomy == nil || len(stored.Taxonomy.Roots) != 1 || stored.MaxPagesPerIngest != 9 {
		t.Fatalf("an unrelated patch disturbed the layout: %+v", stored)
	}

	// null clears it.
	if err := json.Unmarshal([]byte(`{"taxonomy":null}`), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Taxonomy != nil {
		t.Fatal("an explicit null must clear the layout")
	}
}
