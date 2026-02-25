<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import AddSiteForm from './components/AddSiteForm.vue'
import SharePanel from './components/SharePanel.vue'
import SiteSettingsDrawer from './components/SiteSettingsDrawer.vue'
import SiteGrid from './components/SiteGrid.vue'
import { useApi } from './composables/useApi'
import type { CreateSharePayload, Share, Site } from './types'

const api = useApi()

const sites = ref<Site[]>([])
const shares = ref<Share[]>([])
const theme = ref<'light' | 'dark'>('light')
const loading = ref(false)
const savingSite = ref(false)
const creatingShare = ref(false)
const alertMessage = ref('')
const searchQuery = ref('')
const selectedGroup = ref('全部')
const showAddForm = ref(false)
const showShareDrawer = ref(false)
const showSiteSettingsDrawer = ref(false)
const settingsSiteId = ref<number | null>(null)
const alertTimer = ref<number | undefined>(undefined)
const alertKind = ref<'success' | 'error'>('success')

const groupedOptions = computed(() => {
  const groups = new Set<string>()
  for (const site of sites.value) {
    const group = site.groupName.trim()
    if (group) {
      groups.add(group)
    }
  }
  return ['全部', ...Array.from(groups).sort((a, b) => a.localeCompare(b, 'zh-CN'))]
})

const filteredSites = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase()
  return sites.value.filter((site) => {
    const inGroup = selectedGroup.value === '全部' || site.groupName === selectedGroup.value
    if (!inGroup) {
      return false
    }

    if (!keyword) {
      return true
    }

    const haystack = `${site.name} ${site.url} ${site.groupName}`.toLowerCase()
    return haystack.includes(keyword)
  })
})

const shareLookup = computed(() => {
  const lookup: Record<number, { active: boolean; total: number }> = {}
  for (const share of shares.value) {
    if (!lookup[share.siteId]) {
      lookup[share.siteId] = { active: false, total: 0 }
    }
    lookup[share.siteId].total += 1
    if (share.status === 'active') {
      lookup[share.siteId].active = true
    }
  }
  return lookup
})

const settingsSite = computed(() => {
  if (settingsSiteId.value === null) {
    return null
  }
  return sites.value.find((item) => item.id === settingsSiteId.value) ?? null
})

const settingsSiteShares = computed(() => {
  if (settingsSiteId.value === null) {
    return []
  }
  return shares.value.filter((item) => item.siteId === settingsSiteId.value)
})

const settingsActiveShare = computed(() => {
  return settingsSiteShares.value.find((item) => item.status === 'active') ?? null
})

const loadData = async () => {
  loading.value = true
  try {
    const [siteList, shareList] = await Promise.all([api.listSites(), api.listShares()])
    sites.value = siteList
    shares.value = shareList
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '加载数据失败', 'error')
  } finally {
    loading.value = false
  }
}

const showAlert = (message: string, kind: 'success' | 'error' = 'success') => {
  alertKind.value = kind
  alertMessage.value = message
}

const applyTheme = (value: 'light' | 'dark') => {
  theme.value = value
  document.documentElement.setAttribute('data-theme', value)
  window.localStorage.setItem('nonav-theme', value)
}

const toggleTheme = async (event: MouseEvent) => {
  const nextTheme = theme.value === 'light' ? 'dark' : 'light'
  const doc = document as Document & {
    startViewTransition?: (callback: () => void) => {
      ready: Promise<void>
    }
  }

  if (!doc.startViewTransition) {
    applyTheme(nextTheme)
    return
  }

  const originX = event.clientX
  const originY = event.clientY
  const endRadius = Math.hypot(
    Math.max(originX, window.innerWidth - originX),
    Math.max(originY, window.innerHeight - originY),
  )

  const transition = doc.startViewTransition(() => {
    applyTheme(nextTheme)
  })

  await transition.ready
  document.documentElement.animate(
    {
      clipPath: [
        `circle(0px at ${originX}px ${originY}px)`,
        `circle(${endRadius}px at ${originX}px ${originY}px)`,
      ],
    },
    {
      duration: 780,
      easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
      pseudoElement: '::view-transition-new(root)',
    } as KeyframeAnimationOptions,
  )
}

const addSite = async (payload: { name: string; url: string; groupName: string; icon: string }) => {
  try {
    const created = await api.createSite(payload)
    sites.value = [created, ...sites.value]
    showAlert('网站已添加')
    showAddForm.value = false
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '添加失败', 'error')
  }
}

