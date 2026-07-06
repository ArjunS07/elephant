<template>
  <section>
    <h1>Your podcasts</h1>

    <form @submit.prevent="submitAdd">
      <input v-model="feedURL" placeholder="feed URL" />
      <button type="submit">Add show</button>
    </form>
    <p v-if="error">{{ error }}</p>

    <ul>
      <li v-for="s in shows" :key="s.show_id">
        <RouterLink :to="{ name: 'show', params: { id: s.show_id } }">
          {{ s.title || s.source_feed_url }}
        </RouterLink>
        <div>
          <code>{{ s.custom_feed_url }}</code>
          <button @click="copy(s.custom_feed_url)">Copy</button>
        </div>
      </li>
    </ul>
  </section>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getShows, addShow } from '../api'

const shows = ref([])
const feedURL = ref('')
const error = ref('')

async function load() {
  shows.value = (await getShows()) || []
}

async function submitAdd() {
  error.value = ''
  try {
    await addShow(feedURL.value)
    feedURL.value = ''
    await load()
  } catch (e) {
    error.value = e.message
  }
}

function copy(text) {
  navigator.clipboard.writeText(text)
}

onMounted(load)
</script>
