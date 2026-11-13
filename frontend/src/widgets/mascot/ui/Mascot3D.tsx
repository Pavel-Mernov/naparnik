// Маскот, который двигается сам (08-DESIGN §2): рендер разрезан на тело и поднятую лапу
// (scripts/mascot_layers.py), лапа вращается вокруг запястья — машет, держит письмо, опускается во сне.
// Поверх тела — веки (моргание, дремота, сон) и наклон всей фигуры в перспективе к курсору.
//   idle — дышит, моргает, лапа чуть покачивается;  greeting — машет «привет»;
//   has_answer / needs_confirm / reminder — машет письмом и часто моргает, пока событие не открыто;
//   thinking — постукивает лапой, облачко «…»;  error — разводит лапой и качает головой;
//   drowsy — веки опускаются, лапа никнет, кивает;  sleeping — глаза закрыты, лапа опущена, храпит: грудь
//   ходит вдох-выдох и на каждом выдохе вылетают «z z Z»;  waking — потягивается, лапа вверх.
// prefers-reduced-motion → статичные кадры: поза лапы, веки, письмо и «Z» видны без движения.
import { useEffect, useRef, type ReactNode } from "react";
import { Box, keyframes } from "@mui/material";
import bodyImg from "@/shared/assets/mascot/body.png";
import pawImg from "@/shared/assets/mascot/paw.png";
import type { MascotMood } from "../model/types";

const ASPECT = 480 / 526;
const MAX_TILT = 14; // градусов
const REACH = 420; // на каком расстоянии от маскота курсор даёт полный наклон, px
// Где лапа на исходнике 526×480 (вывод scripts/mascot_layers.py: paw crop 2 207 128 327) и где запястье.
const PAW = { left: 2 / 526, top: 207 / 480, width: 126 / 526, height: 120 / 480, pivot: "55% 95%" };

// ---------- тело ----------
const breathe = keyframes`0%,100%{transform:translateY(0) scale(1)}50%{transform:translateY(-2.5%) scale(1.012)}`;
const nod = keyframes`0%,100%{transform:rotateX(0) translateY(0)}50%{transform:rotateX(14deg) translateY(3%)}`;
// Храп: долгий вдох (грудь поднимается), короткий шумный выдох с подрагиванием.
const snore = keyframes`0%{transform:rotateZ(-7deg) translateY(5%) scale(1,1)}55%{transform:rotateZ(-7deg) translateY(3%) scale(1.035,1.045)}62%{transform:rotateZ(-6deg) translateY(4%) scale(1.02,1.02)}66%{transform:rotateZ(-7.5deg) translateY(4.5%) scale(1.025,1.015)}100%{transform:rotateZ(-7deg) translateY(5%) scale(1,1)}`;
const wake = keyframes`0%{transform:scale(1,1)}35%{transform:scale(.94,1.1) translateY(-6%)}70%{transform:scale(1.04,.97)}100%{transform:scale(1,1)}`;
const think = keyframes`0%,100%{transform:rotateX(-10deg) translateY(-2%)}50%{transform:rotateX(-14deg) translateY(-4%)}`;
const hop = keyframes`0%,100%{transform:translateY(0)}12%{transform:translateY(-10%) scale(1.03,.98)}24%{transform:translateY(0) scale(1.04,.95)}32%,100%{transform:translateY(0) scale(1)}`;
const shake = keyframes`0%,100%{transform:translateX(0) rotateY(0)}20%{transform:translateX(-5%) rotateY(-18deg)}40%{transform:translateX(5%) rotateY(18deg)}60%{transform:translateX(-3%) rotateY(-10deg)}80%{transform:translateX(3%) rotateY(10deg)}`;

// ---------- лапа (вращение вокруг запястья) ----------
const pawIdle = keyframes`0%,100%{transform:rotate(0)}50%{transform:rotate(-3deg)}`;
const pawHello = keyframes`0%,100%{transform:rotate(0)}15%{transform:rotate(-22deg)}30%{transform:rotate(14deg)}45%{transform:rotate(-22deg)}60%{transform:rotate(14deg)}75%{transform:rotate(-12deg)}`;
// Машет письмом: серия взмахов, пауза с поднятым письмом, снова взмахи — пока событие не открыто.
const pawLetter = keyframes`0%{transform:rotate(0)}6%{transform:rotate(-24deg)}12%{transform:rotate(16deg)}18%{transform:rotate(-24deg)}24%{transform:rotate(16deg)}30%{transform:rotate(-20deg)}36%{transform:rotate(10deg)}42%,100%{transform:rotate(-6deg) translateY(-4%)}`;
const pawTap = keyframes`0%,100%{transform:rotate(-8deg)}50%{transform:rotate(4deg)}`;
const pawShrug = keyframes`0%,100%{transform:rotate(0)}30%,70%{transform:rotate(-34deg) translateX(-6%)}`;
const pawStretch = keyframes`0%{transform:rotate(-40deg) translateY(20%)}60%{transform:rotate(8deg) translateY(-10%)}100%{transform:rotate(0)}`;

