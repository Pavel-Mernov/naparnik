// Маскот в «3D» (08-DESIGN §2): объёмный рендер наклоняется в перспективе к курсору, блик и тень едут вместе с
// наклоном, нажатие чуть вдавливает. Поверх — состояния из §2.2: дыхание и моргание, дремота, сон с «z z z»,
// пробуждение, приветствие, «…» пока думает, прыжок на готовый ответ, растерянность при ошибке.
// prefers-reduced-motion → статичные кадры: наклона и анимаций нет, состояние видно по векам, облачку и бейджу.
import { useEffect, useRef, type ReactNode } from "react";
import { Box, keyframes } from "@mui/material";
import mascotImg from "@/shared/assets/maskot2.png";
import type { MascotMood } from "../model/types";

const ASPECT = 480 / 526;
const MAX_TILT = 14; // градусов
const REACH = 420; // на каком расстоянии от маскота курсор даёт полный наклон, px

const breathe = keyframes`0%,100%{transform:translateY(0) scale(1)}50%{transform:translateY(-3%) scale(1.012)}`;
const blink = keyframes`0%,92%,100%{transform:scaleY(0)}95%{transform:scaleY(1)}`;
const nod = keyframes`0%,100%{transform:rotateX(0) translateY(0)}50%{transform:rotateX(14deg) translateY(3%)}`;
const sleepTilt = keyframes`0%,100%{transform:rotateZ(-7deg) translateY(4%)}50%{transform:rotateZ(-7deg) translateY(6%)}`;
const wake = keyframes`0%{transform:scale(1,1)}35%{transform:scale(.94,1.1) translateY(-6%)}70%{transform:scale(1.04,.97)}100%{transform:scale(1,1)}`;
const wave = keyframes`0%,100%{transform:rotateZ(0)}20%{transform:rotateZ(-8deg)}40%{transform:rotateZ(6deg)}60%{transform:rotateZ(-6deg)}80%{transform:rotateZ(4deg)}`;
const think = keyframes`0%,100%{transform:rotateX(-10deg) translateY(-2%)}50%{transform:rotateX(-14deg) translateY(-4%)}`;
const hop = keyframes`0%,100%{transform:translateY(0) scale(1)}30%{transform:translateY(-18%) scale(1.04,.98)}55%{transform:translateY(0) scale(1.06,.92)}75%{transform:translateY(-5%)}`;
const shake = keyframes`0%,100%{transform:translateX(0) rotateY(0)}20%{transform:translateX(-5%) rotateY(-18deg)}40%{transform:translateX(5%) rotateY(18deg)}60%{transform:translateX(-3%) rotateY(-10deg)}80%{transform:translateX(3%) rotateY(10deg)}`;
const nudge = keyframes`0%,100%{transform:rotateZ(0)}25%{transform:rotateZ(-4deg)}75%{transform:rotateZ(4deg)}`;
const zzz = keyframes`0%{opacity:0;transform:translate(0,0) scale(.6)}30%{opacity:1}100%{opacity:0;transform:translate(60%,-120%) scale(1.15)}`;
const dots = keyframes`0%,20%{opacity:.2}50%{opacity:1}100%{opacity:.2}`;

/** Что делает тело в каждом состоянии. */
const MOTION: Partial<Record<MascotMood, string>> = {
  idle: `${breathe} 5s ease-in-out infinite`,
  drowsy: `${nod} 4s ease-in-out infinite`,
  sleeping: `${sleepTilt} 7s ease-in-out infinite`,
  waking: `${wake} .8s ease-out 1`,
  greeting: `${wave} 1.4s ease-in-out 1`,
  thinking: `${think} 2.4s ease-in-out infinite`,
  has_answer: `${hop} .9s cubic-bezier(.3,.7,.4,1) 1`,
  reminder: `${nudge} .6s ease-in-out 2`,
  needs_confirm: `${breathe} 5s ease-in-out infinite`,
  error: `${shake} .7s ease-in-out 1`,
};

/** Насколько закрыты веки: 0 — открыты, 1 — закрыты (ниже — кадр, а не анимация: виден и без движения). */
const LIDS: Partial<Record<MascotMood, number>> = { drowsy: 0.55, sleeping: 1 };

// Глаза на исходнике 526×480: веки — овалы цвета меха, опускаются сверху.
const EYES = [
  { left: "33%", top: "17.5%", width: "22.5%", height: "23%", rotate: "8deg" },
  { left: "55.5%", top: "25.5%", width: "22%", height: "22.5%", rotate: "8deg" },
];

