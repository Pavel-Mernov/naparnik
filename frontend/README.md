# frontend — «Доска дня»

**Владельцы:** роль 3 (Сегодня, анимации, токены) и роль 4 (Люди, Cmd+K, Ментор).

Стек выбран: React 19 + TypeScript + Vite, Tailwind CSS v4 (свои токены в `@theme`), `motion`, `cmdk`,
`@dnd-kit/core`, `sonner`, `react-router-dom`, vitest. MUI не используем — см. [docs/decisions.md](../docs/decisions.md) #7.
Визуальная концепция и правила анимаций — [docs/05-ux/visual-concept.md](../docs/05-ux/visual-concept.md).

## Структура (план)

```
frontend/src/
├── main.tsx, App.tsx      роутинг, MotionConfig reducedMotion="user"
├── styles.css             токены: бумага, чернила, один акцент; шрифты Unbounded / Onest / JetBrains Mono
├── api.ts, types.ts       единый клиент и типы строго по docs/api-contract.md
├── pages/                 Login, Today, People, Plan, Memory, Friday, Mentor
├── components/            Timeline, TaskSticker, BusinessCard (переворот), CommandPalette (Cmd+K), Rail
├── mocks/                 фикстуры по контракту — VITE_MOCK=1, работаем без backend
└── utils/                 расчёт слотов таймлайна, форматирование времени (+ тесты)
```

## Запуск

```bash
npm install
VITE_MOCK=1 npm run dev     # без backend, на фикстурах
npm run dev                 # с backend на :8000 (прокси /api)
```
