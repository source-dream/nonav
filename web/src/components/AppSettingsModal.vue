<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'

type SettingFile = 'internal' | 'gateway'
type SettingControl = 'text' | 'url' | 'number' | 'switch' | 'select' | 'path' | 'password'
type SettingValue = string | number | boolean

interface SettingOption {
  label: string
  value: string
}

interface SettingField {
  id: string
  envKey: string
  label: string
  description: string
  control: SettingControl
  value: SettingValue
  placeholder?: string
  hint?: string
  options?: SettingOption[]
}

interface SettingGroup {
  id: string
  label: string
  fields: SettingField[]
}

interface SettingPage {
  id: SettingFile
  label: string
  filePath: string
  groups: SettingGroup[]
}

defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const STORAGE_KEY = 'nonav-settings-draft:v2'

const makeField = (
  file: SettingFile,
  envKey: string,
  label: string,
  description: string,
  control: SettingControl,
  value: SettingValue,
  extra: Omit<Partial<SettingField>, 'id' | 'envKey' | 'label' | 'description' | 'control' | 'value'> = {},
): SettingField => ({
  id: `${file}:${envKey}`,
  envKey,
  label,
  description,
  control,
  value,
  ...extra,
})

const buildPages = (): SettingPage[] => [
  {
    id: 'internal',
    label: '导航设置',
    filePath: 'server/internal.env',
    groups: [
      {
        id: 'internal-basic',
        label: '基础',
        fields: [
          makeField('internal', 'NONAV_API_LISTEN_ADDR', 'API 监听地址', 'nonav API 进程监听的本地地址。', 'text', ':8081'),
          makeField('internal', 'NONAV_DB_PATH', '数据库路径', 'SQLite 数据库存储位置。', 'path', './data/nonav.db'),
          makeField('internal', 'NONAV_CORS_ORIGIN', '允许跨域来源', '允许控制台前端访问 API 的来源地址。', 'url', 'http://127.0.0.1:5173'),
          makeField('internal', 'NONAV_PUBLIC_BASE_URL', '公共访问地址', '系统对外展示的基础访问地址。', 'url', 'http://localhost:8080'),
        ],
      },
      {
        id: 'internal-share',
        label: '分享',
        fields: [
          makeField('internal', 'NONAV_SHARE_SUBDOMAIN_ENABLED', '启用泛子域分享', '开启后可通过子域名方式访问分享。', 'switch', true),
          makeField('internal', 'NONAV_SHARE_SUBDOMAIN_BASE', '分享根域名', '泛子域分享所依赖的根域名。', 'text', 'localhost'),
        ],
      },
      {
        id: 'internal-frp',
        label: 'FRP',
        fields: [
          makeField('internal', 'NONAV_FORCE_FRP', '强制走 FRP', '启用后分享流量仅通过 FRP 上游转发。', 'switch', true),
          makeField('internal', 'NONAV_FRP_UPSTREAM_URL', 'FRP 上游地址', '动态分配端口后拼接的 FRP 上游目标。', 'url', 'http://127.0.0.1:13000'),
          makeField('internal', 'NONAV_FRP_PORT_MIN', '端口池最小值', '分配分享代理端口的起始值。', 'number', 13000),
          makeField('internal', 'NONAV_FRP_PORT_MAX', '端口池最大值', '分配分享代理端口的结束值。', 'number', 13100),
          makeField('internal', 'NONAV_FRP_CLIENT_BIN', 'frpc 可执行文件', '由 API 拉起的 frpc 客户端程序路径。', 'path', '../frp/frpc'),
          makeField('internal', 'NONAV_FRP_SERVER_ADDR', 'frps 服务地址', 'frpc 回连使用的公网地址或域名。', 'text', '127.0.0.1'),
          makeField('internal', 'NONAV_FRP_SERVER_PORT', 'frps 服务端口', 'frps 对外暴露的控制连接端口。', 'number', 7000),
          makeField('internal', 'NONAV_FRP_AUTH_TOKEN', 'FRP 认证 Token', 'API 与 gateway 之间保持一致的 FRP 鉴权密钥。', 'password', 'change-me'),
          makeField('internal', 'NONAV_FRP_RECOVER_ON_START', '启动时恢复历史代理', 'nonav 启动后自动尝试恢复历史分享代理。', 'switch', true),
          makeField('internal', 'NONAV_FRP_EXPOSE_API', '暴露 API 到 FRP', '允许通过 FRP 侧远程端口回源到 nonav API。', 'switch', true),
          makeField('internal', 'NONAV_FRP_API_REMOTE_PORT', 'API 远程端口', 'FRP 暴露 nonav API 时使用的固定远程端口。', 'number', 18081),
        ],
      },
      {
        id: 'internal-log',
        label: '日志',
        fields: [
          makeField('internal', 'NONAV_LOG_LEVEL', '日志级别', '整体日志输出级别。', 'select', 'info', {
            options: [
              { label: 'info', value: 'info' },
              { label: 'warn', value: 'warn' },
              { label: 'error', value: 'error' },
              { label: 'debug', value: 'debug' },
            ],
          }),
          makeField('internal', 'NONAV_LOG_ROUTE_TRACE', '打印路由追踪', '在日志中输出更详细的路由追踪信息。', 'switch', true),
        ],
      },
    ],
  },
  {
    id: 'gateway',
    label: '网关设置',
    filePath: 'server/gateway.env',
    groups: [
      {
        id: 'gateway-basic',
        label: '基础',
        fields: [
          makeField('gateway', 'NONAV_GATEWAY_LISTEN_ADDR', 'Gateway 监听地址', '公网入口服务监听的地址与端口。', 'text', ':8080'),
          makeField('gateway', 'NONAV_API_BASE_URL', 'API 回源地址', 'Gateway 访问 nonav API 的内部地址。', 'url', 'http://127.0.0.1:18081'),
          makeField('gateway', 'NONAV_PUBLIC_BASE_URL', '公共访问地址', '系统对外展示的基础访问地址。', 'url', 'http://lvh.me:8080'),
        ],
      },
      {
        id: 'gateway-share',
        label: '分享',
        fields: [
          makeField('gateway', 'NONAV_SHARE_SUBDOMAIN_ENABLED', '启用泛子域分享', '开启后可通过子域名方式访问分享。', 'switch', true),
          makeField('gateway', 'NONAV_SHARE_SUBDOMAIN_BASE', '分享根域名', '泛子域分享所依赖的根域名。', 'text', 'lvh.me'),
        ],
      },
      {
        id: 'gateway-frp',
        label: 'FRP',
        fields: [
          makeField('gateway', 'NONAV_FORCE_FRP', '强制走 FRP', '启用后分享流量仅通过 FRP 上游转发。', 'switch', true),
          makeField('gateway', 'NONAV_FRP_UPSTREAM_URL', 'FRP 上游地址', '动态分配端口后拼接的 FRP 上游目标。', 'url', 'http://127.0.0.1:13000'),
          makeField('gateway', 'NONAV_FRP_PORT_MIN', '端口池最小值', '分配分享代理端口的起始值。', 'number', 13000),
          makeField('gateway', 'NONAV_FRP_PORT_MAX', '端口池最大值', '分配分享代理端口的结束值。', 'number', 13100),
          makeField('gateway', 'NONAV_EMBED_FRPS', '内嵌启动 frps', '开启后由 gateway 进程直接启动 frps。', 'switch', true),
          makeField('gateway', 'NONAV_FRP_SERVER_BIN', 'frps 可执行文件', 'gateway 内嵌启动 frps 时使用的可执行文件路径。', 'path', 'frps'),
          makeField('gateway', 'NONAV_FRP_SERVER_BIND_ADDR', 'frps 绑定地址', 'frps 对外监听的绑定地址。', 'text', '0.0.0.0'),
          makeField('gateway', 'NONAV_FRP_SERVER_ADDR', 'frps 服务地址', '网关侧使用的 frps 服务地址或域名。', 'text', '127.0.0.1'),
          makeField('gateway', 'NONAV_FRP_SERVER_PORT', 'frps 服务端口', 'frps 对外暴露的控制连接端口。', 'number', 7000),
          makeField('gateway', 'NONAV_FRP_AUTH_TOKEN', 'FRP 认证 Token', '网关侧使用的 FRP 鉴权密钥。', 'password', 'change-me'),
        ],
      },
      {
        id: 'gateway-log',
        label: '日志',
        fields: [
          makeField('gateway', 'NONAV_LOG_LEVEL', '日志级别', '整体日志输出级别。', 'select', 'info', {
            options: [
              { label: 'info', value: 'info' },
              { label: 'warn', value: 'warn' },
              { label: 'error', value: 'error' },
              { label: 'debug', value: 'debug' },
            ],
          }),
          makeField('gateway', 'NONAV_LOG_ROUTE_TRACE', '打印路由追踪', '在日志中输出更详细的路由追踪信息。', 'switch', true),
        ],
      },
    ],
  },
]

