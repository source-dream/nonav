<script setup lang="ts">
import type { Share } from '../types'

defineProps<{
  shares: Share[]
}>()

const emit = defineEmits<{
  stop: [share: Share]
  copy: [share: Share]
}>()

const formatDate = (iso: string) => {
  return new Date(iso).toLocaleString()
}
</script>

<template>
  <section class="share-panel">
    <header class="share-header">
      <p>共 {{ shares.length }} 条</p>
    </header>

    <div v-if="shares.length === 0" class="share-empty">当前没有活跃分享</div>

    <ul v-else class="share-list">
      <li v-for="share in shares" :key="share.id" class="share-item">
        <div class="share-title-row">
          <strong>{{ share.siteName }}</strong>
          <span :class="['share-status', `share-status-${share.status}`]">{{ share.status }}</span>
        </div>
        <p class="share-url">{{ share.shareUrl }}</p>
        <div class="share-meta">
          <span>访问 {{ share.accessHits }}</span>
          <span>到期 {{ formatDate(share.expiresAt) }}</span>
        </div>
        <div class="share-actions">
          <button class="button-soft" type="button" @click="emit('copy', share)">复制链接</button>
          <button class="button-warn" type="button" @click="emit('stop', share)" :disabled="share.status !== 'active'">停止分享</button>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.share-panel {
  display: grid;
  gap: 10px;
}

.share-header {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.share-empty {
  color: var(--text-secondary);
  font-size: 14px;
  border: 1px dashed var(--line-soft);
  border-radius: 12px;
  padding: 20px 14px;
}

.share-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 10px;
}

.share-item {
  border-radius: 12px;
  border: 1px solid var(--line-soft);
  background: var(--surface-main);
  padding: 12px;
  display: grid;
  gap: 8px;
}

.share-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.share-status {
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 999px;
}

.share-status-active {
  background: var(--surface-accent);
  color: var(--accent-main);
}

.share-status-stopped,
.share-status-expired {
  background: var(--surface-tint);
  color: var(--text-secondary);
}

.share-url {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.share-meta {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--text-tertiary);
}

.share-actions {
  display: flex;
  gap: 8px;
}

button {
  border: 0;
  border-radius: 10px;
  padding: 8px 10px;
  font-family: inherit;
  font-size: 13px;
  cursor: pointer;
}

.button-soft {
  background: var(--surface-tint);
  color: var(--text-main);
}

.button-warn {
  background: #7d3131;
  color: white;
}

.button-warn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
