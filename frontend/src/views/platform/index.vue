<template>
    <div class="main" :class="{ 'mobile-menu-open': mobileMenuOpen }" @keydown.esc="mobileMenuOpen = false" ref="dropzone">
        <header class="mobile-workspace-header">
            <button type="button" @click="router.push('/home')">环枢</button>
            <span aria-hidden="true"></span>
            <button type="button" :aria-expanded="mobileMenuOpen" aria-controls="mobile-workspace-menu" @click="mobileMenuOpen = !mobileMenuOpen">{{ mobileMenuOpen ? t('menu.collapseSidebar') : t('menu.expandSidebar') }}</button>
        </header>
        <button v-if="mobileMenuOpen" class="mobile-menu-scrim" type="button" :aria-label="t('menu.collapseSidebar')" @click="mobileMenuOpen = false" />
        <Menu id="mobile-workspace-menu" :operating-controller="operatingController" @navigate="mobileMenuOpen = false"></Menu>
        <div v-if="isRouterAlive" v-show="!isOperatingRoute" class="platform-route-outlet">
            <RouterView />
        </div>
        <OperatingWorkspace
            v-if="operatingWorkspaceMounted"
            v-show="isOperatingRoute"
            @controller-change="handleOperatingControllerChange"
        />
        <div class="upload-mask" v-show="ismask">
            <UploadMask></UploadMask>
        </div>
        <!-- 全局设置模态框，供所有 platform 子路由使用 -->
        <Settings />
        <!-- 全局命令面板 (⌘K)，随 platform 路由存活 -->
        <GlobalCommandPalette />
        <!-- 全局右上角"待处理邀请"铃铛。固定定位，z-index 低于抽屉，业务页面
             右侧抽屉弹出时会自然覆盖；仅在有待处理邀请时渲染。 -->
        <GlobalInvitationBell />
        <!-- 带遮罩层的新手引导：首次进入自动开启，可从用户菜单顶部昵称旁帮助按钮重新打开 -->
        <NewUserGuide />
    </div>
</template>
<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import { needsKnowledgeBaseConfiguration } from '@/utils/knowledgeBaseInitialization'
import Menu from '@/components/menu.vue'
import { computed, ref, shallowRef, onMounted, onUnmounted, nextTick, provide, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router'
import UploadMask from '@/components/upload-mask.vue'
import Settings from '@/views/settings/Settings.vue'
import GlobalCommandPalette from '@/components/GlobalCommandPalette.vue'
import GlobalInvitationBell from '@/components/GlobalInvitationBell.vue'
import NewUserGuide from '@/components/NewUserGuide.vue'
import { useCommandPaletteStore } from '@/stores/commandPalette'
import { useChatResourcesStore } from '@/stores/chatResources'
import { getKnowledgeBaseById } from '@/api/knowledge-base/index'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { collectDroppedFiles } from './collectDroppedFiles'
import OperatingWorkspace from '@/views/operating/OperatingWorkspace.vue'
import { isOperatingRoutePath } from '@/views/operating/operatingHost'
import type { OperatingController } from '@/views/operating/operatingClient'

const authStore = useAuthStore();
const mobileMenuOpen = ref(false);
const route = useRoute();
const isOperatingRoute = computed(() => isOperatingRoutePath(route.path));
const operatingWorkspaceMounted = ref(isOperatingRoute.value);
const operatingController = shallowRef<OperatingController | null>(null);
const handleOperatingControllerChange = (controller: OperatingController | null) => {
    operatingController.value = controller;
};
watch(isOperatingRoute, (isOperating) => {
    if (isOperating) operatingWorkspaceMounted.value = true;
});
watch(() => route.fullPath, () => { mobileMenuOpen.value = false; });
const router = useRouter();
const commandPaletteStore = useCommandPaletteStore();
let ismask = ref(false)
const { t } = useI18n();

const isRouterAlive = ref(true)
const reloadApp = () => {
    isRouterAlive.value = false
    nextTick(() => {
        isRouterAlive.value = true
    })
}
provide('app:reload', reloadApp)

// 仅在 Wails 桌面端运行时拦截 Cmd/Ctrl+R：
// 桌面端没有浏览器地址栏，整页重载会白屏，所以用前端软刷新替代。
// 浏览器（含 Web 版 / 非 Lite 部署）里不拦截，交给浏览器做真正的整页刷新，
// 否则会出现左侧菜单、全局设置、Pinia store 等不随"刷新"一起重置的问题。
// @ts-ignore
const isWailsDesktop = typeof window !== 'undefined' && !!(window as any).runtime?.EventsOn

const handleGlobalKeyDown = (e: KeyboardEvent) => {
    if (!isWailsDesktop) return
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'r') {
        e.preventDefault()
        reloadApp()
    }
}