const pages = reactive(buildPages())
const activePageId = ref<SettingFile>('internal')
const openSelectId = ref<string | null>(null)
const hydrated = ref(false)

const allFields = computed(() => {
  return pages.flatMap((page) => page.groups.flatMap((group) => group.fields))
})

const activePage = computed(() => {
  return pages.find((page) => page.id === activePageId.value) ?? pages[0]
})

const createDraftMap = () => {
  const next: Record<string, SettingValue> = {}
  for (const field of allFields.value) {
    next[field.id] = field.value
  }
  return next
}

const applyDraftMap = (draft: Record<string, unknown>) => {
  for (const field of allFields.value) {
    const storedValue = draft[field.id]
    if (storedValue === undefined) {
      continue
    }

    if (field.control === 'switch' && typeof storedValue === 'boolean') {
      field.value = storedValue
      continue
    }

    if (field.control === 'number') {
      if (typeof storedValue === 'number' && Number.isFinite(storedValue)) {
        field.value = storedValue
      } else if (typeof storedValue === 'string') {
        const parsed = Number.parseInt(storedValue, 10)
        if (Number.isFinite(parsed)) {
          field.value = parsed
        }
      }
      continue
    }

    if (typeof storedValue === 'string') {
      field.value = storedValue
    }
  }
}

