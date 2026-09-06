package router

import (
	"net/http"
	"path"
	"strings"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// Tenant roles and knowledge access are separate axes:
//   - Employee (Viewer) consumes authorized knowledge.
//   - Knowledge Administrator (Contributor) maintains content and grants on
//     existing knowledge bases, but cannot create KBs or Agents or manage the
//     workspace.
//   - Admin and Owner manage workspace resources and membership.
//   - SystemAdmin manages models, storage, vector stores and other platform
//     infrastructure; tenant roles never imply this authority.
//
// Business Roles are checked by the KB access guards only and do not grant an
// administrative role. Tenant-role guards honor EnableRBAC audit mode;
// SystemAdmin is always enforced.
type rbacGuards struct {
	cfg *config.Config

	// Services for the KB-access guard (own / org-shared / via shared
	// agent). Captured here so route lines can reference g.KBAccess()
	// without having to plumb the services through every Register*
	// function.
	kbService         middleware.KBLookup
	knowledgeService  middleware.KnowledgeLookup
	chunkService      middleware.ChunkLookup
	kbShareService    interfaces.KBShareService
	agentShareService interfaces.AgentShareService
	governanceService interfaces.KnowledgeGovernanceService

	// apiKeyAuthorizer is the single source of truth for which routes an
	// X-API-Key principal may call. Routes opt in via the apiKeyGroup
	// helpers below; anything not declared is denied by the gate. See
	// middleware.APIKeyRouteAuthorizer.
	apiKeyAuthorizer *middleware.APIKeyRouteAuthorizer
}

// newRBACGuards wires the guards from the live configuration and the
// already-built handlers. Called once from NewRouter.
func newRBACGuards(
	cfg *config.Config,
	kbService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	chunkService interfaces.ChunkService,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
	governanceService interfaces.KnowledgeGovernanceService,
) *rbacGuards {
	g := &rbacGuards{cfg: cfg, apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	g.kbService = kbService
	g.knowledgeService = knowledgeService
	g.chunkService = chunkService
	g.kbShareService = kbShareService
	g.agentShareService = agentShareService
	g.governanceService = governanceService
	return g
}

// Role-only guards — pure RequireRole convenience wrappers, named after
// the matrix entries so route lines stay readable.

func (g *rbacGuards) Viewer() gin.HandlerFunc {
	return middleware.RequireRole(types.TenantRoleViewer, g.cfg)
}

func (g *rbacGuards) Contributor() gin.HandlerFunc {
	return middleware.RequireRole(types.TenantRoleContributor, g.cfg)
}

func (g *rbacGuards) Admin() gin.HandlerFunc {
	return middleware.RequireRole(types.TenantRoleAdmin, g.cfg)
}

func (g *rbacGuards) Owner() gin.HandlerFunc {
	return middleware.RequireRole(types.TenantRoleOwner, g.cfg)
}

// TenantCreator permits tenantless onboarding while requiring Contributor+
// for a member who is already operating inside a tenant. API-key callers are
// still governed by the platform-only route policy. This keeps a Viewer from
// self-promoting into an Owner by creating another tenant without breaking
// invitation-first tenantless admission.
func (g *rbacGuards) TenantCreator() gin.HandlerFunc {
	contributor := g.Contributor()
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if _, apiKey := types.TenantAPIKeyScopeFromContext(ctx); apiKey {
			c.Next()
			return
		}
		if tenantID, ok := types.TenantIDFromContext(ctx); !ok || tenantID == 0 {
			c.Next()
			return
		}
		contributor(c)
	}
}

// API-key authorization is separate from JWT role and knowledge-access
// guards. Every API-key-accessible route declares one
// APIKeyRoutePolicy via the apiKeyGroup helpers; the gate on /api/v1 enforces
// it and denies any undeclared route by default. JWT sessions ignore all of
// this (they short-circuit the gate).
//
// Policy constructors. API keys do not reuse tenant-member roles: a key is
// either full-access, or it carries explicit capabilities. KB allow-lists are
// pure data filters applied downstream by KBAccess guards and handlers.

func apiKeyAny() middleware.APIKeyRoutePolicy {
	return middleware.APIKeyRoutePolicy{}
}

func apiKeyFullAccess() middleware.APIKeyRoutePolicy {
	return middleware.APIKeyRoutePolicy{RequireFullAccess: true}
}

func apiKeyPlatform(capabilities ...types.APIKeyCapability) middleware.APIKeyRoutePolicy {
	policy := middleware.APIKeyRoutePolicy{PlatformOnly: true}
	for _, capability := range capabilities {
		policy = policy.WithCapability(capability)
	}
	return policy
}

// apiKeyRetrieve grants read/search access to knowledge-base data.
func apiKeyRetrieve(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityRetrieve)
}