// 用于跟踪拖拽进入/离开的计数器，解决子元素触发 dragleave 的问题
let dragCounter = 0;

// 获取当前知识库ID
const getCurrentKbId = (): string | null => {
    return (route.params as any)?.kbId as string || null
}

const CHAT_DROP_ROUTE_NAMES = new Set(['chat', 'globalCreatChat', 'kbCreatChat']);

const isChatDropRoute = () => {
    return CHAT_DROP_ROUTE_NAMES.has(String(route.name || ''));
}

// 检查知识库初始化状态
const checkKnowledgeBaseInitialization = async (): Promise<boolean> => {
    const currentKbId = getCurrentKbId();
    
    if (!currentKbId) {
        MessagePlugin.error(t('knowledgeBase.missingId'));
        return false;
    }
    
    try {
        const kbResponse = await getKnowledgeBaseById(currentKbId);
        const kb = kbResponse.data;
        
        if (needsKnowledgeBaseConfiguration(kb, authStore.isSystemAdmin)) {
            MessagePlugin.warning(t('knowledgeBase.notInitialized'));
            return false;
        }
        return true;
    } catch (error) {
        MessagePlugin.error(t('knowledgeBase.getInfoFailed'));
        return false;
    }
}


// isFileDrag distinguishes an OS file drag (the only thing the global upload
// drop zone cares about) from an in-app element drag such as the wiki
// folder/page drag-and-drop. Element drags carry only "text/*" types, never
// "Files", so we bail out and let the originating component handle the drop.
const isFileDrag = (event: DragEvent): boolean => {
    const types = event.dataTransfer?.types
    if (!types) return false
    return Array.from(types).includes('Files')
}

// 全局拖拽事件处理
const handleGlobalDragEnter = (event: DragEvent) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    dragCounter++;
    if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = 'all';
    }
    ismask.value = true;
}

const handleGlobalDragOver = (event: DragEvent) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    if (event.dataTransfer) {
        event.dataTransfer.dropEffect = 'copy';
    }
}

const handleGlobalDragLeave = (event: DragEvent) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    dragCounter--;
    if (dragCounter === 0) {
        ismask.value = false;
    }
}

const handleGlobalDrop = async (event: DragEvent) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    dragCounter = 0;
    ismask.value = false;

    const droppedFiles = await collectDroppedFiles(event);
    if (droppedFiles.length === 0) {
        MessagePlugin.warning(t('knowledgeBase.dragFileNotText'));
        return;
    }

    if (isChatDropRoute() || isOperatingRoute.value) {
        event.stopPropagation();
        window.dispatchEvent(new CustomEvent('weknora:chat-file-drop', {
            detail: { files: droppedFiles }
        }));
        return;
    }
    
    const isInitialized = await checkKnowledgeBaseInitialization();
    if (!isInitialized) {
        return;
    }

    window.dispatchEvent(new CustomEvent('weknora:knowledge-file-drop', {
        detail: { kbId: getCurrentKbId(), files: droppedFiles }
    }));
}