export const Mascot3D = ({ size, mood = "idle", interactive = true, badge }: {
  size: number; mood?: MascotMood; interactive?: boolean; badge?: ReactNode;
}) => {
  const root = useRef<HTMLDivElement>(null);
  const w = size * 1.2;
  const h = w * ASPECT;

  // Наклон к курсору: CSS-переменные через requestAnimationFrame, без перерисовки React.
  useEffect(() => {
    const el = root.current;
    if (!el || !interactive || window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;
    let frame = 0;
    const onMove = (e: PointerEvent) => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const r = el.getBoundingClientRect();
        const dx = Math.max(-1, Math.min(1, (e.clientX - (r.left + r.width / 2)) / REACH));
        const dy = Math.max(-1, Math.min(1, (e.clientY - (r.top + r.height / 2)) / REACH));
        el.style.setProperty("--ry", `${(dx * MAX_TILT).toFixed(2)}deg`);
        el.style.setProperty("--rx", `${(-dy * MAX_TILT).toFixed(2)}deg`);
        el.style.setProperty("--gx", `${50 + dx * 30}%`);
        el.style.setProperty("--gy", `${40 + dy * 30}%`);
      });
    };
    const reset = () => { el.style.setProperty("--rx", "0deg"); el.style.setProperty("--ry", "0deg"); };
    window.addEventListener("pointermove", onMove, { passive: true });
    document.addEventListener("pointerleave", reset);
    return () => { cancelAnimationFrame(frame); window.removeEventListener("pointermove", onMove); document.removeEventListener("pointerleave", reset); };
  }, [interactive]);

  const lids = LIDS[mood] ?? 0;
  return (
    <Box ref={root} data-mood={mood} aria-hidden={!badge}
      sx={{
        position: "relative", width: w, height: h, flex: "none", perspective: `${Math.round(w * 4)}px`,
        "--rx": "0deg", "--ry": "0deg", "--gx": "50%", "--gy": "40%",
        "@media (prefers-reduced-motion: reduce)": { "& *": { animation: "none !important", transition: "none !important" } },
      }}>
      {/* Тень на «полу»: сжимается, когда маскот наклоняется или подпрыгивает */}
      <Box aria-hidden sx={{
        position: "absolute", left: "18%", right: "18%", bottom: "-4%", height: "10%", borderRadius: "50%",
        background: "radial-gradient(closest-side, rgba(0,40,120,.32), transparent)",
        transform: "scaleX(calc(1 - (var(--ry) / 1deg) * (var(--ry) / 1deg) / 900))", transition: "transform .3s ease-out",
      }} />
      {/* Наклон в перспективе */}
      <Box sx={{
        width: "100%", height: "100%", transformStyle: "preserve-3d",
        transform: "rotateX(var(--rx)) rotateY(var(--ry))", transition: "transform .35s cubic-bezier(.2,.8,.3,1)",
        "&:active": { transform: "rotateX(var(--rx)) rotateY(var(--ry)) scale(.95)" },
      }}>
        {/* Движение состояния */}
        <Box sx={{ position: "relative", width: "100%", height: "100%", transformOrigin: "50% 90%", animation: MOTION[mood],
          filter: mood === "sleeping" ? "saturate(.8) brightness(.95)" : "drop-shadow(0 10px 18px rgba(0, 80, 180, .28))" }}>
          <Box component="img" src={mascotImg} alt="" draggable={false}
            sx={{ width: "100%", height: "100%", display: "block", objectFit: "contain", maxWidth: "none", userSelect: "none", transform: "translateZ(20px)" }} />
          {/* Веки: моргание раз в несколько секунд; полуприкрыты — дремлет, закрыты — спит */}
          {EYES.map((e, i) => (
            <Box key={i} aria-hidden data-lid={lids} sx={{
              position: "absolute", left: e.left, top: e.top, width: e.width, height: e.height, transform: `translateZ(21px) rotate(${e.rotate})`,
              borderRadius: "50%", overflow: "hidden", pointerEvents: "none",
            }}>
              {/* Веко цвета меха с мягким краем; нижняя кромка — тёмная линия ресниц («закрытый глаз») */}
              <Box sx={{
                position: "absolute", inset: "-2%", transformOrigin: "50% 0", borderRadius: "0 0 50% 50% / 0 0 38% 38%",
                background: "radial-gradient(90% 80% at 50% 35%, #2f68e2 0%, #2459d2 60%, #183f9f 100%)",
                boxShadow: "inset 0 0 6px rgba(10,30,90,.55)",
                transform: lids ? `scaleY(${lids})` : "scaleY(0)", transition: "transform .5s ease-in-out",
                animation: lids ? undefined : `${blink} ${i ? 6.3 : 6}s ease-in-out infinite`,
              }} />
              {/* Спит: глаз закрыт — дуга ресниц «‿» и лёгкая тень, а не гладкое пятно */}
              {lids === 1 && <Box sx={{
                position: "absolute", left: "16%", right: "16%", top: "34%", height: "32%",
                borderBottom: `${Math.max(2, Math.round(size / 30))}px solid rgba(8, 24, 78, .85)`, borderRadius: "0 0 50% 50%",
              }} />}
            </Box>
          ))}
          {/* Блик: свет «падает» с той стороны, куда смотрит курсор */}
          <Box aria-hidden sx={{
            position: "absolute", inset: "4%", borderRadius: "50%", pointerEvents: "none", mixBlendMode: "soft-light",
            background: "radial-gradient(circle at var(--gx) var(--gy), rgba(255,255,255,.55), transparent 55%)", transform: "translateZ(22px)",
          }} />
        </Box>
      </Box>
      {mood === "sleeping" && ["z", "z", "Z"].map((z, i) => (
        <Box key={i} component="span" aria-hidden sx={{
          position: "absolute", right: `${8 - i * 4}%`, top: `${10 - i * 6}%`, fontWeight: 700, color: "#3a74ef",
          fontSize: Math.max(10, Math.round(size * (0.14 + i * 0.04))), animation: `${zzz} 3s ease-out ${i}s infinite`,
        }}>{z}</Box>
      ))}
      {mood === "thinking" && (
        <Box component="span" aria-hidden sx={{
          position: "absolute", right: "-4%", top: "-6%", px: 0.75, py: 0.25, borderRadius: 2, bgcolor: "background.paper",
          boxShadow: "0 2px 8px rgba(0,0,0,.18)", fontWeight: 700, fontSize: Math.max(10, Math.round(size * 0.16)), lineHeight: 1.2,
        }}>
          {[0, 1, 2].map((i) => <Box key={i} component="span" sx={{ animation: `${dots} 1.2s ${i * 0.2}s infinite` }}>.</Box>)}
        </Box>
      )}
      {badge}
    </Box>
  );
};