const openSite = async (site: Site) => {
  window.open(site.url, '_blank', 'noopener,noreferrer')
  try {
    await api.incrementSiteClick(site.id)
    const match = sites.value.find((item) => item.id === site.id)
    if (match) {
      match.clickCount += 1
    }
  } catch {
    // Ignore click stat errors to avoid blocking navigation.
  }
}

const openSiteSettings = (site: Site) => {
  settingsSiteId.value = site.id
  showSiteSettingsDrawer.value = true
}

const closeSiteSettings = () => {
  showSiteSettingsDrawer.value = false
}

const saveSiteSettings = async (payload: { id: number; name: string; url: string; groupName: string; icon: string }) => {
  savingSite.value = true
  try {
    const updated = await api.updateSite(payload)
    sites.value = sites.value.map((item) => (item.id === updated.id ? updated : item))
    showAlert('网站配置已更新')
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '保存配置失败', 'error')
  } finally {
    savingSite.value = false
  }
}

const deleteSiteFromSettings = async (site: Site) => {
  try {
    await api.deleteSite(site.id)
    sites.value = sites.value.filter((item) => item.id !== site.id)
    shares.value = shares.value.filter((item) => item.siteId !== site.id)
    showAlert(`已删除 ${site.name}`)
    closeSiteSettings()
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '删除失败', 'error')
  }
}

const startShareForSite = async (payload: CreateSharePayload) => {
  creatingShare.value = true
  try {
    const created = await api.createShare(payload)
    await loadShares()
    showAlert(
      created.plainPassword
      ? `分享已创建，密码：${created.plainPassword}`
      : '分享已创建（未设置密码）',
    )
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '创建分享失败', 'error')
  } finally {
    creatingShare.value = false
  }
}

const stopShare = async (share: Share) => {
  try {
    await api.stopShare(share.id)
    await loadShares()
    showAlert('分享已停止')
  } catch (error) {
    showAlert(error instanceof Error ? error.message : '停止失败', 'error')
  }
}

const copyShareLink = async (share: Share) => {
  try {
    await navigator.clipboard.writeText(share.shareUrl)
    showAlert('链接已复制')
  } catch {
    showAlert('复制失败，请手动复制', 'error')
  }
}

const loadShares = async () => {
  try {
    shares.value = await api.listShares()
  } catch {
    // Ignore refresh errors for drawer actions.
  }
}

const clearAlert = () => {
  if (!alertMessage.value) {
    return
  }

  window.clearTimeout(alertTimer.value)
  alertTimer.value = window.setTimeout(() => {
    alertMessage.value = ''
  }, 3200)
}

onMounted(() => {
  const cachedTheme = window.localStorage.getItem('nonav-theme')
  applyTheme(cachedTheme === 'dark' ? 'dark' : 'light')

  void loadData()
})

watch(alertMessage, () => {
  clearAlert()
})

onUnmounted(() => {
  window.clearTimeout(alertTimer.value)
})
</script>

