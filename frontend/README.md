# Фабрика гипотез — Frontend

SPA на **Vue 3 + Vite + TypeScript** для бэкенда Hypothesis Factory (Go/Fiber).

## Стек

- **Vue 3.5** (`<script setup>`, Composition API) + **TypeScript**
- **Vite 6** — dev-сервер и сборка
- **Tailwind CSS v4** — стили (CSS-first `@theme`, без `tailwind.config.js`), плагин `@tailwindcss/vite`
- **reka-ui** — headless UI-примитивы (напр. `AppSelect`)
- **Pinia** — состояние (стор прогонов с поллингом статуса)
- **vue-router 4** — маршрутизация
- **vee-validate + zod** (`@vee-validate/zod`) — валидация форм
- **axios** — HTTP-клиент
- **@iconify/vue** + **@iconify-json/lucide** — иконки (lucide)
- **vue-sonner** — тост-уведомления

## Структура

```
frontend/
├── index.html
├── vite.config.ts        # dev-прокси /v1 и /health → :8080
├── env.d.ts
├── .env.example
└── src/
    ├── main.ts           # точка входа
    ├── App.vue           # оболочка + навигация + Toaster + переключатель темы
    ├── router/           # маршруты
    ├── stores/           # Pinia-сторы (runs + поллинг)
    ├── api/              # HTTP-клиент, типы под DTO бэкенда, эндпоинты
    ├── components/ui/    # переиспользуемые UI-компоненты (AppSelect на reka-ui)
    ├── composables/      # useTheme (тёмная тема)
    ├── views/            # страницы (список/создание/детали прогона, документы)
    └── assets/           # main.css — Tailwind v4 + дизайн-токены
```

## Запуск

```bash
cd frontend
cp .env.example .env      # при необходимости поправьте адрес бэкенда
npm install
npm run dev               # http://localhost:5173
```

Бэкенд должен слушать `http://localhost:8080` (переопределяется через
`VITE_API_PROXY_TARGET`). Dev-сервер проксирует `/v1/*` и `/health` на него, так
что CORS не требуется.

## Команды

| Команда | Действие |
|---------|----------|
| `npm run dev` | dev-сервер с HMR |
| `npm run build` | проверка типов + прод-сборка в `dist/` |
| `npm run preview` | локальный предпросмотр прод-сборки |
| `npm run type-check` | только `vue-tsc` |
| `npm run lint` | ESLint с автофиксом |
| `npm run format` | Prettier |

## Конфигурация

- `VITE_API_BASE_URL` — префикс API. Пусто в dev (идём через прокси); абсолютный
  URL бэкенда в проде.
- `VITE_API_PROXY_TARGET` — куда dev-сервер проксирует API.

API-контракт (эндпоинты `/v1/runs`, `/v1/documents`, `/v1/hypotheses/*/feedback`)
типизирован в `src/api/types.ts` под DTO бэкенда (`out/`, `in/`).
