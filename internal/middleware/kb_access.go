package middleware

import (
	"context"
	stderrors "errors"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// KB access is resolved once here for both path-guarded and body-addressed
// operations. Handlers consume the typed result instead of interpreting
// ownership, organization shares, or shared-Agent access themselves.

// KBAccess captures the result of a successful KB access resolution.
// Stashed on gin.Context under KBAccessContextKey so handlers that
// need the resolved KB / permission (e.g. to render
// my_permission in the response) can pull it without re-running the
// resolution.
type KBAccess struct {
	KnowledgeBase     *types.KnowledgeBase
	EffectiveTenantID uint64
	Permission        types.OrgMemberRole
	SharedAgentID     string
}

// KBAccessContextKey is the gin.Context key under which a successful
// KB access resolution is stored.
const KBAccessContextKey = "rbac.kb_access"

// KBAccessFromContext returns the KBAccess stashed by the guard, if
// any. Handlers that don't care can just rely on the rewritten
// c.Request.Context() for tenant scoping.
func KBAccessFromContext(c *gin.Context) (*KBAccess, bool) {
	v, ok := c.Get(KBAccessContextKey)
	if !ok {
		return nil, false
	}
	a, ok := v.(*KBAccess)
	return a, ok
}

// KBLookup is the minimum surface ResolveKBAccess needs from the
// knowledge-base service: a single method that turns an ID into a
// KnowledgeBase pointer (or repo.ErrKnowledgeBaseNotFound). Defining
// it as a tiny dedicated interface keeps the guard testable without
// forcing test stubs to satisfy the full KnowledgeBaseService surface.
type KBLookup interface {
	GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error)
}

type KBSourceLookup interface {
	GetKnowledgeBaseByIDOnly(ctx context.Context, id string) (*types.KnowledgeBase, error)
}

// KnowledgeLookup mirrors KBLookup but for resolving a knowledge id
// (document id) back to its parent KB. Used by the chunk routes whose
// URL param is a knowledge_id, not a kb_id.
type KnowledgeLookup interface {
	GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error)
}

// ChunkLookup mirrors KBLookup for resolving a chunk id back to its
// owning knowledge document, which then resolves to the parent KB.
// Used by the /chunks/by-id/:id routes that address chunks directly.
type ChunkLookup interface {
	GetChunkByIDOnly(ctx context.Context, id string) (*types.Chunk, error)
}

// KBIDResolver tells the guard how to find the kb_id for a given
// request. Built-in resolvers below cover the param shapes we use:
// :id, :kb_id, :kbId, :knowledge_id (-> parent KB).
//
// On error, resolvers MUST return either a 4xx apperror (bad request /
// not found) or a generic Go error for transient/internal failures;
// the guard maps the latter to 503.
type KBIDResolver func(c *gin.Context) (string, error)

// KBIDFromParam returns a resolver that reads a fixed gin param.
func KBIDFromParam(param string) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		return v, nil
	}
}

// KBIDFromKnowledgeIDParam reads `:knowledge_id` from the URL, looks
// up the knowledge document, and returns its KB id. Used by the chunk
// routes that address a chunk via /chunks/:knowledge_id.
//
// A genuine "not found" maps to 404; transient errors (DB hiccup,
// service unavailable) are surfaced as a plain Go error so the guard
// can return 503 instead of pretending the resource doesn't exist
// (a 404 here would also short-circuit any retry / monitoring).
func KBIDFromKnowledgeIDParam(param string, kgService KnowledgeLookup) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		k, err := kgService.GetKnowledgeByIDOnly(c.Request.Context(), v)
		if err != nil {
			if isResourceNotFound(err) {
				return "", apperrors.NewNotFoundError("Knowledge not found")
			}
			return "", err
		}
		if k == nil {
			return "", apperrors.NewNotFoundError("Knowledge not found")
		}
		return k.KnowledgeBaseID, nil
	}
}

