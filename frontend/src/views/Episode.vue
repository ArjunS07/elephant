<template>
  <section v-if="episode">
    <h1>{{ episode.title }}</h1>
    <p v-for="s in episode.segments" :key="s.idx">
      {{ formatMs(s.start_ms) }} {{ s.text }}
    </p>
  </section>
  <p v-else-if="error">{{ error }}</p>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getTranscript } from '../api'

const route = useRoute()
const episode = ref(null)
const error = ref('')

function formatMs(ms) {
  const total = Math.floor(ms / 1000)
  const m = Math.floor(total / 60)
  const s = String(total % 60).padStart(2, '0')
  return `[${m}:${s}]`
}

onMounted(async () => {
  try {
    episode.value = await getTranscript(route.params.id)
  } catch (e) {
    error.value = e.message
  }
})
</script>
