/** Таймеры маскота из docs/04-mock-data/notifications.json (SC-25, BR-91). */
export const MASCOT_TIMERS = {
  drowsyAfterSec: 60,
  sleepAfterSec: 90,
  greetingCooldownMin: 60,
  wakeTriggers: ["click", "hover", "event"] as const,
} as const;

/** Ускорение таймеров в тестах: VITE_MASCOT_TIMER_SCALE=0.05 */
export const mascotScale = (): number => {
  const raw = import.meta.env.VITE_MASCOT_TIMER_SCALE;
  const n = raw ? Number(raw) : 1;
  return Number.isFinite(n) && n > 0 ? n : 1;
};

export const mascotMs = (sec: number) => Math.max(50, Math.round(sec * 1000 * mascotScale()));
