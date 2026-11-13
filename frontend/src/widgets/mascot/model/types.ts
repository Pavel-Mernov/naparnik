/** Состояния из docs/03-requirements/08-DESIGN.md §2.2 */
export type MascotMood =
  | "idle"
  | "drowsy"
  | "sleeping"
  | "waking"
  | "greeting"
  | "thinking"
  | "has_answer"
  | "reminder"
  | "needs_confirm"
  | "error"
  | "minimized";

export type MascotSignal = {
  thinking: boolean;
  error: boolean;
  hasAnswer: boolean;
  needsConfirm: boolean;
  /** Ближайшее событие, напр. «15:00 · 1:1» */
  reminderLabel?: string;
  /** Сколько требует внимания (бейдж) */
  badgeCount: number;
};

export type MascotEdge = "left" | "right";

export type MascotPersist = {
  edge: MascotEdge;
  offsetY: number;
  hiddenUntil: number;
  minimized: boolean;
  lastGreetingAt: number;
  greetingDay: string;
};