// KBIDFromChunkIDParam walks chunk_id -> knowledge_id -> kb_id.
// Used by /chunks/by-id/:id routes that address a chunk directly. The
// chunk's KnowledgeBaseID is denormalised on the row, so a single
// lookup is enough — no need to chain through GetKnowledgeByIDOnly.
//
// Not-found / transient split mirrors KBIDFromKnowledgeIDParam.
func KBIDFromChunkIDParam(param string, chunkService ChunkLookup) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		ch, err := chunkService.GetChunkByIDOnly(c.Request.Context(), v)
		if err != nil {
			if isResourceNotFound(err) {
				return "", apperrors.NewNotFoundError("Chunk not found")
			}
			return "", err
		}
		if ch == nil {
			return "", apperrors.NewNotFoundError("Chunk not found")
		}
		if ch.KnowledgeBaseID == "" {
			// Should-never-happen on a fresh schema; on legacy rows the
			// chunk effectively isn't resolvable to a KB so the client
			// gets the same 404 they'd get for a missing chunk rather
			// than a 500 that pollutes alerting.
			logger.Warnf(c.Request.Context(),
				"[kb_access] chunk %s has empty knowledge_base_id; treating as not-found", v)
			return "", apperrors.NewNotFoundError("Chunk not found")
		}
		return ch.KnowledgeBaseID, nil
	}
}

// isResourceNotFound recognises the various "not found" sentinels we
// might see from the underlying services. Keeps the resolvers above
// from forcing every service to standardise on a single error type
// before this refactor is useful.
func isResourceNotFound(err error) bool {
	// ErrChunkNotFound is defined in the repository layer and aliased by the
	// service; match the canonical repo sentinel so this predicate depends
	// only on the repository package (KB / Knowledge / Chunk are all here).
	return stderrors.Is(err, apprepo.ErrKnowledgeBaseNotFound) ||
		stderrors.Is(err, apprepo.ErrKnowledgeNotFound) ||
		stderrors.Is(err, apprepo.ErrChunkNotFound)
}

// RequireKBAccess returns a gin.HandlerFunc that resolves KB access
// (own / org-shared / via shared agent), enforces the minimum required
// org-level permission, and on success stores the result under
// KBAccessContextKey AND rewrites c.Request.Context() to carry the
// effective tenant ID. Handlers downstream just read tenant from
// context as before.
//
// On failure the guard aborts with the appropriate HTTP status (400 /
// 401 / 404 / 403 / 503). Behaviour matches what each handler's
// effectiveCtxForKB helper used to do; the guard is what consolidates
// the repetition so a fix in the resolution order propagates to every
// gated route at once.
//
// Required permission semantics:
//   - OrgRoleViewer -> read-only routes (the agent-share fallback path
//     activates only at this level)
//   - OrgRoleEditor -> mutating routes (org-shared editor or own KB)
//   - OrgRoleAdmin  -> share-management routes (only the original
//     sharer / KB owner / Org admin should pass)
func RequireKBAccess(
	resolveKBID KBIDResolver,
	requiredPermission types.OrgMemberRole,
	kbService KBLookup,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
	governanceService interfaces.KnowledgeGovernanceService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		kbID, err := resolveKBID(c)
		if err != nil {
			_ = c.Error(err)
			c.Abort()
			return
		}

		access, err := ResolveKBAccess(c, kbID, requiredPermission, kbService, kbShareService, agentShareService, governanceService)
		if err != nil {
			_ = c.Error(err)
			c.Abort()
			return
		}

		// Stash the resolution and rewrite the request to carry the
		// effective tenant id. Handlers reading tenant from context now
		// see the source-tenant for shared KBs (so retrieval queries
		// hit the right embedding store) without having to know.
		c.Set(KBAccessContextKey, access)
		ctx := c.Request.Context()
		callerTenantID, _ := types.TenantIDFromContext(ctx)
		newCtx := ctx
		if access.KnowledgeBase != nil && callerTenantID != 0 && access.EffectiveTenantID != callerTenantID {
			if access.SharedAgentID != "" {
				newCtx = types.WithAuthorizedSharedAgentExecution(
					newCtx, callerTenantID, access.EffectiveTenantID, access.SharedAgentID,
				)
			} else {
				newCtx = types.WithAuthorizedSharedKnowledgeBase(
					newCtx, callerTenantID, access.EffectiveTenantID, access.KnowledgeBase.ID, access.Permission,
				)
			}
		}
		newCtx = context.WithValue(newCtx, types.TenantIDContextKey, access.EffectiveTenantID)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
	}
}

