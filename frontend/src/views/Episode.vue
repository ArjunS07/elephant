<template>
  <section>
    <h1>Episode</h1>
    <p>Transcript for episode {{ route.params.id }} goes here.</p>
    <RouterLink :to="{ name: 'search' }">Back to search</RouterLink>
  </section>
</template>

<script setup>
import { useRoute } from 'vue-router'

const route = useRoute()
</script>
