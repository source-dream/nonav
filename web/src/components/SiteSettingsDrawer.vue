<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { Share, Site } from '../types'

const props = defineProps<{
  visible: boolean
  site: Site | null
  activeShare?: Share | null
  saving?: boolean
  sharing?: boolean
}>()

const emit = defineEmits<{
  close: []
  saveSite: [payload: { id: number; name: string; url: string; groupName: string; checkEnabled: boolean }]
  startShare: [payload: { siteId: number; expiresInHours?: number; password?: string; shareMode?: 'path_ctx' | 'subdomain'; subdomainSlug?: string }]
  stopShare: [share: Share]
  copyShare: [share: Share]
  deleteSite: [site: Site]
}>()

const siteForm = reactive({
  name: '',
  url: '',
  groupName: '',
  checkEnabled: true,
})

const shareForm = reactive({
  expiresInHours: '',
  password: '',
  shareMode: 'path_ctx' as 'path_ctx' | 'subdomain',
  subdomainSlug: '',
})

const deleteConfirmText = reactive({ value: '' })

watch(
  () => props.site,
  (site) => {
    siteForm.name = site?.name ?? ''
    siteForm.url = site?.url ?? ''
    siteForm.groupName = site?.groupName ?? ''
    siteForm.checkEnabled = site?.checkEnabled ?? true
    shareForm.expiresInHours = ''
    shareForm.password = ''
    shareForm.shareMode = 'path_ctx'
    shareForm.subdomainSlug = ''
    deleteConfirmText.value = ''
  },
  { immediate: true },
)

const canDelete = computed(() => {
  if (!props.site) {
    return false
  }
  return deleteConfirmText.value.trim() === props.site.name
})

const submitSite = () => {
  if (!props.site) {
    return
  }

  emit('saveSite', {
    id: props.site.id,
    name: siteForm.name.trim(),
    url: siteForm.url.trim(),
    groupName: siteForm.groupName.trim(),
    checkEnabled: siteForm.checkEnabled,
  })
}

const submitStartShare = () => {
  if (!props.site) {
    return
  }

  const expiresRaw = shareForm.expiresInHours.trim()
  const expiresNumber = Number.parseInt(expiresRaw, 10)
  const expiresInHours = Number.isFinite(expiresNumber) && expiresNumber > 0 ? expiresNumber : undefined
  const password = shareForm.password.trim() || undefined

  emit('startShare', {
    siteId: props.site.id,
    expiresInHours,
    password,
    shareMode: shareForm.shareMode,
    subdomainSlug: shareForm.shareMode === 'subdomain' ? shareForm.subdomainSlug.trim() || undefined : undefined,
  })
}

const formatDate = (iso: string) => new Date(iso).toLocaleString()
</script>