// ---------- лицо ----------
const blink = keyframes`0%,92%,100%{transform:scaleY(0)}95%{transform:scaleY(1)}`;
// Двойное моргание — «эй, тут для тебя кое-что есть».
const blinkTwice = keyframes`0%,80%,88%,100%{transform:scaleY(0)}84%,92%{transform:scaleY(1)}`;
const zzz = keyframes`0%{opacity:0;transform:translate(0,0) scale(.6) rotate(-8deg)}20%{opacity:1}100%{opacity:0;transform:translate(70%,-140%) scale(1.25) rotate(10deg)}`;
const dots = keyframes`0%,20%{opacity:.2}50%{opacity:1}100%{opacity:.2}`;
const letterWiggle = keyframes`0%,100%{transform:rotate(14deg)}50%{transform:rotate(4deg)}`;

const LETTER: MascotMood[] = ["has_answer", "needs_confirm", "reminder"];

const BODY: Partial<Record<MascotMood, string>> = {
  idle: `${breathe} 5s ease-in-out infinite`,
  greeting: `${breathe} 5s ease-in-out infinite`,
  drowsy: `${nod} 4s ease-in-out infinite`,
  sleeping: `${snore} 4.2s ease-in-out infinite`,
  waking: `${wake} .8s ease-out 1`,
  thinking: `${think} 2.4s ease-in-out infinite`,
  has_answer: `${hop} 5s ease-out infinite`,
  needs_confirm: `${breathe} 5s ease-in-out infinite`,
  reminder: `${hop} 5s ease-out infinite`,
  error: `${shake} .7s ease-in-out 1`,
};
const PAW_MOTION: Partial<Record<MascotMood, string>> = {
  idle: `${pawIdle} 5s ease-in-out infinite`,
  greeting: `${pawHello} 1.6s ease-in-out 1`,
  has_answer: `${pawLetter} 5s ease-in-out infinite`,
  needs_confirm: `${pawLetter} 6s ease-in-out infinite`,
  reminder: `${pawLetter} 5s ease-in-out infinite`,
  thinking: `${pawTap} .9s ease-in-out infinite`,
  error: `${pawShrug} 1.2s ease-in-out 1`,
  waking: `${pawStretch} .9s ease-out 1`,
};
/** Поза лапы без анимации: никнет в дремоте, опущена во сне (кадр виден и при reduced motion). */
const PAW_POSE: Partial<Record<MascotMood, string>> = {
  drowsy: "rotate(-22deg) translateY(8%)",
  sleeping: "rotate(-58deg) translate(-6%, 30%)",
};
/** Насколько закрыты веки: 0 — открыты, 1 — закрыты. */
const LIDS: Partial<Record<MascotMood, number>> = { drowsy: 0.55, sleeping: 1 };

// Глаза на исходнике 526×480: веки — овалы цвета меха, опускаются сверху.
const EYES = [
  { left: "33%", top: "17.5%", width: "22.5%", height: "23%", rotate: "8deg" },
  { left: "55.5%", top: "25.5%", width: "22%", height: "22.5%", rotate: "8deg" },
];