// apiKeyChat layers the "chat" capability on top of a base policy: keys that
// carry the chat capability can use the conversation flow (sessions, agent
// listing) without full tenant access.
func apiKeyChat(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityChat)
}

// apiKeyReadAgents layers the "read_agents" capability on top of a base
// policy so scoped integrations can inspect available agents without chat or
// authoring permissions.
func apiKeyReadAgents(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityReadAgents)
}

// apiKeyIngest layers the "ingest" capability on top of a base policy so a
// scoped key can write content into its allowed knowledge bases (documents,
// chunks, FAQ, tags, wiki).
func apiKeyIngest(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityIngest)
}

// apiKeyManageKnowledgeBases layers the "manage_kbs" capability on top of a
// base policy so a scoped key can manage the KB lifecycle (create/copy/
// duplicate/update/delete + config). Existing-KB operations stay bounded by
// the key's allow-list downstream; create has no source to bound.
func apiKeyManageKnowledgeBases(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageKnowledgeBases)
}

// apiKeyManageAgents layers the "manage_agents" capability on top of a base policy.
func apiKeyManageAgents(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageAgents)
}

// apiKeyMessageHistory layers the "message_history" capability on top of a
// base policy so an explicitly granted key can search or inspect tenant chat
// history without being promoted to full Owner.
func apiKeyMessageHistory(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityMessageHistory)
}

func apiKeyManageModels(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageModels)
}

func apiKeyManageMCPServices(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageMCPServices)
}

func apiKeyManageDataSources(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageDataSources)
}

func apiKeyManageChannels(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageChannels)
}

func apiKeyManageVectorStores(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageVectorStores)
}

// apiKeyManageStorageBackends layers the "manage_storage_backends" capability
// on top of a base policy so a scoped key can manage object/file storage
// backend instances without carrying vector-store or full tenant access.
func apiKeyManageStorageBackends(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageStorageBackends)
}

func apiKeyManageWebSearch(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageWebSearch)
}

func apiKeyRunEvaluations(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityRunEvaluations)
}

func apiKeyManageMembers(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageMembers)
}

func apiKeyManageSpaces(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageSpaces)
}

func apiKeyManageTenantSettings(base middleware.APIKeyRoutePolicy) middleware.APIKeyRoutePolicy {
	return base.WithCapability(types.APIKeyCapabilityManageTenantSettings)
}

// apiKeyRouteGroup wraps a *gin.RouterGroup so route registration also records
// the route's API-key policy into the authorizer. Use g.apiKeyGroup(grp,
// policy) then register the API-key-accessible routes through it; register
// API-key-denied routes on the raw *gin.RouterGroup so they stay undeclared
// (default-deny). Per-route overrides use With().
type apiKeyRouteGroup struct {
	g      *rbacGuards
	grp    *gin.RouterGroup
	policy middleware.APIKeyRoutePolicy
}

// ensureAPIKeyAuthorizer lazily allocates the authorizer so route
// registration is safe even when rbacGuards is built directly in tests
// (bypassing newRBACGuards). In production it is always pre-allocated.
func (g *rbacGuards) ensureAPIKeyAuthorizer() *middleware.APIKeyRouteAuthorizer {
	if g.apiKeyAuthorizer == nil {
		g.apiKeyAuthorizer = middleware.NewAPIKeyRouteAuthorizer()
	}
	return g.apiKeyAuthorizer
}

// apiKeyGroup returns a wrapper that declares `policy` for every route
// registered through it (unless overridden via With).
func (g *rbacGuards) apiKeyGroup(grp *gin.RouterGroup, policy middleware.APIKeyRoutePolicy) *apiKeyRouteGroup {
	g.ensureAPIKeyAuthorizer()
	return &apiKeyRouteGroup{g: g, grp: grp, policy: policy}
}

