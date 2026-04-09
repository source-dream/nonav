<script setup lang="ts">
import { computed, ref } from 'vue'
import type { GatewayHealth, GatewayStatusSnapshot, ServiceStatusItem, SystemLogEntry, SystemLogSource } from '../types'

const props = defineProps<{
  visible: boolean
  snapshot: GatewayStatusSnapshot
  logs: SystemLogEntry[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  close: []
}>()

const logSourceFilter = ref<'all' | SystemLogSource>('all')
const showOnlyAlerts = ref(false)

const healthMeta: Record<GatewayHealth, { label: string }> = {
  online: { label: '正常' },
  degraded: { label: '降级' },
  offline: { label: '异常' },
}

const sourceLabel: Record<SystemLogSource, string> = {
  'nonav-gateway': '网关',
  nonav: '导航',
}

const healthLabel = computed(() => healthMeta[props.snapshot.health].label)

const visibleLogs = computed(() => {
  return props.logs.filter((item) => {
    const matchesSource = logSourceFilter.value === 'all' || item.source === logSourceFilter.value
    const matchesTone = !showOnlyAlerts.value || item.tone !== 'normal'
    return matchesSource && matchesTone
  })
})

const toneLabel = (tone: SystemLogEntry['tone']) => {
  if (tone === 'error') {
    return 'error'
  }

  if (tone === 'warning') {
    return 'warn'
  }

  return 'info'
}

const serviceToneLabel = (health: ServiceStatusItem['health']) => healthMeta[health].label

const formatTimestamp = (value: string) => {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return value
  }

  return parsed.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}
</script>

<template>
  <Transition name="modal">
    <aside v-if="visible" class="modal-mask" @mousedown.self="emit('close')">
      <section class="modal-panel" @click.stop>
        <header class="modal-head">
          <div class="modal-title-row">
            <h2>系统状态</h2>
            <span :class="['health-badge', `health-badge-${snapshot.health}`]">{{ healthLabel }}</span>
          </div>
          <button class="button-subtle" type="button" @click="emit('close')">关闭</button>
        </header>

        <section class="summary-shell">
          <div class="summary-row summary-row-services">
            <div v-for="service in snapshot.services" :key="service.key" class="summary-item">
              <div class="summary-item-head">
                <span class="summary-key">{{ service.label }}</span>
                <span :class="['summary-status', `summary-status-${service.health}`]">{{ serviceToneLabel(service.health) }}</span>
              </div>
              <span class="summary-value">{{ service.summary }}</span>
            </div>
          </div>
        </section>

        <section class="log-panel">
          <div class="log-toolbar">
            <div class="log-toolbar-title">系统日志</div>
            <div class="log-filter">
              <button
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': logSourceFilter === 'all' }"
                @click="logSourceFilter = 'all'"
              >
                全部
              </button>
              <button
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': logSourceFilter === 'nonav-gateway' }"
                @click="logSourceFilter = 'nonav-gateway'"
              >
                网关
              </button>
              <button
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': logSourceFilter === 'nonav' }"
                @click="logSourceFilter = 'nonav'"
              >
                导航
              </button>
              <button
                type="button"
                class="filter-chip"
                :class="{ 'filter-chip-active': showOnlyAlerts }"
                @click="showOnlyAlerts = !showOnlyAlerts"
              >
                仅异常
              </button>
            </div>
          </div>

          <div class="log-scroll">
            <p v-if="error" class="log-banner log-banner-error">{{ error }}</p>
            <p v-else-if="loading && visibleLogs.length === 0" class="log-banner">正在加载日志...</p>
            <ul class="log-lines">
              <li v-for="item in visibleLogs" :key="item.id" class="log-line" :class="`log-line-${item.tone}`">
                <div class="log-main-line">
                  <span class="log-time">{{ formatTimestamp(item.timestamp) }}</span>
                  <span class="log-bracket log-source">[{{ sourceLabel[item.source] }}]</span>
                  <span class="log-bracket" :class="`log-level-${item.tone}`">[{{ toneLabel(item.tone) }}]</span>
                  <span class="log-event">{{ item.event }}</span>
                  <span class="log-req">req={{ item.req }}</span>
                  <span class="log-message">{{ item.message }}</span>
                </div>
                <div v-if="item.details.length > 0" class="log-detail-line">
                  <span class="log-detail-indent">↳</span>
                  <span>{{ item.details.join(' · ') }}</span>
                </div>
              </li>
            </ul>

            <p v-if="visibleLogs.length === 0" class="log-empty">当前筛选条件下没有日志。</p>
          </div>
        </section>
      </section>
    </aside>
  </Transition>
</template>

<style scoped>
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
  width: min(980px, 100%);
  max-height: min(88vh, 860px);
  border-radius: 18px;
  border: 1px solid var(--line-soft);
  background: linear-gradient(180deg, var(--surface-solid), var(--surface-main));
  box-shadow: var(--shadow-main);
  padding: 18px;
  overflow: hidden;
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  gap: 12px;
  user-select: text;
  -webkit-user-select: text;
}

