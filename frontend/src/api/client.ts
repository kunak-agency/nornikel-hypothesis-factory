import axios, { type AxiosInstance } from 'axios'

// В dev VITE_API_BASE_URL пуст → относительные пути идут через vite-прокси (/v1).
// В проде задаётся абсолютный URL бэкенда.
const baseURL = import.meta.env.VITE_API_BASE_URL || ''

export const http: AxiosInstance = axios.create({
  baseURL,
  headers: { 'Content-Type': 'application/json' },
})