// With returns a sibling wrapper on the same gin group but with a different
// policy, for the odd route that differs from its group default (e.g. a read
// search inside an otherwise contributor-gated group).
func (a *apiKeyRouteGroup) With(policy middleware.APIKeyRoutePolicy) *apiKeyRouteGroup {
	return &apiKeyRouteGroup{g: a.g, grp: a.grp, policy: policy}
}

func (a *apiKeyRouteGroup) handle(method, rel string, handlers ...gin.HandlerFunc) gin.IRoutes {
	full := path.Join(a.grp.BasePath(), rel)
	a.g.ensureAPIKeyAuthorizer().Register(method, full, a.policy)
	return a.grp.Handle(method, rel, handlers...)
}

func (a *apiKeyRouteGroup) GET(rel string, h ...gin.HandlerFunc) gin.IRoutes {
	return a.handle(http.MethodGet, rel, h...)
}

func (a *apiKeyRouteGroup) POST(rel string, h ...gin.HandlerFunc) gin.IRoutes {
	return a.handle(http.MethodPost, rel, h...)
}

func (a *apiKeyRouteGroup) PUT(rel string, h ...gin.HandlerFunc) gin.IRoutes {
	return a.handle(http.MethodPut, rel, h...)
}

func (a *apiKeyRouteGroup) PATCH(rel string, h ...gin.HandlerFunc) gin.IRoutes {
	return a.handle(http.MethodPatch, rel, h...)
}

func (a *apiKeyRouteGroup) DELETE(rel string, h ...gin.HandlerFunc) gin.IRoutes {
	return a.handle(http.MethodDelete, rel, h...)
}

// apiKeyRoute declares a single API-key-accessible route directly on a gin
// group (for routes registered outside an apiKeyGroup, e.g. top-level r.POST).
func (g *rbacGuards) apiKeyRoute(
	grp *gin.RouterGroup, method, rel string, policy middleware.APIKeyRoutePolicy, handlers ...gin.HandlerFunc,
) gin.IRoutes {
	full := path.Join(grp.BasePath(), rel)
	g.ensureAPIKeyAuthorizer().Register(method, full, policy)
	return grp.Handle(method, rel, handlers...)
}

// assertAPIKeyPoliciesMatchRoutes verifies every declared API-key policy
// resolves to a real registered route. gin's c.FullPath() must match the
// authorizer key verbatim or the gate silently 403s the route for API keys;
// panicking here turns that latent misconfiguration into a startup failure.
func (g *rbacGuards) assertAPIKeyPoliciesMatchRoutes(engine *gin.Engine) {
	registered := map[string]struct{}{}
	for _, ri := range engine.Routes() {
		// Authorizer keys are stored normalized (trailing slash trimmed), so
		// normalize gin's reported path the same way. Otherwise a route
		// registered with a "/" rel (gin path ".../evaluation/") would look
		// missing against the normalized key (".../evaluation") even though
		// the gate — which also normalizes c.FullPath() — matches it fine.
		p := ri.Path
		if len(p) > 1 {
			p = strings.TrimRight(p, "/")
		}
		registered[ri.Method+" "+p] = struct{}{}
	}
	var missing []string
	for method, paths := range g.apiKeyAuthorizer.RegisteredRoutes() {
		for _, p := range paths {
			if _, ok := registered[method+" "+p]; !ok {
				missing = append(missing, method+" "+p)
			}
		}
	}
	if len(missing) > 0 {
		panic("api-key policy declared for non-existent route(s): " + strings.Join(missing, ", "))
	}
}

func (g *rbacGuards) SystemAdmin() gin.HandlerFunc {
	return middleware.RequireSystemAdmin(g.cfg)
}

// Tenant-access guards. Distinct from the role guards above: these
// answer the orthogonal question "may this caller touch this tenant
// at all", before role membership inside the tenant is even
// considered. Both delegate to middleware/access.go which centralises
// the cross-tenant rules so the router stays declarative.

