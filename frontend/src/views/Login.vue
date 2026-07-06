<template>
  <section>
    <h1>Log in</h1>
    <form @submit.prevent="submit">
      <input v-model="username" placeholder="username" autocomplete="username" />
      <input v-model="password" type="password" placeholder="password" autocomplete="current-password" />
      <button type="submit">Log in</button>
    </form>
    <p v-if="error">{{ error }}</p>
  </section>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { login } from '../api'

const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    await login(username.value, password.value)
    router.push({ name: 'shows' })
  } catch (e) {
    error.value = e.message
  }
}
</script>