<template>
  <Transition name="drawer">
    <aside v-if="visible && site" class="drawer-mask" @mousedown.self="emit('close')">
      <section class="drawer-panel" @click.stop>
        <header class="drawer-header">
          <div>
            <h2>{{ site.name }}</h2>
            <p>卡片设置</p>
          </div>
          <button class="button-subtle" type="button" @click="emit('close')">关闭</button>
        </header>

        <section class="block">
          <h3>网站配置</h3>
          <div class="field-grid">
            <label class="field">
              <span>名称</span>
              <input v-model="siteForm.name" type="text" placeholder="站点名称" />
            </label>
            <label class="field">
              <span>分组</span>
              <input v-model="siteForm.groupName" type="text" placeholder="例如：研发" />
            </label>
            <label class="field field-full">
              <span>URL</span>
              <input v-model="siteForm.url" type="url" placeholder="https://intranet.example.local" />
            </label>
            <label class="field field-full switch-field">
              <span>检测状态</span>
              <span class="switch-row">
                <input v-model="siteForm.checkEnabled" class="switch-input" type="checkbox" />
                <span>{{ siteForm.checkEnabled ? '开启' : '关闭' }}</span>
              </span>
            </label>
          </div>
          <div class="actions-row">
            <button class="button-primary" type="button" :disabled="saving" @click="submitSite">保存网站配置</button>
          </div>
        </section>

        <section class="block">
          <h3>分享配置</h3>
          <div class="field-grid">
            <label class="field">
              <span>有效期（小时）</span>
              <input v-model="shareForm.expiresInHours" type="text" inputmode="numeric" placeholder="默认24" />
            </label>
            <label class="field">
              <span>分享密码</span>
              <input v-model="shareForm.password" type="text" placeholder="留空则不设置密码" />
            </label>
            <div class="field field-full">
              <span>分享模式</span>
              <div class="mode-segment" role="radiogroup" aria-label="分享模式">
                <button
                  class="mode-chip"
                  :class="{ 'mode-chip-active': shareForm.shareMode === 'path_ctx' }"
                  type="button"
                  role="radio"
                  :aria-checked="shareForm.shareMode === 'path_ctx'"
                  @click="shareForm.shareMode = 'path_ctx'"
                >
                  <strong>路径上下文</strong>
                  <small>/x/&lt;ctx-id&gt;/...</small>
                </button>
                <button
                  class="mode-chip"
                  :class="{ 'mode-chip-active': shareForm.shareMode === 'subdomain' }"
                  type="button"
                  role="radio"
                  :aria-checked="shareForm.shareMode === 'subdomain'"
                  @click="shareForm.shareMode = 'subdomain'"
                >
                  <strong>泛子域名</strong>
                  <small>&lt;slug&gt;.域名</small>
                </button>
              </div>
            </div>
            <label v-if="shareForm.shareMode === 'subdomain'" class="field field-full">
              <span>子域前缀</span>
              <input v-model="shareForm.subdomainSlug" type="text" placeholder="留空自动生成10位随机串" />
            </label>
          </div>
          <p class="hint">不填写时默认有效期 24 小时，且不设置访问密码。子域前缀留空会自动生成10位随机串。</p>

          <div class="actions-row">
            <button
              class="button-primary"
              type="button"
              :disabled="sharing"
              @click="submitStartShare"
            >
              {{ activeShare ? '重新开始分享' : '开始分享' }}
            </button>
            <button
              class="button-danger-soft"
              type="button"
              :disabled="!activeShare"
              @click="activeShare && emit('stopShare', activeShare)"
            >
              结束分享
            </button>
          </div>

          <div v-if="activeShare" class="active-share-box">
            <div class="share-head">
              <span class="status status-active">分享中</span>
              <span>到期 {{ formatDate(activeShare.expiresAt) }}</span>
            </div>
            <p class="share-url">{{ activeShare.shareUrl }}</p>
            <div class="actions-row actions-inline">
              <button class="button-subtle" type="button" @click="emit('copyShare', activeShare)">复制链接</button>
            </div>
          </div>
          <p v-else class="hint">当前未开启分享。</p>
        </section>

        <section class="block block-danger">
          <h3>危险操作</h3>
          <p>输入站点名 <strong>{{ site.name }}</strong> 后可删除该卡片。</p>
          <label class="field field-full">
            <span>删除确认</span>
            <input v-model="deleteConfirmText.value" type="text" placeholder="输入站点名确认删除" />
          </label>
          <div class="actions-row">
            <button class="button-danger" type="button" :disabled="!canDelete" @click="emit('deleteSite', site)">
              删除网站
            </button>
          </div>
        </section>
      </section>
    </aside>
  </Transition>
</template>