// CrossTenant gates a route on the caller being an org-level
// superuser (CanAccessAllTenants AND EnableCrossTenantAccess). Used by
// /tenants/all, /tenants/search, POST /tenants, GET /tenants — the
// endpoints that operate across tenants. Replaces the if-blocks that
// used to live inside ListAllTenants/SearchTenants/CreateTenant.
func (g *rbacGuards) CrossTenant() gin.HandlerFunc {
	return middleware.RequireCrossTenantAccess(g.cfg)
}

// PathTenantMatch enforces that the URL :id matches the caller's
// active tenant context (cross-tenant superusers bypass). Routes apply
// it at the /tenants/:id group level so every per-tenant endpoint —
// GetTenant / UpdateTenant / DeleteTenant / member
// management / leave — shares the same check. Replaces the
// authorizeTenantAccess helper that used to live inside the tenant
// handler.
func (g *rbacGuards) PathTenantMatch() gin.HandlerFunc {
	return middleware.RequirePathTenantMatch(g.cfg)
}

// KB-access guards are orthogonal to tenant roles.
// above. They answer "can the caller's tenant operate on THIS KB?"
// taking into account three paths:
//
//   1. Own KB                         — full access (Admin)
//   2. Org-shared KB (Plan 3)         — capped permission
//   3. Visible via shared agent       — read-only
//
// On success the resolved (KB + effective tenant id + permission)
// tuple is stashed on c.Keys under middleware.KBAccessContextKey AND
// the request context's tenant ID is rewritten to the effective tenant
// — so handlers downstream just read tenant the way they always did
// (types.MustTenantIDFromContext) without knowing whether the KB is
// owned or shared.
//
// These guards replace the per-handler effectiveCtxForKB /
// validateAndGetKnowledgeBase helpers that used to be re-implemented
// in chunk.go, faq.go, tag.go, knowledge.go and knowledgebase.go;
// the share-fallback logic now lives in exactly one place
// (middleware/kb_access.go).

// KBAccessRead gates a KB-scoped read route on the caller having at
// least Viewer-level access. The agent-share fallback only activates
// at this level — Editor/Admin reads never go through "I just see it
// because someone shared an agent". The kbID is read from the gin
// param named in `param` (typically "id" for /knowledge-bases/:id/...).
func (g *rbacGuards) KBAccessRead(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromParam(param),
		types.OrgRoleViewer,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}

// KBAccessWrite gates a KB-scoped mutating route on the caller having
// at least Editor-level access (own KB or org-shared with editor).
// Used by FAQ upsert, tag CRUD, chunk update/delete, etc.
func (g *rbacGuards) KBAccessWrite(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromParam(param),
		types.OrgRoleEditor,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}

// KBAccessReadFromKnowledgeIDParam is like KBAccessRead but resolves
// the kb_id by walking a knowledge document (URL `:knowledge_id`)
// back to its parent KB. Used by the chunk routes whose URL addresses
// the chunk via /chunks/:knowledge_id rather than /knowledge-bases/:id.
func (g *rbacGuards) KBAccessReadFromKnowledgeIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromKnowledgeIDParam(param, g.knowledgeService),
		types.OrgRoleViewer,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}

// KBAccessWriteFromKnowledgeIDParam mirrors KBAccessReadFromKnowledgeIDParam
// for mutating routes (Editor minimum).
func (g *rbacGuards) KBAccessWriteFromKnowledgeIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromKnowledgeIDParam(param, g.knowledgeService),
		types.OrgRoleEditor,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}

// KBAccessReadFromChunkIDParam walks chunk_id -> kb_id (using the
// chunk's denormalised KnowledgeBaseID column). Used by
// /chunks/by-id/:id read routes.
func (g *rbacGuards) KBAccessReadFromChunkIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromChunkIDParam(param, g.chunkService),
		types.OrgRoleViewer,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}

// KBAccessWriteFromChunkIDParam — same as KBAccessReadFromChunkIDParam
// but requires Editor minimum. Used by chunk write routes that
// address the chunk via /chunks/by-id/:id.
func (g *rbacGuards) KBAccessWriteFromChunkIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromChunkIDParam(param, g.chunkService),
		types.OrgRoleEditor,
		g.kbService,
		g.kbShareService,
		g.agentShareService,
		g.governanceService,
	)
}
