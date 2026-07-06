<template>
  <section v-if="show">
    <h1>{{ show.title || show.source_feed_url }}</h1>

    <ul>
      <li v-for="e in episodes" :key="e.episode_id">
        <RouterLink :to="{ name: 'episode', params: { id: e.episode_id } }">
          {{ e.title }}
        </RouterLink>
        <div>
          listened {{ e.last_played_at }} · {{ e.transcript_status }}
        </div>
      </li>
    </ul>
  </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { getShow } from '../api'

const route = useRoute()
const show = ref(null)
const episodes = ref([])

onMounted(async () => {
  const data = await getShow(route.params.id)
  show.value = data.show
  episodes.value = data.episodes || []
})
</script>