const persistDraft = () => {
  if (typeof window === 'undefined') {
    return
  }

  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(createDraftMap()))
}

const loadDraft = () => {
  if (typeof window === 'undefined') {
    return
  }

  const rawDraft = window.localStorage.getItem(STORAGE_KEY)
  if (!rawDraft) {
    return
  }

  try {
    const parsed = JSON.parse(rawDraft) as Record<string, unknown>
    applyDraftMap(parsed)
  } catch {
    window.localStorage.removeItem(STORAGE_KEY)
  }
}

const updateStringField = (field: SettingField, event: Event) => {
  field.value = (event.target as HTMLInputElement).value
}

const updateNumberField = (field: SettingField, event: Event) => {
  const rawValue = (event.target as HTMLInputElement).value
  field.value = rawValue === '' ? 0 : Number.parseInt(rawValue, 10) || 0
}

const toggleBooleanField = (field: SettingField) => {
  field.value = !Boolean(field.value)
}

const updateSelectField = (field: SettingField, value: string) => {
  field.value = value
  openSelectId.value = null
}

const toggleSelect = (field: SettingField) => {
  openSelectId.value = openSelectId.value === field.id ? null : field.id
}

const selectedOptionLabel = (field: SettingField) => {
  const value = String(field.value)
  return field.options?.find((option) => option.value === value)?.label ?? value
}

const handleDocumentPointerDown = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (!target?.closest('.custom-select')) {
    openSelectId.value = null
  }
}

const handleDocumentKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    openSelectId.value = null
  }
}

