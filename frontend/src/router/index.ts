import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { autoSetup, getCurrentUser, getEnterpriseSession, EnterpriseSessionRequestError, userInfoFromApi } from '@/api/auth'
import {
  consumeOperatingAnalysisHandoff,
  exchangeOperatingAnalysis,
  getOperatingAnalysisAvailability,
  OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY,
  OPERATING_ANALYSIS_HANDOFF_REF_KEY,
} from '@/api/operatingAnalysis'
import { employeeSurfaceMinRoleForPath, SETTINGS_SECTION_MIN_ROLE } from '@/config/settingsAccess'
import { DEFAULT_EMPLOYEE_WORKSPACE_PATH, loginDestination, safeReturnTo } from './safeReturnTo'

/** Lite /桌面 WebView 硬刷新时可能只打开 `/`，用 session 记住上次页面以便恢复 */
const LITE_LAST_PATH_KEY = 'weknora_lite_last_path'
const AUTO_SETUP_FAILED_KEY = 'weknora_auto_setup_failed'

function shouldTryAutoSetup() {
  return localStorage.getItem(AUTO_SETUP_FAILED_KEY) !== 'true'
}

function markAutoSetupFailed() {
  localStorage.setItem(AUTO_SETUP_FAILED_KEY, 'true')
}

function isLiteEdition(authStore: ReturnType<typeof useAuthStore>) {
  return authStore.isLiteMode || localStorage.getItem('weknora_lite_mode') === 'true'
}

function isLiteSpaDefaultEntry(to: RouteLocationNormalized) {
  return (
    to.path === '/' ||
    to.path === '/platform' ||
    to.path === '/platform/knowledge-bases' ||
    to.name === 'knowledgeBaseList'
  )
}

function isSafeLiteRestoreTarget(path: string) {
  return path.startsWith('/platform/') && !path.startsWith('/platform/organizations')
}

function hasPendingOIDCCallback() {
  if (typeof window === 'undefined') return false
  const hash = window.location.hash || ''
  return hash.includes('oidc_result=') || hash.includes('oidc_error=')
}