<template>
  <main class="page">
    <header class="topbar">
      <div class="brand">
        <h1>内网导航</h1>
      </div>
      <div class="toolbar">
        <input v-model="searchQuery" class="search-input" type="search" placeholder="搜索站点名称、URL、分组" />
        <button class="button-subtle" type="button" @click="showAddForm = true">添加网站</button>
        <button class="button-subtle" type="button" @click="showShareDrawer = true">分享列表</button>
        <button
          class="button-subtle button-theme"
          type="button"
          :aria-label="theme === 'light' ? '切换夜间模式' : '切换日间模式'"
          @click="toggleTheme($event)"
        >
          <svg class="theme-icon" viewBox="0 0 24 24" aria-hidden="true">
            <g class="theme-icon-sun" :class="{ 'theme-icon-hidden': theme !== 'light' }">
              <circle cx="12" cy="12" r="4.2" fill="currentColor" />
              <g stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
                <line x1="12" y1="2.4" x2="12" y2="5" />
                <line x1="12" y1="19" x2="12" y2="21.6" />
                <line x1="2.4" y1="12" x2="5" y2="12" />
                <line x1="19" y1="12" x2="21.6" y2="12" />
                <line x1="5.2" y1="5.2" x2="7.1" y2="7.1" />
                <line x1="16.9" y1="16.9" x2="18.8" y2="18.8" />
                <line x1="16.9" y1="7.1" x2="18.8" y2="5.2" />
                <line x1="5.2" y1="18.8" x2="7.1" y2="16.9" />
              </g>
            </g>
            <g class="theme-icon-moon" :class="{ 'theme-icon-hidden': theme !== 'dark' }">
              <path
                d="M15.7 3.6a7.9 7.9 0 1 0 4.7 13.8A8.8 8.8 0 0 1 15.7 3.6Z"
                fill="currentColor"
              />
            </g>
          </svg>
        </button>
      </div>
    </header>

    <Transition name="modal">
      <aside v-if="showAddForm" class="modal-mask" @mousedown.self="showAddForm = false">
        <section class="modal-panel" @click.stop>
          <header class="modal-head">
            <h2>添加网站</h2>
            <button class="button-subtle" type="button" @click="showAddForm = false">关闭</button>
          </header>
          <AddSiteForm @submit="addSite" />
        </section>
      </aside>
    </Transition>

    <Transition name="toast">
      <aside v-if="alertMessage" class="toast" :class="`toast-${alertKind}`" role="status" aria-live="polite">
        {{ alertMessage }}
      </aside>
    </Transition>

    <section class="group-tabs">
      <button
        v-for="group in groupedOptions"
        :key="group"
        type="button"
        :class="['group-tab', { 'group-tab-active': selectedGroup === group }]"
        @click="selectedGroup = group"
      >
        {{ group }}
      </button>
    </section>

    <section class="grid-area">
      <p v-if="loading" class="empty-state">正在加载站点...</p>
      <p v-else-if="filteredSites.length === 0" class="empty-state">没有匹配站点，换个关键词试试。</p>
      <SiteGrid
        v-else
        :sites="filteredSites"
        :share-lookup="shareLookup"
        @open="openSite"
        @open-settings="openSiteSettings"
      />
    </section>

    <Transition name="drawer">
      <aside v-if="showShareDrawer" class="drawer-mask" @mousedown.self="showShareDrawer = false">
        <section class="drawer-panel" @click.stop>
          <header class="drawer-head">
            <h2>分享列表</h2>
            <button class="button-subtle" type="button" @click="showShareDrawer = false">关闭</button>
          </header>
          <SharePanel :shares="shares" @copy="copyShareLink" @stop="stopShare" />
        </section>
      </aside>
    </Transition>

    <SiteSettingsDrawer
      :visible="showSiteSettingsDrawer"
      :site="settingsSite"
      :active-share="settingsActiveShare"
      :saving="savingSite"
      :sharing="creatingShare"
      @close="closeSiteSettings"
      @save-site="saveSiteSettings"
      @start-share="startShareForSite"
      @stop-share="stopShare"
      @copy-share="copyShareLink"
      @delete-site="deleteSiteFromSettings"
    />
  </main>
</template>

<style scoped>
.page {
  width: min(1320px, 100% - 40px);
  margin: 20px auto 48px;
  display: grid;
  gap: 18px;
}

.topbar {
  display: flex;
  justify-content: space-between;
  align-items: end;
  gap: 14px;
}

.brand {
  display: flex;
  align-items: center;
}

