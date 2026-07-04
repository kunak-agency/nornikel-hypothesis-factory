import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

import { http } from '@/api/client'

// Ключи в localStorage.
const TOKEN_KEY = 'hf-auth-token'
const USER_KEY = 'hf-auth-user'

export interface AuthUser {
  email: string
  name: string
}

/**
 * Стор авторизации.
 *
 * Бэкенд Hypothesis Factory пока не отдаёт эндпоинт логина, поэтому `login`
 * работает как клиентская заглушка: валидирует непустые креды и кладёт
 * псевдо-токен в localStorage. Когда появится реальный `POST /v1/auth/login`,
 * достаточно заменить тело `login()` (см. пометку STUB) — всё остальное
 * (гард роутера, шапка, http-интерсептор) продолжит работать.
 */
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem(TOKEN_KEY))
  const user = ref<AuthUser | null>(readUser())

  const isAuthenticated = computed(() => token.value !== null)

  // Проставляем Authorization для всех запросов, если токен уже есть.
  applyAuthHeader(token.value)

  async function login(email: string, password: string): Promise<void> {
    // --- STUB: заменить на реальный запрос к бэкенду ---
    // const { data } = await http.post('/v1/auth/login', { email, password })
    // setSession(data.token, data.user)
    await new Promise((r) => setTimeout(r, 450)) // имитация сети
    if (!email || password.length < 6) {
      throw new Error('Неверный email или пароль')
    }
    const name = email.split('@')[0].replace(/[._-]+/g, ' ')
    setSession(`stub.${btoa(email)}.${Date.now()}`, {
      email,
      name: name.charAt(0).toUpperCase() + name.slice(1),
    })
    // --- /STUB ---
  }

  function logout(): void {
    token.value = null
    user.value = null
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
    applyAuthHeader(null)
  }

  function setSession(newToken: string, newUser: AuthUser): void {
    token.value = newToken
    user.value = newUser
    localStorage.setItem(TOKEN_KEY, newToken)
    localStorage.setItem(USER_KEY, JSON.stringify(newUser))
    applyAuthHeader(newToken)
  }

  return { token, user, isAuthenticated, login, logout }
})

function readUser(): AuthUser | null {
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as AuthUser
  } catch {
    return null
  }
}

function applyAuthHeader(token: string | null): void {
  if (token) {
    http.defaults.headers.common.Authorization = `Bearer ${token}`
  } else {
    delete http.defaults.headers.common.Authorization
  }
}
