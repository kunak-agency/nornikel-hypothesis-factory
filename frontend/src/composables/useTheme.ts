import { ref, watch } from 'vue'

// Тема хранится в localStorage; класс `dark` навешивается на <html>.
const STORAGE_KEY = 'hf-theme'

const prefersDark =
  typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches

const stored = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null
const isDark = ref(stored ? stored === 'dark' : prefersDark)

function apply(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark)
}

watch(
  isDark,
  (dark) => {
    apply(dark)
    localStorage.setItem(STORAGE_KEY, dark ? 'dark' : 'light')
  },
  { immediate: true },
)

export function useTheme() {
  function toggle() {
    isDark.value = !isDark.value
  }
  return { isDark, toggle }
}