function employeeWorkspaceMinRole(to: RouteLocationNormalized) {
  const surfaceMinimum = employeeSurfaceMinRoleForPath(to.path)
  if (surfaceMinimum) return surfaceMinimum
  if (to.path === '/platform/settings' && typeof to.query.section === 'string') {
    return SETTINGS_SECTION_MIN_ROLE[to.query.section] ?? 'viewer'
  }
  return undefined
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/",
      redirect: DEFAULT_EMPLOYEE_WORKSPACE_PATH,
    },
    {
      path: "/login",
      name: "login",
      component: () => import("../views/auth/Login.vue"),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: "/home",
      name: "retailAgentHome",
      component: () => import("../views/home/RetailAgentHome.vue"),
      meta: { requiresInit: true, requiresAuth: true }
    },
    // Embed chat is a separate entry (embed.html + embed-main.ts), not this SPA.
    {
      path: "/register",
      name: "registerByInvite",
      // Share-link landing page reuses the Login form: the same Vue
      // component renders both modes and detects ?token=xxx on mount
      // to switch into invite-register flow. Avoids a parallel page
      // that would duplicate the OIDC / language-switch / styling
      // surface for one extra field.
      component: () => import("../views/auth/Login.vue"),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: "/onboarding/workspace",
      name: "workspaceOnboarding",
      component: () => import("../views/auth/WorkspaceOnboarding.vue"),
      meta: { requiresAuth: true, requiresInit: false, requiresTenant: false }
    },
    {
      path: "/join",
      name: "joinOrganization",
      // 重定向到组织列表页，并将 code 参数转换为 invite_code
      redirect: (to) => {
        const code = to.query.code as string
        return {
          path: '/platform/organizations',
          query: code ? { invite_code: code } : {}
        }
      },
      meta: { requiresInit: true, requiresAuth: true }
    },
    {
      path: "/knowledgeBase",
      name: "home",
      component: () => import("../views/knowledge/KnowledgeBase.vue"),
      meta: { requiresInit: true, requiresAuth: true }
    },
    {
      path: "/platform",
      name: "Platform",
      redirect: DEFAULT_EMPLOYEE_WORKSPACE_PATH,
      component: () => import("../views/platform/index.vue"),
      meta: { requiresInit: true, requiresAuth: true },
      children: [
        {
          path: "tenant",
          redirect: "/platform/settings"
        },
        {
          path: "settings",
          name: "settings",
          component: () => import("../views/settings/Settings.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          // Canonical enterprise-administration entry. The real record (rather
          // than a static redirect) makes every navigation re-check the server
          // projection before reusing the existing Settings members UI.
          path: 'enterprise',
          name: 'enterpriseAdministration',
          component: { render: () => null },
          meta: { requiresInit: true, requiresAuth: true },
          beforeEnter: async (to) => {
            try {
              const projection = await getEnterpriseSession()
              if (!projection.surfaces.enterpriseAdministration) {
                return DEFAULT_EMPLOYEE_WORKSPACE_PATH
              }
              return {
                path: '/platform/settings',
                query: { section: to.query.section === 'tenant' ? 'tenant' : 'members' },
              }
            } catch (error) {
              if (error instanceof EnterpriseSessionRequestError && error.status === 401) {
                return loginDestination(router, to.fullPath)
              }
              return DEFAULT_EMPLOYEE_WORKSPACE_PATH
            }
          },
        },
        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "operating-analysis",
          name: "operatingAnalysis",
          component: { render: () => null },
          meta: { requiresInit: true, requiresAuth: true },
          beforeEnter: async () => {
            try {
              const handoffRef = sessionStorage.getItem(OPERATING_ANALYSIS_HANDOFF_REF_KEY)
              let response
              let handoffPrompt = null
              if (handoffRef) {
                sessionStorage.removeItem(OPERATING_ANALYSIS_HANDOFF_REF_KEY)
                response = await consumeOperatingAnalysisHandoff(handoffRef)
                if (response.handoff?.schema !== 'OperatingAnalysisHandoffV1' || !response.handoff.question) {
                  return '/platform/creatChat'
                }
                handoffPrompt = response.handoff
              } else {
                const availability = await getOperatingAnalysisAvailability()
                if (availability.availability.state !== 'enabled' || !availability.availability.canExchange) {
                  return '/platform/creatChat'
                }
                response = await exchangeOperatingAnalysis()
              }
              if (!response.access_token || response.expires_in !== 900) {
                return '/platform/creatChat'
              }
              if (handoffPrompt) {
                sessionStorage.setItem(
                  OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY,
                  JSON.stringify(handoffPrompt),
                )
              }
              localStorage.setItem('retail_ai_app_auth_token', response.access_token)
              document.cookie = `retail_ai_app_auth_token=${response.access_token}; Path=/app; Max-Age=900; SameSite=Lax`
              window.location.assign(handoffPrompt ? '/app/?data_workspace=data' : '/app/')
              return false
            } catch {
              return '/platform/creatChat'
            }
          },
        },
        {
          path: "knowledge-bases/:kbId",
          name: "knowledgeBaseDetail",
          component: () => import("../views/knowledge/KnowledgeBase.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-search",
          // 旧路径保留为重定向，打开全局命令面板（⌘K），带上可选的 q 参数
          redirect: (to) => {
            const q = to.query.q
            return {
              path: '/platform/knowledge-bases',
              query: typeof q === 'string' ? { cmdk: q } : { cmdk: '' },
            }
          },
        },
        {
          path: "agents",
          name: "agentList",
          component: () => import("../views/agent/AgentList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "integrations",
          redirect: (to) => ({
            path: "/platform/settings",
            query: {
              ...to.query,
              section: "integrations",
            },
          }),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "creatChat",
          name: "globalCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-bases/:kbId/creatChat",
          name: "kbCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "chat/:chatid",
          name: "chat",
          component: () => import("../views/chat/index.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "organizations",
          name: "organizationList",
          component: () => import("../views/organization/OrganizationList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        // Compatibility redirects for /platform/system/* URLs. System
        // administration surfaces live as dedicated sections inside the
        // standard Settings modal; keep stable URLs for bookmarks and
        // external links.
        {
          path: "system",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/settings",
          name: "systemSettings",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/admins",
          name: "systemAdmins",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/queues",
          name: "systemQueues",
          redirect: { path: "/platform/settings", query: { section: "runtime-queues" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
      ],
    },
    // Dev-only markdown rendering test page
    ...(import.meta.env.DEV ? [{
      path: '/platform/dev/markdown',
      name: 'markdownTest',
      component: () => import('../views/dev/MarkdownTestPage.vue'),
      meta: { requiresAuth: false, requiresInit: false }
    }] : []),
  ],
});

// 持久化 auto-setup / login 返回的认证信息到 store
function persistLoginResponse(authStore: ReturnType<typeof useAuthStore>, response: any) {
  if (response.user && response.tenant && response.token) {
    authStore.setUser(userInfoFromApi(response.user, response.tenant.id))
    authStore.setToken(response.token)
    if (response.refresh_token) {
      authStore.setRefreshToken(response.refresh_token)
    }
    authStore.setTenant({
      id: String(response.tenant.id) || '',
      name: response.tenant.name || '',
      owner_id: response.user.id || '',
      created_at: response.tenant.created_at || new Date().toISOString(),
      updated_at: response.tenant.updated_at || new Date().toISOString()
    })
  }
}

async function hydrateSessionFromToken(authStore: ReturnType<typeof useAuthStore>) {
  const token = localStorage.getItem('weknora_token')
  if (!token) return false

  if (!authStore.token) {
    authStore.setToken(token)
  }

  const storedRefreshToken = localStorage.getItem('weknora_refresh_token')
  if (storedRefreshToken && !authStore.refreshToken) {
    authStore.setRefreshToken(storedRefreshToken)
  }

  try {
    const response = await getCurrentUser()
    const user = response.data?.user
    if (!response.success || !user) {
      return false
    }

    authStore.setUser(userInfoFromApi(user, response.data?.tenant?.id))

    const tenant = response.data?.tenant
    if (tenant) {
      authStore.setTenant({
        id: String(tenant.id) || '',
        name: tenant.name || '',
        owner_id: tenant.owner_id || user.id || '',
        description: tenant.description,
        status: tenant.status,
        business: tenant.business,
        storage_quota: tenant.storage_quota,
        storage_used: tenant.storage_used,
        created_at: tenant.created_at || new Date().toISOString(),
        updated_at: tenant.updated_at || new Date().toISOString(),
      })
    } else {
      authStore.setTenant(null)
    }

    // Refresh memberships on every page load — same reason as
    // App.vue's syncOIDCUserContext: without this the auth store
    // would only ever see the snapshot from the original /auth/login
    // call, so role changes (and tenant-switch role lookups) would
    // be silently stale until the user logged out and back in.
    const memberships = response.data?.memberships
    if (Array.isArray(memberships)) {
      authStore.setMemberships(memberships)
    }

    const canCreateTenant = response.data?.capabilities?.can_create_tenant
    if (typeof canCreateTenant === 'boolean') {
      authStore.setCanCreateTenant(canCreateTenant)
    }
    authStore.setCanManageAllTenantMembers(
      response.data?.capabilities?.can_manage_all_tenant_members === true,
    )

    return true
  } catch {
    return false
  }
}

let autoSetupAttempted = false
let liteDeepLinkRestoreDone = false

// 路由守卫：检查认证状态和系统初始化状态
router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()

  // OIDC 回跳登录结果依赖 App.vue 在挂载后消费 URL hash。
  // 如果这里先按“未登录”拦截到 /login，会导致回调结果没有机会落盘。
  if (hasPendingOIDCCallback()) {
    next()
    return
  }

  // Lite：硬刷新后若落在默认首页，恢复本次会话中最后访问的 /platform 子路径
  if (!liteDeepLinkRestoreDone) {
    liteDeepLinkRestoreDone = true
    if (isLiteEdition(authStore)) {
      const saved = sessionStorage.getItem(LITE_LAST_PATH_KEY)
      if (saved && isSafeLiteRestoreTarget(saved) && isLiteSpaDefaultEntry(to)) {
        if (saved !== to.fullPath) {
          next(saved)
          return
        }
      }
    }
  }

  // Tenantless onboarding still requires a valid user token even though it
  // deliberately skips the normal tenant/system-initialization gates.
  if (to.path === '/onboarding/workspace') {
    if (!authStore.isLoggedIn) {
      const restored = await hydrateSessionFromToken(authStore)
      if (!restored) {
        next(loginDestination(router, to.fullPath))
        return
      }
    }
    if (authStore.hasValidTenant) {
      next(DEFAULT_EMPLOYEE_WORKSPACE_PATH)
    } else {
      next()
    }
    return
  }

  // 如果访问的是登录页面或初始化页面，直接放行
  if (to.meta.requiresAuth === false || to.meta.requiresInit === false) {
    // 如果已登录用户访问登录页面，重定向到知识库列表页面
    if (to.path === '/login' && authStore.isLoggedIn) {
      const confirmed = await authStore.refreshFromAuthMe()
      if (!confirmed) {
        authStore.logout()
        next()
        return
      }
      const returnTo = safeReturnTo(router, to.query.returnTo)
      next(returnTo || (authStore.hasValidTenant ? DEFAULT_EMPLOYEE_WORKSPACE_PATH : '/onboarding/workspace'))
      return
    }
    next()
    return
  }

  // 检查用户认证状态
  if (to.meta.requiresAuth !== false) {
    if (!authStore.isLoggedIn) {
      const restored = await hydrateSessionFromToken(authStore)
      if (restored) {
        next(
          !authStore.hasValidTenant && to.meta.requiresTenant !== false
            ? '/onboarding/workspace'
            : to.fullPath,
        )
        return
      }

      if (!autoSetupAttempted && shouldTryAutoSetup()) {
        autoSetupAttempted = true
        try {
          const response = await autoSetup()
          if (response.success) {
            persistLoginResponse(authStore, response)
            authStore.setLiteMode(true)
            next(to.fullPath)
            return
          } else {
            markAutoSetupFailed()
          }
        } catch {
          markAutoSetupFailed()
        }
      }
      next(loginDestination(router, to.fullPath))
      return
    }
  }

  if (to.meta.requiresTenant !== false && !authStore.hasValidTenant) {
    next('/onboarding/workspace')
    return
  }

  // This only keeps the employee UI coherent with the product-surface policy.
  // API route guards remain the authority for every read and mutation.
  const minimumRole = employeeWorkspaceMinRole(to)
  if (minimumRole && !authStore.hasRole(minimumRole)) {
    next(DEFAULT_EMPLOYEE_WORKSPACE_PATH)
    return
  }

  // SystemAdmin gate — checked AFTER auth so a non-admin who's logged
  // out gets redirected to /login first (consistent with how the rest
  // of the auth flow works), and only an authenticated non-admin sees
  // the bounce. This is UI-only; the server enforces the real check.
  if (to.meta.requiresSystemAdmin === true) {
    if (!authStore.isSystemAdmin) {
      next(DEFAULT_EMPLOYEE_WORKSPACE_PATH)
      return
    }
  }

  next()
})

router.afterEach((to) => {
  if (!isLiteEdition(useAuthStore())) return
  if (to.path === '/login') return
  if (!to.path.startsWith('/platform')) return
  sessionStorage.setItem(LITE_LAST_PATH_KEY, to.fullPath)
})

export default router
