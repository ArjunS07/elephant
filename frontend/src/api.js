// Thin fetch wrapper: attaches the stored JWT, decodes JSON, and on 401 clears
// the token and bounces to /login. Routing lives in router.js; this only knows
// how to talk to the API.

const TOKEN_KEY = 'token'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

async function request(method, path, { body, query } = {}) {
  const url = new URL(path, window.location.origin)
  if (query) {
    for (const [k, v] of Object.entries(query)) url.searchParams.set(k, v)
  }

  const headers = {}
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(url, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    const message = (await res.text()).trim() || `request failed: ${res.status}`
    // A 401 while we hold a token means the session expired -> bounce to login.
    // A 401 without a token (e.g. a bad login) is just a failed request; let the
    // caller show the message.
    if (res.status === 401 && token) {
      clearToken()
      window.location.assign('/login')
    }
    throw new Error(message)
  }
  return res.status === 204 ? null : res.json()
}

export async function login(username, password) {
  const { token } = await request('POST', '/api/login', { body: { username, password } })
  setToken(token)
  return token
}

export function getShows() {
  return request('GET', '/api/shows')
}

export function addShow(feedURL) {
  return request('POST', '/api/podcasts/register', { query: { feed_url: feedURL } })
}

export function getShow(id) {
  return request('GET', `/api/shows/${id}`)
}

export function getTranscript(id) {
  return request('GET', `/api/episodes/${id}/transcript`)
}