.modal-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 14px;
}

.modal-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.modal-title-row h2 {
  margin: 0;
  font-size: 22px;
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
  user-select: none;
  -webkit-user-select: none;
}

.health-badge,
.summary-status {
  border-radius: 999px;
  padding: 5px 10px;
  font-size: 12px;
  font-weight: 600;
}

.health-badge-online,
.summary-status-online {
  background: rgba(34, 169, 94, 0.14);
  color: #1d8f4d;
}

.health-badge-degraded,
.summary-status-degraded {
  background: rgba(214, 151, 23, 0.16);
  color: #bb7a00;
}

.health-badge-offline,
.summary-status-offline {
  background: rgba(199, 75, 75, 0.16);
  color: #c74b4b;
}

.summary-shell {
  display: grid;
  gap: 10px;
}

.summary-row {
  display: grid;
  gap: 10px;
}

.summary-row-services {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.summary-item {
  min-width: 0;
  border-radius: 14px;
  border: 1px solid var(--line-soft);
  background: var(--surface-main);
}

.summary-item {
  padding: 12px 14px;
  display: grid;
  grid-template-rows: auto auto;
  gap: 10px;
  min-height: 88px;
}

.summary-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.summary-key {
  color: var(--text-secondary);
  font-size: 13px;
}

.summary-value {
  min-width: 0;
  font-size: 14px;
  color: var(--text-main);
  line-height: 1.4;
  white-space: normal;
  word-break: break-word;
}

.log-panel {
  min-height: 0;
  border-radius: 16px;
  border: 1px solid #243148;
  background: #0f1624;
  color: #d7e2f5;
  overflow: hidden;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
}

.log-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(173, 188, 213, 0.14);
  background: rgba(6, 11, 19, 0.45);
}

.log-toolbar-title {
  color: #eef4ff;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.log-filter {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.filter-chip {
  border: 1px solid rgba(173, 188, 213, 0.18);
  border-radius: 10px;
  background: rgba(17, 25, 38, 0.9);
  color: #adbcd5;
  padding: 6px 10px;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
}

.filter-chip-active {
  border-color: rgba(143, 177, 255, 0.5);
  background: rgba(72, 103, 177, 0.24);
  color: #eef4ff;
}

.log-scroll {
  min-height: 0;
  overflow: auto;
  scrollbar-gutter: stable;
  padding: 6px 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
}

.log-lines {
  margin: 0;
  padding: 0;
  list-style: none;
}

.log-line {
  padding: 8px 14px;
  border-bottom: 1px solid rgba(173, 188, 213, 0.08);
}

.log-line:last-child {
  border-bottom: 0;
}

.log-main-line,
.log-detail-line {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  flex-wrap: wrap;
  line-height: 1.5;
}

.log-main-line {
  color: #d7e2f5;
  font-size: 13px;
}

.log-detail-line {
  margin-top: 4px;
  color: #8d9ab2;
  font-size: 12px;
  padding-left: 6px;
}

.log-time {
  color: #8d9ab2;
  white-space: nowrap;
}

.log-bracket,
.log-req,
.log-event {
  white-space: nowrap;
}

.log-source {
  color: #8fb1ff;
}

.log-level-normal {
  color: #adbcd5;
}

.log-level-warning {
  color: #ffd36e;
}

.log-level-error {
  color: #ff9a9a;
}

.log-event {
  color: #eef4ff;
  font-weight: 600;
}

.log-req {
  color: #78a6ff;
}

.log-message {
  min-width: min(280px, 100%);
  flex: 1 1 360px;
  color: #d7e2f5;
}

.log-line-warning {
  background: linear-gradient(90deg, rgba(214, 151, 23, 0.08), transparent 28%);
}

.log-line-error {
  background: linear-gradient(90deg, rgba(199, 75, 75, 0.12), transparent 28%);
}

.log-detail-indent {
  color: #5f7297;
}

.log-empty {
  margin: 0;
  padding: 18px 14px;
  color: #8d9ab2;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
}

.log-banner {
  margin: 0;
  padding: 10px 14px;
  color: #8d9ab2;
  font-size: 12px;
  border-bottom: 1px solid rgba(173, 188, 213, 0.08);
}

.log-banner-error {
  color: #ffb0b0;
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

@media (max-width: 760px) {
  .modal-panel {
    padding: 14px;
  }

  .modal-head,
  .log-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .summary-row-services {
    grid-template-columns: 1fr;
  }

  .summary-item {
    justify-content: flex-start;
  }

  .log-filter {
    width: 100%;
  }

  .filter-chip {
    flex: 1 1 calc(50% - 8px);
    text-align: center;
  }

  .log-main-line,
  .log-detail-line {
    gap: 6px;
  }
}
</style>