watch(
  pages,
  () => {
    if (!hydrated.value) {
      return
    }
    persistDraft()
  },
  { deep: true },
)

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleDocumentKeydown)
  loadDraft()
  hydrated.value = true
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
})
</script>

<template>
  <Transition name="modal">
    <aside v-if="visible" class="settings-mask" @mousedown.self="emit('close')">
      <section class="settings-panel" @click.stop>
        <header class="settings-header">
          <div class="settings-title-block">
            <span class="settings-eyebrow">系统配置</span>
            <h2>设置</h2>
          </div>

          <button class="button-subtle" type="button" @click="emit('close')">关闭</button>
        </header>

        <div class="settings-layout">
          <aside class="settings-nav">
            <nav class="nav-list" aria-label="设置分组">
              <button
                v-for="page in pages"
                :key="page.id"
                type="button"
                class="nav-item"
                :class="{ 'nav-item-active': activePageId === page.id }"
                @click="activePageId = page.id"
              >
                <span>{{ page.label }}</span>
                <small>{{ page.filePath }}</small>
              </button>
            </nav>
          </aside>

          <div class="settings-content">
            <section class="page-header">
              <div>
                <span class="section-kicker">{{ activePage.filePath }}</span>
                <h3>{{ activePage.label }}</h3>
              </div>
            </section>

            <section
              v-for="group in activePage.groups"
              :key="group.id"
              class="group-section"
            >
              <header class="group-header">
                <h4>{{ group.label }}</h4>
              </header>

              <div class="field-list">
                <article
                  v-for="field in group.fields"
                  :key="field.id"
                  class="field-row"
                  :class="{ 'field-row-open': openSelectId === field.id }"
                >
                  <div class="field-main">
                    <div class="field-title-row">
                      <h5>{{ field.label }}</h5>
                    </div>
                    <code>{{ field.envKey }}</code>
                    <p>{{ field.description }}</p>
                    <p v-if="field.hint" class="field-hint">{{ field.hint }}</p>
                  </div>

                  <div class="field-control">
                    <button
                      v-if="field.control === 'switch'"
                      type="button"
                      class="switch-field"
                      :class="{ 'switch-field-active': Boolean(field.value) }"
                      role="switch"
                      :aria-checked="Boolean(field.value)"
                      @click="toggleBooleanField(field)"
                    >
                      <span>{{ field.value ? '开启' : '关闭' }}</span>
                      <span class="switch-track" aria-hidden="true">
                        <span class="switch-thumb" />
                      </span>
                    </button>

                    <div
                      v-else-if="field.control === 'select'"
                      class="custom-select"
                      :data-select-id="field.id"
                    >
                      <button
                        type="button"
                        class="field-input select-trigger"
                        :class="{ 'select-trigger-open': openSelectId === field.id }"
                        :aria-expanded="openSelectId === field.id"
                        @click="toggleSelect(field)"
                      >
                        <span>{{ selectedOptionLabel(field) }}</span>
                        <span class="select-chevron" aria-hidden="true" />
                      </button>

                      <div v-if="openSelectId === field.id" class="select-menu">
                        <button
                          v-for="option in field.options"
                          :key="option.value"
                          type="button"
                          class="select-option"
                          :class="{ 'select-option-active': String(field.value) === option.value }"
                          @click="updateSelectField(field, option.value)"
                        >
                          <span>{{ option.label }}</span>
                          <span v-if="String(field.value) === option.value" class="select-check" aria-hidden="true">✓</span>
                        </button>
                      </div>
                    </div>

                    <input
                      v-else-if="field.control === 'number'"
                      class="field-input"
                      type="number"
                      :value="Number(field.value)"
                      :placeholder="field.placeholder"
                      @input="updateNumberField(field, $event)"
                    />

                  <input
                    v-else
                    class="field-input"
                    type="text"
                    :value="String(field.value)"
                    :placeholder="field.placeholder"
                    @input="updateStringField(field, $event)"
                  />
                  </div>
                </article>
              </div>
            </section>
          </div>
        </div>
      </section>
    </aside>
  </Transition>