// ResolveKBAccess is the only interpreter for own, organization-shared, and
// shared-Agent KB access. Body-addressed handlers call it directly; route
// guards call it before stashing the result on gin.Context.
func ResolveKBAccess(
	c *gin.Context,
	kbID string,
	requiredPermission types.OrgMemberRole,
	kbService KBLookup,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
	governanceService interfaces.KnowledgeGovernanceService,
) (*KBAccess, error) {
	ctx := c.Request.Context()
	if err := types.AuthorizeTenantAPIKeyKnowledgeBases(ctx, kbID); err != nil {
		return nil, err
	}
	access, err := resolveKBAccessOnce(ctx, c, kbID, requiredPermission, kbService, kbShareService, agentShareService, governanceService)
	switch {
	case stderrors.Is(err, errKBAccessUnauthorized):
		return nil, apperrors.NewUnauthorizedError("Unauthorized")
	case stderrors.Is(err, errKBAccessNotFound):
		return nil, apperrors.NewNotFoundError("knowledge base not found")
	case stderrors.Is(err, errKBAccessAPIKeyForeign):
		return nil, apperrors.NewForbiddenError("API keys cannot access knowledge bases from another tenant")
	case stderrors.Is(err, errKBAccessForbidden):
		return nil, apperrors.NewForbiddenError("Permission denied to access this knowledge base")
	case stderrors.Is(err, errKBAccessBadRequest):
		return nil, apperrors.NewBadRequestError("invalid agent_source_tenant_id")
	case err != nil:
		logger.ErrorWithFields(ctx, err, nil)
		return nil, apperrors.NewServiceUnavailableError("cannot verify KB access right now")
	default:
		return access, nil
	}
}

// resolveKBAccessOnce performs the actual three-step resolution. Kept
// unexported and using package-private sentinel errors so the guard's
// error mapping is the only public surface.
//
// The shared-agent step honours the ?agent_id query parameter when
// present, mirroring the in-handler resolution in
// knowledgebase.go's validateAndGetKnowledgeBase: a request with a
// specific agent_id is validated against THAT agent's KB scope (mode =
// all / selected / none), not against "any shared agent". Falling
// back to "any shared agent" when agent_id is empty preserves the
// "通过智能体可见" KB list entry point.
func resolveKBAccessOnce(
	ctx context.Context,
	c *gin.Context,
	kbID string,
	requiredPermission types.OrgMemberRole,
	kbService KBLookup,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
	governanceService interfaces.KnowledgeGovernanceService,
) (*KBAccess, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, errKBAccessUnauthorized
	}
	callerTenantRole := types.TenantRoleFromContext(ctx)

	// The normal service method already applies KnowledgeAccess; using it here
	// would turn a revoked own-KB read into a misleading 404 and then duplicate
	// the policy call below. Resolve the raw source once, then authorize once.
	var kb *types.KnowledgeBase
	var err error
	if source, ok := kbService.(KBSourceLookup); ok {
		kb, err = source.GetKnowledgeBaseByIDOnly(ctx, kbID)
	} else {
		kb, err = kbService.GetKnowledgeBaseByID(ctx, kbID)
	}
	if err != nil {
		if stderrors.Is(err, apprepo.ErrKnowledgeBaseNotFound) {
			return nil, errKBAccessNotFound
		}
		return nil, err
	}
	if kb == nil {
		return nil, errKBAccessNotFound
	}
	if _, isAPIKey := types.TenantAPIKeyScopeFromContext(ctx); isAPIKey && kb.TenantID != tenantID {
		return nil, errKBAccessAPIKeyForeign
	}

	// 1. Own KB.
	if kb.TenantID == tenantID {
		if governanceService != nil {
			userID, _ := types.UserIDFromContext(ctx)
			allowed, accessErr := governanceService.CanAccessKnowledgeBase(ctx, tenantID, userID, callerTenantRole, kbID)
			if accessErr != nil {
				return nil, accessErr
			}
			if !allowed {
				return nil, errKBAccessForbidden
			}
		}
		return &KBAccess{
			KnowledgeBase:     kb,
			EffectiveTenantID: tenantID,
			Permission:        types.OrgRoleAdmin,
		}, nil
	}

	// 2. Org-shared KB. Plan 3's 3-D cap is applied inside
	//    CheckTenantKBPermission; we just check the result satisfies
	//    the minimum requirement.
	// Knowledge Administrators may maintain every existing KB in their own
	// enterprise, but never gain a new cross-enterprise write path through a
	// share. Admin+ retains the established shared-editor behaviour.
	if requiredPermission == types.OrgRoleEditor && callerTenantRole == types.TenantRoleAdmin {
		return nil, errKBAccessForbidden
	}
	if kbShareService != nil {
		permission, isShared, permErr := kbShareService.CheckTenantKBPermission(ctx, kbID, tenantID, callerTenantRole)
		if permErr == nil && isShared && permission.HasPermission(requiredPermission) {
			source, srcErr := kbShareService.GetKBSourceTenant(ctx, kbID)
			if srcErr == nil {
				logger.Infof(ctx, "[kb_access] tenant %d -> shared KB %s perm=%s source=%d",
					tenantID, kbID, permission, source)
				return &KBAccess{
					KnowledgeBase:     kb,
					EffectiveTenantID: source,
					Permission:        permission,
				}, nil
			}
		}
	}

	// 3. Shared agent that carries this KB — only ever grants read.
	if requiredPermission == types.OrgRoleViewer && agentShareService != nil {
		access, agentErr := resolveSharedAgentAccess(ctx, c, tenantID, callerTenantRole, kb, agentShareService)
		if agentErr != nil {
			return nil, agentErr
		}
		if access != nil {
			return access, nil
		}
	}

	logger.Warnf(ctx, "[kb_access] tenant %d -> KB %s denied (required=%s)", tenantID, kbID, requiredPermission)
	return nil, errKBAccessForbidden
}

