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

export type MascotEdge = "left" | "right";
