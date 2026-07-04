import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

// Авторизации в бэкенде НЕТ (осознанное упрощение под хакатон): проверка
// учётных данных — клиентская, пара захардкожена. Экран входа — аккуратная
// «дверь» и место для бренда, а не настоящий сервер аутентификации.
const CREDENTIALS = { login: 'nornickel', password: 'nornickel' }
const TOKEN_KEY = 'hf-auth'

// Статичный профиль (ролей нет, пользователь один).
export const CURRENT_USER = {
  initials: 'ИС',
  name: 'Игорь Соколов',
  role: 'Металлург · R&D',
}

export const useAuthStore = defineStore('auth', () => {
  const authed = ref(localStorage.getItem(TOKEN_KEY) === '1')
  const isAuthenticated = computed(() => authed.value)

  // Локальная проверка — без обращения к API.
  function login(loginValue: string, password: string): boolean {
    if (loginValue === CREDENTIALS.login && password === CREDENTIALS.password) {
      authed.value = true
      localStorage.setItem(TOKEN_KEY, '1')
      return true
    }
    return false
  }

  function logout(): void {
    authed.value = false
    localStorage.removeItem(TOKEN_KEY)
  }

  return { isAuthenticated, login, logout, user: CURRENT_USER }
})
