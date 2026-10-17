package types

import (
	"strings"
	"testing"
)

func TestWithWikiTaxonomyGuidance(t *testing.T) {
	withLayout := cfg(WikiTaxonomyPresetITProduct, nil)

	c, e := WithWikiTaxonomyGuidance(withLayout, "", "")
	if c != WikiContentGuidance || e != WikiExtractionGuidance {
		t.Fatalf("with no user instructions the built-in guidance stands alone: %q / %q", c, e)
	}

	c, e = WithWikiTaxonomyGuidance(withLayout, "  Prefer short pages. ", "Skip appendices.")
	if !strings.HasPrefix(c, WikiContentGuidance) || !strings.HasSuffix(c, "Prefer short pages.") {
		t.Fatalf("user content instructions must follow the built-in ones, trimmed: %q", c)
	}
	if !strings.HasPrefix(e, WikiExtractionGuidance) || !strings.HasSuffix(e, "Skip appendices.") {
		t.Fatalf("user extraction instructions must follow the built-in ones: %q", e)
	}

	// No layout: not our KB, nothing changes, not even whitespace.
	for _, free := range []*WikiConfig{nil, {}, cfg("", &WikiTaxonomy{})} {
		c, e = WithWikiTaxonomyGuidance(free, " keep ", "")
		if c != " keep " || e != "" {
			t.Fatalf("a KB without a layout must be untouched, got %q / %q", c, e)
		}
	}
}

// The guidance goes into every extraction and writing call, so it must stay small
// and must not smuggle in output-format instructions that fight the system prompt.
func TestWikiGuidanceStaysCompact(t *testing.T) {
	for name, text := range map[string]string{"content": WikiContentGuidance, "extraction": WikiExtractionGuidance} {
		if n := len([]rune(text)); n > 2000 {
			t.Errorf("%s guidance is %d characters; it is paid for on every LLM call", name, n)
		}
		if strings.Contains(strings.ToLower(text), "json") {
			t.Errorf("%s guidance must not talk about output format", name)
		}
	}
}
