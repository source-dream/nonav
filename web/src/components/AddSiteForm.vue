<script setup lang="ts">
import { computed, reactive } from 'vue'

interface CreateSitePayload {
  name: string
  url: string
  groupName: string
}

const emit = defineEmits<{
  submit: [payload: CreateSitePayload]
}>()

const form = reactive({
  name: '',
  url: '',
  groupName: '',
})

const canSubmit = computed(() => form.name.trim().length > 0 && form.url.trim().length > 0)

const handleSubmit = () => {
  if (!canSubmit.value) {
    return
  }

  emit('submit', {
    name: form.name.trim(),
    url: form.url.trim(),
    groupName: form.groupName.trim(),
  })

  form.name = ''
  form.url = ''
  form.groupName = ''
}
</script>

<template>
  <form class="site-form" @submit.prevent="handleSubmit">
    <div class="field-group field-group-two">
      <input v-model="form.name" type="text" placeholder="站点名称" required />
      <input v-model="form.url" type="url" placeholder="https://intranet.example.local" required />
    </div>
    <div class="field-group">
      <input v-model="form.groupName" type="text" placeholder="分组（可选）" />
    </div>
    <button type="submit" :disabled="!canSubmit">添加网站</button>
  </form>
</template>

<style scoped>
.site-form {
  display: grid;
  gap: 12px;
}

.field-group {
  display: grid;
  gap: 10px;
}

.field-group-two {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

input,
button {
  border: 0;
  border-radius: var(--radius-m);
  padding: 12px 14px;
  font-size: 14px;
  font-family: inherit;
}

input {
  background: var(--panel-input);
  color: var(--text-main);
  box-shadow: inset 0 0 0 1px var(--line-soft);
}

button {
  justify-self: start;
  background: var(--accent-main);
  color: white;
  cursor: pointer;
  transition: transform 0.2s ease, opacity 0.2s ease;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

button:not(:disabled):hover {
  transform: translateY(-1px);
}

@media (max-width: 768px) {
  .field-group-two {
    grid-template-columns: 1fr;
  }

  button {
    width: 100%;
  }
}
</style>