<style scoped>
.drawer-mask { position: fixed; inset: 0; z-index: 40; background: rgba(4, 10, 20, 0.34); display: flex; justify-content: flex-end; }
.drawer-panel { width: min(560px, 100%); height: 100%; background: var(--surface-solid); border-left: 1px solid var(--line-soft); padding: 16px; overflow-y: auto; display: grid; align-content: start; gap: 12px; }
.drawer-header { display: flex; align-items: center; justify-content: space-between; }
.drawer-header h2 { margin: 0; font-size: 22px; }
.drawer-header p { margin: 4px 0 0; color: var(--text-secondary); font-size: 13px; }
.block { border: 1px solid var(--line-soft); border-radius: 14px; padding: 12px; background: var(--surface-main); display: grid; gap: 10px; }
.block h3 { margin: 0; font-size: 15px; }
.field-grid { display: grid; gap: 10px; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.field { display: grid; gap: 6px; }
.field span { color: var(--text-secondary); font-size: 12px; }
.field input:not(.switch-input) { border: 1px solid var(--line-soft); border-radius: 10px; background: var(--surface-solid); color: var(--text-main); font: inherit; padding: 9px 10px; }
.field-full { grid-column: 1 / -1; }
.switch-field { align-content: start; }
.switch-row { display: inline-flex; align-items: center; gap: 10px; color: var(--text-main); font-size: 13px; }
.switch-input { width: 16px; height: 16px; accent-color: var(--accent-main); }
.mode-segment { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.mode-chip {
  border: 1px solid var(--line-soft);
  border-radius: 12px;
  background: var(--surface-solid);
  color: var(--text-main);
  cursor: pointer;
  text-align: left;
  padding: 10px 12px;
  display: grid;
  gap: 4px;
  transition: border-color 0.2s ease, background-color 0.2s ease, transform 0.2s ease;
}
.mode-chip:hover { border-color: var(--accent-main); transform: translateY(-1px); }
.mode-chip strong { font-size: 13px; line-height: 1.2; }
.mode-chip small { color: var(--text-tertiary); font-size: 11px; }
.mode-chip-active {
  border-color: var(--accent-main);
  background: color-mix(in srgb, var(--accent-main) 14%, var(--surface-solid));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent-main) 55%, transparent);
}
.actions-row { display: flex; gap: 8px; }
.actions-inline { justify-content: flex-end; }
.button-primary, .button-subtle, .button-danger, .button-danger-soft { border: 1px solid var(--line-soft); border-radius: 10px; background: var(--surface-solid); color: var(--text-main); font: inherit; padding: 8px 12px; cursor: pointer; }
.button-primary { background: var(--accent-main); border-color: var(--accent-main); color: #fff; }
.button-danger-soft { color: #9b3d3d; }
.button-danger { background: #8c3434; border-color: #8c3434; color: #fff; }
button:disabled { opacity: 0.52; cursor: not-allowed; }
.active-share-box { border: 1px solid var(--line-soft); border-radius: 10px; background: var(--surface-solid); padding: 10px; display: grid; gap: 8px; }
.share-head { display: flex; justify-content: space-between; color: var(--text-tertiary); font-size: 12px; }
.status { border-radius: 999px; padding: 3px 8px; font-size: 11px; background: var(--surface-tint); }
.status-active { color: var(--accent-main); background: var(--surface-accent); }
.share-url { margin: 0; color: var(--text-secondary); font-size: 12px; overflow: hidden; text-overflow: ellipsis; }
.hint { margin: 0; color: var(--text-tertiary); font-size: 12px; }
.block-danger { border-color: rgba(139, 52, 52, 0.34); }
.block-danger p { margin: 0; color: var(--text-secondary); font-size: 13px; }
.drawer-enter-active, .drawer-leave-active { transition: background-color 0.24s ease; }
.drawer-enter-active .drawer-panel, .drawer-leave-active .drawer-panel { transition: transform 0.3s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.26s ease; }
.drawer-enter-from { background: rgba(4, 10, 20, 0); }
.drawer-enter-from .drawer-panel, .drawer-leave-to .drawer-panel { transform: translateX(22px); opacity: 0; }
@media (max-width: 640px) { .drawer-panel { width: 100%; padding: 14px; } .field-grid { grid-template-columns: 1fr; } }
</style>