/** Письмо в лапе: конверт с печатью; у напоминания — восклицательный знак. */
const Letter = ({ alert }: { alert: boolean }) => (
  <Box component="svg" viewBox="0 0 64 46" aria-hidden data-letter={alert ? "alert" : "letter"} sx={{
    position: "absolute", left: "38%", top: "-30%", width: "86%", transformOrigin: "20% 90%",
    animation: `${letterWiggle} 1.4s ease-in-out infinite`, filter: "drop-shadow(0 3px 4px rgba(0,30,90,.35))",
  }}>
    <rect x="2" y="4" width="60" height="40" rx="5" fill="#fff" stroke="#c8d6ee" strokeWidth="2" />
    <path d="M4 8 L32 28 L60 8" fill="none" stroke="#9fb3d6" strokeWidth="3" strokeLinejoin="round" />
    <circle cx="32" cy="28" r="7" fill={alert ? "#ff4053" : "#2a62de"} />
    {alert && <path d="M32 23v6M32 32v.5" stroke="#fff" strokeWidth="2.6" strokeLinecap="round" />}
  </Box>
);

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
  const letter = LETTER.includes(mood);
  const blinkAnim = letter ? `${blinkTwice} 2.6s ease-in-out infinite` : `${blink} 6s ease-in-out infinite`;
  return (
    <Box ref={root} data-mood={mood} aria-hidden={!badge}
      sx={{
        position: "relative", width: w, height: h, flex: "none", perspective: `${Math.round(w * 4)}px`,
        "--rx": "0deg", "--ry": "0deg", "--gx": "50%", "--gy": "40%",
        "@media (prefers-reduced-motion: reduce)": { "& *": { animation: "none !important", transition: "none !important" } },
      }}>
      {/* Тень на «полу»: сжимается, когда маскот наклоняется */}
      <Box aria-hidden sx={{
        position: "absolute", left: "18%", right: "18%", bottom: "-4%", height: "10%", borderRadius: "50%",
        background: "radial-gradient(closest-side, rgba(0,40,120,.32), transparent)",
        transform: "scaleX(calc(1 - (var(--ry) / 1deg) * (var(--ry) / 1deg) / 900))", transition: "transform .3s ease-out",
      }} />
      {/* Наклон в перспективе к курсору */}
      <Box sx={{
        width: "100%", height: "100%", transformStyle: "preserve-3d",
        transform: "rotateX(var(--rx)) rotateY(var(--ry))", transition: "transform .35s cubic-bezier(.2,.8,.3,1)",
        "&:active": { transform: "rotateX(var(--rx)) rotateY(var(--ry)) scale(.95)" },
      }}>
        {/* Тело со всем, что на нём: двигается по состоянию */}
        <Box sx={{ position: "relative", width: "100%", height: "100%", transformOrigin: "50% 90%", animation: BODY[mood],
          filter: mood === "sleeping" ? "saturate(.85) brightness(.96)" : "drop-shadow(0 10px 18px rgba(0, 80, 180, .28))" }}>
          <Box component="img" src={bodyImg} alt="" draggable={false}
            sx={{ width: "100%", height: "100%", display: "block", objectFit: "contain", maxWidth: "none", userSelect: "none", transform: "translateZ(20px)" }} />
          {/* Веки: моргание; двойное — когда есть письмо; полуприкрыты — дремлет; закрыты — спит */}
          {EYES.map((e, i) => (
            <Box key={i} aria-hidden data-lid={lids} sx={{
              position: "absolute", left: e.left, top: e.top, width: e.width, height: e.height, transform: `translateZ(21px) rotate(${e.rotate})`,
              borderRadius: "50%", overflow: "hidden", pointerEvents: "none",
            }}>
              <Box sx={{
                position: "absolute", inset: "-2%", transformOrigin: "50% 0", borderRadius: "0 0 50% 50% / 0 0 38% 38%",
                background: "radial-gradient(90% 80% at 50% 35%, #2f68e2 0%, #2459d2 60%, #183f9f 100%)",
                boxShadow: "inset 0 0 6px rgba(10,30,90,.55)",
                transform: lids ? `scaleY(${lids})` : "scaleY(0)", transition: "transform .5s ease-in-out",
                animation: lids ? undefined : blinkAnim, animationDelay: lids ? undefined : `${i * 0.04}s`,
              }} />
              {/* Спит: глаз закрыт — дуга ресниц «‿» */}
              {lids === 1 && <Box sx={{
                position: "absolute", left: "16%", right: "16%", top: "34%", height: "32%",
                borderBottom: `${Math.max(2, Math.round(size / 30))}px solid rgba(8, 24, 78, .85)`, borderRadius: "0 0 50% 50%",
              }} />}
            </Box>
          ))}
          {/* Лапа: отдельный слой, вращается вокруг запястья */}
          <Box data-paw={mood} sx={{
            position: "absolute", left: `${PAW.left * 100}%`, top: `${PAW.top * 100}%`, width: `${PAW.width * 100}%`, height: `${PAW.height * 100}%`,
            transformOrigin: PAW.pivot, transform: `translateZ(24px) ${PAW_POSE[mood] ?? ""}`, transition: "transform .9s ease-in-out",
          }}>
            <Box sx={{ position: "relative", width: "100%", height: "100%", transformOrigin: PAW.pivot, animation: PAW_MOTION[mood] }}>
              <Box component="img" src={pawImg} alt="" draggable={false} sx={{ width: "100%", height: "100%", display: "block", userSelect: "none" }} />
              {letter && <Letter alert={mood === "reminder"} />}
            </Box>
          </Box>
          {/* Блик: свет «падает» с той стороны, куда смотрит курсор */}
          <Box aria-hidden sx={{
            position: "absolute", inset: "4%", borderRadius: "50%", pointerEvents: "none", mixBlendMode: "soft-light",
            background: "radial-gradient(circle at var(--gx) var(--gy), rgba(255,255,255,.55), transparent 55%)", transform: "translateZ(26px)",
          }} />
        </Box>
      </Box>
      {/* Храп: на каждом выдохе вылетают «z z Z», ритм совпадает с дыханием */}
      {mood === "sleeping" && ["z", "z", "Z"].map((z, i) => (
        <Box key={i} component="span" aria-hidden sx={{
          position: "absolute", right: `${10 - i * 6}%`, top: `${14 - i * 9}%`, fontWeight: 800, color: "#3a74ef", fontFamily: "inherit",
          fontSize: Math.max(10, Math.round(size * (0.16 + i * 0.06))), textShadow: "0 1px 0 #fff",
          animation: `${zzz} 4.2s ease-out ${2.3 + i * 0.35}s infinite`, opacity: 0,
          "@media (prefers-reduced-motion: reduce)": { opacity: 1 },
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