h1 {
  margin: 0;
  font-size: clamp(30px, 5vw, 44px);
  letter-spacing: -0.02em;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.search-input {
  width: min(360px, 90vw);
  border: 1px solid var(--line-soft);
  background: var(--surface-main);
  color: var(--text-main);
  border-radius: 12px;
  padding: 10px 12px;
  font: inherit;
}

.button-subtle {
  border: 1px solid var(--line-soft);
  border-radius: 12px;
  padding: 10px 12px;
  background: var(--surface-main);
  color: var(--text-main);
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}

.button-theme {
  width: 44px;
  display: grid;
  place-items: center;
  padding: 10px 0;
  color: var(--text-main);
  line-height: 1;
  overflow: hidden;
}

.theme-icon {
  width: 20px;
  height: 20px;
}

.theme-icon-sun,
.theme-icon-moon {
  transform-origin: 50% 50%;
  transition: transform 0.38s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.3s ease;
}

.theme-icon-sun {
  transform: rotate(0deg) scale(1);
}

.theme-icon-moon {
  transform: rotate(-16deg) scale(0.78);
}

.theme-icon-hidden {
  opacity: 0;
  transform: rotate(90deg) scale(0.4);
}

.button-theme:hover .theme-icon-sun:not(.theme-icon-hidden),
.button-theme:hover .theme-icon-moon:not(.theme-icon-hidden) {
  transform: rotate(14deg) scale(1.08);
}

.add-form-wrap {
  border: 1px solid var(--line-soft);
  border-radius: 16px;
  background: var(--surface-main);
  padding: 14px;
}

.modal-mask {
  position: fixed;
  inset: 0;
  z-index: 55;
  background: rgba(4, 10, 20, 0.34);
  display: grid;
  place-items: center;
  padding: 14px;
}

.modal-panel {
  width: min(620px, 100%);
  border-radius: 14px;
  border: 1px solid var(--line-soft);
  background: var(--surface-solid);
  box-shadow: var(--shadow-main);
  padding: 14px;
  display: grid;
  gap: 12px;
}

.modal-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.modal-head h2 {
  margin: 0;
  font-size: 18px;
}

.modal-enter-active,
.modal-leave-active {
  transition: background-color 0.22s ease;
}

.modal-enter-active .modal-panel,
.modal-leave-active .modal-panel {
  transition: transform 0.25s ease, opacity 0.2s ease;
}

.modal-enter-from {
  background: rgba(4, 10, 20, 0);
}

.modal-enter-from .modal-panel,
.modal-leave-to .modal-panel {
  transform: translateY(10px) scale(0.98);
  opacity: 0;
}

.toast {
  position: fixed;
  top: 20px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 60;
  min-width: 260px;
  max-width: min(88vw, 620px);
  border-radius: 12px;
  padding: 11px 14px;
  box-shadow: var(--shadow-main);
  border: 1px solid var(--line-soft);
  text-align: center;
  font-size: 14px;
}

.toast-success {
  background: var(--surface-solid);
  color: var(--text-main);
}

.toast-error {
  background: #7d3131;
  color: #fff;
  border-color: rgba(255, 255, 255, 0.24);
}

.toast-enter-active,
.toast-leave-active {
  transition: transform 0.24s ease, opacity 0.2s ease;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translate(-50%, -10px);
}

.group-tabs {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: max-content;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 4px;
}

.group-tab {
  border: 1px solid var(--line-soft);
  border-radius: 999px;
  background: var(--surface-main);
  color: var(--text-secondary);
  padding: 8px 14px;
  cursor: pointer;
  font: inherit;
}

.group-tab-active {
  color: var(--text-main);
  border-color: var(--line-strong);
  background: var(--surface-tint);
}

.grid-area {
  min-height: 300px;
}

.empty-state {
  margin: 0;
  border: 1px dashed var(--line-soft);
  border-radius: 16px;
  background: var(--surface-main);
  padding: 36px 18px;
  text-align: center;
  color: var(--text-secondary);
}

.drawer-mask {
  position: fixed;
  inset: 0;
  z-index: 25;
  background: rgba(4, 10, 20, 0.32);
  display: flex;
  justify-content: flex-end;
}

.drawer-panel {
  width: min(480px, 100%);
  height: 100%;
  background: var(--surface-solid);
  border-left: 1px solid var(--line-soft);
  padding: 16px;
  overflow-y: auto;
}

.drawer-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.drawer-head h2 {
  margin: 0;
  font-size: 20px;
}

.drawer-enter-active,
.drawer-leave-active {
  transition: background-color 0.26s ease;
}

.drawer-enter-active .drawer-panel,
.drawer-leave-active .drawer-panel {
  transition: transform 0.32s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.28s ease;
}

.drawer-enter-from {
  background: rgba(4, 10, 20, 0);
}

.drawer-enter-from .drawer-panel,
.drawer-leave-to .drawer-panel {
  transform: translateX(24px);
  opacity: 0;
}

:global(::view-transition-old(root)),
:global(::view-transition-new(root)) {
  animation: none;
}

:global(::view-transition-new(root)) {
  z-index: 999;
}

@media (max-width: 1024px) {
  .topbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .toolbar {
    width: 100%;
    justify-content: flex-start;
  }

  .search-input {
    flex: 1;
    min-width: 210px;
  }
}

@media (max-width: 640px) {
  .page {
    width: min(100% - 20px, 1320px);
    margin-top: 14px;
    gap: 14px;
  }

  h1 {
    font-size: 30px;
  }

  .toolbar {
    display: grid;
    grid-template-columns: 1fr 1fr;
    width: 100%;
  }

  .search-input {
    grid-column: 1 / -1;
    width: 100%;
  }

  .button-subtle {
    width: 100%;
  }
}
</style>
