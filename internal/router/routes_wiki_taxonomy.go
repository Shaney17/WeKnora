package router

// Wiki-taxonomy extension: route registration. Kept out of routes_knowledge.go
// so the extension adds one call site in router.go and nothing else in the core
// route files. See docs/extensions/wiki-taxonomy.md.

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterWikiTaxonomyRoutes wires the global, read-only list of built-in wiki
// directory templates that the KB editor offers as starting points. Like
// RegisterImageAttrRoutes it carries no KB id and needs only the Viewer role.
func RegisterWikiTaxonomyRoutes(r *gin.RouterGroup, handler *handler.KnowledgeBaseHandler, g *rbacGuards) {
	g.apiKeyRoute(r, http.MethodGet, "/wiki-taxonomy/presets",
		apiKeyRetrieve(apiKeyFullAccess()), g.Viewer(), handler.GetWikiTaxonomyPresets)
}