// 组件挂载时添加全局事件监听器
onMounted(() => {
    document.addEventListener('dragenter', handleGlobalDragEnter, true);
    document.addEventListener('dragover', handleGlobalDragOver, true);
    document.addEventListener('dragleave', handleGlobalDragLeave, true);
    document.addEventListener('drop', handleGlobalDrop, true);
    if (isWailsDesktop) {
        window.addEventListener('keydown', handleGlobalKeyDown);
        // @ts-ignore
        window.runtime.EventsOn('app:reload', () => {
            reloadApp()
        })
    }
    // 支持通过 URL 查询参数打开全局命令面板，例如旧路径
    // /platform/knowledge-search?q=foo 重定向后携带 ?cmdk=foo
    maybeOpenCmdkFromRoute()
    // 后台预取对话输入栏资源，进入 creatChat / chat 时复用缓存
    void useChatResourcesStore().prefetchChatInput()
});

// 监听路由变化，兼容 SPA 内部跳转时的 ?cmdk= 参数
watch(() => route.query.cmdk, () => {
    maybeOpenCmdkFromRoute()
})

function maybeOpenCmdkFromRoute() {
    if (!('cmdk' in route.query)) return
    const q = String(route.query.cmdk ?? '')
    commandPaletteStore.openPalette(q)
    // 清除 query，避免回退/刷新时反复触发
    const newQuery = { ...route.query }
    delete (newQuery as any).cmdk
    router.replace({ path: route.path, query: newQuery, hash: route.hash })
}

// 组件卸载时移除全局事件监听器
onUnmounted(() => {
    document.removeEventListener('dragenter', handleGlobalDragEnter, true);
    document.removeEventListener('dragover', handleGlobalDragOver, true);
    document.removeEventListener('dragleave', handleGlobalDragLeave, true);
    document.removeEventListener('drop', handleGlobalDrop, true);
    if (isWailsDesktop) {
        window.removeEventListener('keydown', handleGlobalKeyDown);
        // @ts-ignore
        if (window.runtime?.EventsOff) {
            // @ts-ignore
            window.runtime.EventsOff('app:reload')
        }
    }
    dragCounter = 0;
});
</script>
<style lang="less">
.main {
    display: flex;
    align-items: stretch;
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    /* 统一整页背景，让左侧菜单与右侧内容区视觉连贯 */
    background: var(--td-bg-color-container);
}

/* 右侧路由区：占满剩余宽度与整列高度，并把 min-height:0 传给子页面以便内部 flex 滚动 */
.platform-route-outlet {
    flex: 1;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.upload-mask {
    background-color: rgba(255, 255, 255, 0.8);
    position: fixed;
    width: 100%;
    height: 100%;
    z-index: 999;
    display: flex;
    justify-content: center;
    align-items: center;
}

img {
    -webkit-user-drag: none;
    -khtml-user-drag: none;
    -moz-user-drag: none;
    -o-user-drag: none;
    user-drag: none;
}
.mobile-workspace-header, .mobile-menu-scrim { display:none; }
@media(max-width:560px) {
    .main { flex-direction:column; }
    .mobile-workspace-header { display:flex; align-items:center; gap:12px; min-height:48px; flex:0 0 48px; padding:0 12px; border-bottom:1px solid var(--td-component-border); background:var(--td-bg-color-sidebar); }
    .mobile-workspace-header > span { flex:1; color:var(--td-text-color-secondary); font-size:12px; }
    .mobile-workspace-header button { border:0; background:transparent; color:var(--td-text-color-primary); font:inherit; min-height:36px; cursor:pointer; }
    .mobile-workspace-header button:focus-visible { outline:2px solid var(--td-brand-color); outline-offset:2px; }
    .main > .aside_box { display:none; }
    .main.mobile-menu-open > .aside_box { display:flex; position:fixed; top:48px; left:0; bottom:0; height:calc(100dvh - 48px); z-index:101; }
    .mobile-menu-scrim { display:block; position:fixed; inset:48px 0 0; z-index:100; border:0; background:rgba(15,30,26,.25); }
    .platform-route-outlet .chat, .platform-route-outlet .chat.is-sidebar-collapsed { min-width:0; max-width:100%; width:100%; }
}
</style>
