// Состояние маскота (08-DESIGN §2.2–2.3, BR-90, BR-91, SC-25): дремлет и засыпает без действий,
// просыпается от движения, клика или события; события — по приоритету error → has_answer → needs_confirm → reminder.
import { useCallback, useEffect, useRef, useState } from "react";
import { MASCOT_TIMERS, mascotMs } from "@/shared/config/mascot";
import type { MascotMood } from "./types";

export type MascotInputs = { thinking?: boolean; error?: boolean; hasAnswer?: boolean; needsConfirm?: boolean; reminder?: boolean };

const KEY = "naparnik.mascot.greeting";
const WAKE_MS = 800; // «просыпается» — один раз, ≤ 1 с
const GREET_MS = 1500; // «машет» — один раз, ≤ 1,5 с
const REACT_MS = 1400; // короткая реакция на ответ или ошибку

type Rest = "idle" | "drowsy" | "sleeping";

/** Первое открытие за день и не чаще раза в час (greeting_cooldown_min). */
const shouldGreet = (now: number) => {
  try {
    const last = Number(localStorage.getItem(KEY) ?? 0);
    const sameDay = last && new Date(last).toDateString() === new Date(now).toDateString();
    if (sameDay || now - last < MASCOT_TIMERS.greetingCooldownMin * 60_000) return false;
    localStorage.setItem(KEY, String(now));
    return true;
  } catch { return false; }
};

export const useMascotMood = ({ thinking, error, hasAnswer, needsConfirm, reminder }: MascotInputs) => {
  const [rest, setRest] = useState<Rest>("idle");
  const [oneShot, setOneShot] = useState<MascotMood | null>(() => (shouldGreet(Date.now()) ? "greeting" : null));
  const restRef = useRef<Rest>("idle");
  const timers = useRef<number[]>([]);
  const shotTimer = useRef<number | undefined>(undefined);

  const play = useCallback((mood: MascotMood, ms: number) => {
    window.clearTimeout(shotTimer.current);
    setOneShot(mood);
    shotTimer.current = window.setTimeout(() => setOneShot(null), ms);
  }, []);

  const schedule = useCallback(() => {
    timers.current.forEach((t) => window.clearTimeout(t));
    timers.current = [
      window.setTimeout(() => { restRef.current = "drowsy"; setRest("drowsy"); }, mascotMs(MASCOT_TIMERS.drowsyAfterSec)),
      window.setTimeout(() => { restRef.current = "sleeping"; setRest("sleeping"); }, mascotMs(MASCOT_TIMERS.sleepAfterSec)),
    ];
  }, []);

  /** Действие человека: будит маскота (с коротким «просыпается»), таймер сна — заново. */
  const poke = useCallback(() => {
    if (restRef.current === "sleeping") play("waking", WAKE_MS);
    if (restRef.current !== "idle") { restRef.current = "idle"; setRest("idle"); }
    schedule();
  }, [play, schedule]);

  // Приветствие — один раз, ≤ 1,5 с.
  useEffect(() => {
    if (oneShot !== "greeting") return;
    shotTimer.current = window.setTimeout(() => setOneShot(null), GREET_MS);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    schedule();
    let last = 0;
    const onActivity = () => { const now = Date.now(); if (now - last > 500) { last = now; poke(); } };
    const events = ["pointermove", "pointerdown", "keydown", "scroll", "touchstart"] as const;
    events.forEach((e) => window.addEventListener(e, onActivity, { passive: true }));
    return () => {
      events.forEach((e) => window.removeEventListener(e, onActivity));
      timers.current.forEach((t) => window.clearTimeout(t));
      window.clearTimeout(shotTimer.current);
    };
  }, [poke, schedule]);

  // Событие будит и даёт одну короткую реакцию; дальше — статичный бейдж (08-DESIGN §2.4).
  const prev = useRef({ hasAnswer, error });
  useEffect(() => {
    if (hasAnswer && !prev.current.hasAnswer) { poke(); play("has_answer", REACT_MS); }
    if (error && !prev.current.error) { poke(); play("error", REACT_MS); }
    prev.current = { hasAnswer, error };
  }, [hasAnswer, error, poke, play]);
  useEffect(() => { if (thinking || needsConfirm || reminder) poke(); }, [thinking, needsConfirm, reminder, poke]);

  // Событие — короткая реакция и статичный кадр с бейджем; без действий маскот всё равно дремлет и засыпает (BR-91),
  // бейдж остаётся. Новое событие будит (эффекты выше).
  const awake: MascotMood = error ? "error" : hasAnswer ? "has_answer" : needsConfirm ? "needs_confirm" : reminder ? "reminder" : "idle";
  const mood: MascotMood = thinking ? "thinking" : oneShot ?? (rest === "idle" ? awake : rest);
  return { mood, poke };
};

/** Подпись состояния для подсказки и скринридера (08-DESIGN §2.6). */
export const moodLabel = (mood: MascotMood): string => ({
  idle: "Спроси меня",
  drowsy: "Задремал… Спроси что угодно",
  sleeping: "Сплю. Нажми, если нужен",
  waking: "Я тут!",
  greeting: "Привет!",
  thinking: "Думаю…",
  has_answer: "Ответ готов",
  reminder: "Скоро событие",
  needs_confirm: "Черновик ждёт подтверждения",
  error: "Не получилось. Повторить?",
  minimized: "Напарник",
})[mood];
