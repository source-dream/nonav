<script setup lang="ts">
import type { Site } from '../types'

defineProps<{
  site: Site
  shareInfo?: {
    active: boolean
    total: number
  }
}>()

const emit = defineEmits<{
  open: [site: Site]
  openSettings: [site: Site]
}>()

const handleContext = (event: MouseEvent, site: Site) => {
  event.preventDefault()
  emit('openSettings', site)
}
</script>

<template>
  <article class="site-card" @click="emit('open', site)" @contextmenu="handleContext($event, site)">
    <div class="site-card-top">
      <span class="site-icon">{{ site.icon || '🌐' }}</span>
      <span class="site-group">{{ site.groupName || '未分组' }}</span>
    </div>
    <h3 class="site-title">{{ site.name }}</h3>
    <p class="site-url">{{ site.url }}</p>
    <footer class="site-meta">
      <span>访问 {{ site.clickCount }}</span>
      <span class="site-share-pill" :class="{ 'site-share-pill-active': shareInfo?.active }">
        {{ shareInfo?.active ? '分享中' : '未分享' }}
        <em v-if="shareInfo && shareInfo.total > 0">{{ shareInfo.total }}</em>
      </span>
    </footer>
  </article>
</template>

<style scoped>
.site-card {
  display: grid;
  gap: 10px;
  padding: 14px;
  border-radius: 14px;
  border: 1px solid var(--line-soft);
  background: var(--surface-main);
  cursor: pointer;
  transition: transform 0.18s ease, border-color 0.18s ease;
}

.site-card:hover {
  transform: translateY(-2px);
  border-color: var(--line-strong);
}

.site-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.site-icon {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  font-size: 18px;
  background: var(--surface-tint);
}

.site-group {
  font-size: 11px;
  color: var(--text-secondary);
  border-radius: 999px;
  background: var(--surface-tint);
  padding: 4px 8px;
}

.site-title {
  margin: 0;
  font-size: 17px;
  color: var(--text-main);
  line-height: 1.2;
}

.site-url {
  margin: 0;
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.site-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: var(--text-tertiary);
  font-size: 12px;
}

.site-share-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 999px;
  background: var(--surface-tint);
  color: var(--text-secondary);
  padding: 4px 8px;
}

.site-share-pill-active {
  color: var(--accent-main);
  background: var(--surface-accent);
}

.site-share-pill em {
  font-style: normal;
  font-size: 11px;
  opacity: 0.78;
}
</style>
