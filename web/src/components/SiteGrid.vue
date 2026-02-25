<script setup lang="ts">
import SiteCard from './SiteCard.vue'
import type { Site } from '../types'

defineProps<{
  sites: Site[]
  shareLookup: Record<number, { active: boolean; total: number }>
}>()

const emit = defineEmits<{
  open: [site: Site]
  openSettings: [site: Site]
}>()
</script>

<template>
  <section class="site-grid">
    <SiteCard
      v-for="site in sites"
      :key="site.id"
      :site="site"
      :share-info="shareLookup[site.id]"
      @open="emit('open', $event)"
      @open-settings="emit('openSettings', $event)"
    />
  </section>
</template>

<style scoped>
.site-grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

@media (max-width: 1200px) {
  .site-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .site-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .site-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .site-grid {
    grid-template-columns: 1fr;
  }
}
</style>