</template>

<style scoped>
.settings-mask {
  position: fixed;
  inset: 0;
  z-index: 58;
  background: rgba(5, 9, 18, 0.42);
  display: grid;
  place-items: center;
  padding: 18px;
}

.settings-panel {
  width: min(1040px, 100%);
  max-height: min(90vh, 880px);
  border-radius: 24px;
  border: 1px solid var(--line-soft);
  background: var(--surface-solid);
  box-shadow: var(--shadow-main);
  overflow: hidden;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  user-select: text;
  -webkit-user-select: text;
}

.settings-header {
  padding: 22px 24px 18px;
  border-bottom: 1px solid var(--line-soft);
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}

.settings-title-block {
  display: grid;
  gap: 8px;
}

.settings-eyebrow,
.section-kicker {
  color: var(--accent-main);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.settings-title-block h2,
.page-header h3,
.group-header h4,
.field-title-row h5 {
  margin: 0;
}

.settings-title-block h2 {
  font-size: 30px;
  letter-spacing: -0.03em;
}

.page-header p,
.field-main p {
  margin: 0;
  color: var(--text-secondary);
  line-height: 1.55;
}

.button-subtle {
  border: 1px solid var(--line-soft);
  border-radius: 12px;
  padding: 10px 12px;
  background: var(--surface-main);
  color: var(--text-main);
  font: inherit;
  cursor: pointer;
  transition: transform 0.18s ease, border-color 0.18s ease;
  user-select: none;
  -webkit-user-select: none;
}

.button-subtle:hover {
  transform: translateY(-1px);
  border-color: var(--line-strong);
}

.settings-layout {
  min-height: 0;
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
}

.settings-nav {
  min-height: 0;
  border-right: 1px solid var(--line-soft);
  padding: 18px 14px;
  background: color-mix(in srgb, var(--surface-main) 84%, transparent);
}

.nav-list {
  display: grid;
  gap: 4px;
}

.nav-item {
  border: 0;
  border-left: 2px solid transparent;
  background: transparent;
  color: var(--text-main);
  padding: 10px 12px;
  text-align: left;
  font: inherit;
  cursor: pointer;
  display: grid;
  gap: 4px;
  user-select: none;
  -webkit-user-select: none;
}

.nav-item span,
.field-title-row h5 {
  font-size: 14px;
}

.nav-item small,
.field-main code,
.field-hint {
  color: var(--text-tertiary);
  font-size: 12px;
}

.nav-item-active,
.nav-item:hover {
  border-left-color: var(--accent-main);
  background: var(--surface-tint);
}

.settings-content {
  min-height: 0;
  overflow-y: auto;
  padding: 18px 22px 22px;
  display: grid;
  align-content: start;
  gap: 20px;
}

.page-header {
  padding-bottom: 2px;
}

.group-section {
  display: grid;
  gap: 10px;
  overflow: visible;
}

.group-header {
  padding-bottom: 2px;
}

.group-header h4 {
  font-size: 14px;
  color: var(--text-secondary);
}

.field-list {
  border: 1px solid var(--line-soft);
  border-radius: 18px;
  background: var(--surface-main);
  overflow: visible;
}

.field-row {
  position: relative;
  z-index: 0;
  padding: 16px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 18px;
  align-items: center;
}

.field-row-open {
  z-index: 4;
}

.field-row + .field-row {
  border-top: 1px solid var(--line-soft);
}

.field-main {
  display: grid;
  gap: 6px;
}

.field-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.field-main code {
  word-break: break-all;
}

.field-control {
  position: relative;
  display: grid;
  align-items: center;
}

.custom-select {
  position: relative;
}

.field-input,
.switch-field {
  width: 100%;
  border: 1px solid var(--line-soft);
  border-radius: 14px;
  background: var(--surface-solid);
  color: var(--text-main);
  font: inherit;
}

.field-input {
  min-height: 46px;
  padding: 11px 13px;
  outline: none;
  user-select: text;
  -webkit-user-select: text;
}

.field-input:focus {
  border-color: color-mix(in srgb, var(--accent-main) 55%, var(--line-soft));
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-main) 10%, transparent);
}