// resolveSharedAgentAccess implements the agent-share fallback,
// mirroring knowledgebase.go's in-handler logic so the guard and the
// (still-present) handler resolver agree on which requests pass.
//
//   - If ?agent_id=X is provided, validate against THAT agent's
//     KBSelectionMode (all / selected / none). A mismatch means deny;
//     we do NOT fall back to "any shared agent" because the client
//     explicitly named one (typically from a @-mention or a deep link
//     scoped to that agent).
//   - If ?agent_id is empty, allow when any shared agent reachable by
//     the caller can access this KB ("通过智能体可见" KB list entry).
func resolveSharedAgentAccess(
	ctx context.Context,
	c *gin.Context,
	tenantID uint64,
	callerTenantRole types.TenantRole,
	kb *types.KnowledgeBase,
	agentShareService interfaces.AgentShareService,
) (*KBAccess, error) {
	agentID := c.Query("agent_id")
	if agentID != "" {
		sourceTenantID, err := types.ParseAgentSourceTenantID(c.Query(types.AgentSourceTenantIDParam))
		if err != nil {
			return nil, errKBAccessBadRequest
		}
		agent, err := agentShareService.GetSharedAgentForTenant(ctx, tenantID, callerTenantRole, agentID, sourceTenantID)
		if err != nil || agent == nil {
			return nil, nil
		}
		if !tools.SharedAgentAllowsKnowledgeBase(agent, kb) {
			logger.Warnf(ctx, "[kb_access] shared agent workspace mismatch: kb=%s kb.tenant=%d agent.tenant=%d",
				kb.ID, kb.TenantID, agent.TenantID)
			return nil, nil
		}
		logger.Infof(ctx, "[kb_access] tenant %d -> KB %s via shared agent %s",
			tenantID, kb.ID, agentID)
		return &KBAccess{
			KnowledgeBase:     kb,
			EffectiveTenantID: kb.TenantID,
			Permission:        types.OrgRoleViewer,
			SharedAgentID:     agent.ID,
		}, nil
	}

	agent, err := agentShareService.FindSharedAgentForKnowledgeBase(ctx, tenantID, callerTenantRole, kb)
	if err == nil && agent != nil {
		logger.Infof(ctx, "[kb_access] tenant %d -> KB %s via some shared agent", tenantID, kb.ID)
		return &KBAccess{
			KnowledgeBase:     kb,
			EffectiveTenantID: kb.TenantID,
			Permission:        types.OrgRoleViewer,
			SharedAgentID:     agent.ID,
		}, nil
	}
	return nil, nil
}

var (
	errKBAccessUnauthorized  = stderrors.New("kb_access: unauthorized")
	errKBAccessNotFound      = stderrors.New("kb_access: not found")
	errKBAccessAPIKeyForeign = stderrors.New("kb_access: api key foreign tenant")
	errKBAccessForbidden     = stderrors.New("kb_access: forbidden")
	errKBAccessBadRequest    = stderrors.New("kb_access: bad request")
)