.field-input[type='number'] {
  appearance: textfield;
  -moz-appearance: textfield;
}

.field-input[type='number']::-webkit-outer-spin-button,
.field-input[type='number']::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.select-trigger {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  text-align: left;
  cursor: pointer;
}

.select-trigger-open {
  border-color: color-mix(in srgb, var(--accent-main) 55%, var(--line-soft));
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent-main) 10%, transparent);
}

.select-chevron {
  width: 10px;
  height: 10px;
  border-right: 1.5px solid var(--text-tertiary);
  border-bottom: 1.5px solid var(--text-tertiary);
  transform: rotate(45deg) translateY(-2px);
  flex: 0 0 auto;
  transition: transform 0.18s ease;
}

.select-trigger-open .select-chevron {
  transform: rotate(-135deg) translateY(-1px);
}

.select-menu {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  right: 0;
  z-index: 12;
  padding: 6px;
  border: 1px solid var(--line-soft);
  border-radius: 14px;
  background: var(--surface-solid);
  box-shadow: 0 14px 28px rgba(5, 9, 18, 0.12);
  display: grid;
  gap: 4px;
}

.select-option {
  width: 100%;
  min-height: 38px;
  padding: 9px 10px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  color: var(--text-main);
  font: inherit;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  text-align: left;
}

.select-option:hover,
.select-option-active {
  background: var(--surface-tint);
}

.select-check {
  color: var(--accent-main);
  font-size: 13px;
  font-weight: 700;
}

.switch-field {
  min-height: 46px;
  padding: 10px 13px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
}

.switch-field span {
  color: var(--text-secondary);
  font-size: 13px;
}

.switch-field-active {
  border-color: color-mix(in srgb, var(--accent-main) 45%, var(--line-soft));
  background: color-mix(in srgb, var(--accent-main) 10%, var(--surface-solid));
}

.switch-track {
  width: 42px;
  height: 24px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--line-soft) 88%, var(--surface-solid));
  padding: 3px;
  display: inline-flex;
  align-items: center;
  transition: background-color 0.18s ease;
  flex: 0 0 auto;
}

.switch-thumb {
  width: 18px;
  height: 18px;
  border-radius: 999px;
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.18);
  transition: transform 0.18s ease;
}

.switch-field-active .switch-track {
  background: var(--accent-main);
}

.switch-field-active .switch-thumb {
  transform: translateX(18px);
}

.modal-enter-active,
.modal-leave-active {
  transition: background-color 0.22s ease;
}

.modal-enter-active .settings-panel,
.modal-leave-active .settings-panel {
  transition: transform 0.26s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.2s ease;
}

.modal-enter-from {
  background: rgba(5, 9, 18, 0);
}

.modal-enter-from .settings-panel,
.modal-leave-to .settings-panel {
  transform: translateY(10px);
  opacity: 0;
}

@media (max-width: 960px) {
  .settings-layout {
    grid-template-columns: 1fr;
  }

  .settings-nav {
    border-right: 0;
    border-bottom: 1px solid var(--line-soft);
  }

  .nav-list {
    grid-auto-flow: column;
    grid-auto-columns: minmax(180px, 1fr);
    overflow-x: auto;
  }

  .field-row {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .settings-mask {
    padding: 0;
  }

  .settings-panel {
    width: 100%;
    height: 100vh;
    max-height: 100vh;
    border-radius: 0;
  }

  .settings-header,
  .settings-content,
  .settings-nav {
    padding-left: 16px;
    padding-right: 16px;
  }

  .settings-header,
  .page-header {
    display: grid;
  }

  .button-subtle {
    width: 100%;
  }
}
</style>
