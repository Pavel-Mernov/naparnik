// Живой маскот (08-DESIGN §2). Рендер разрезан на слои (scripts/mascot_layers.py): персонаж без фона, отдельная
// поднятая лапа; одеяло/подушка маскируются эллипсом — поэтому персонаж может лечь «внутри» кадра.
//   idle — дышит, моргает, лапа чуть покачивается;  greeting — машет «привет»;
//   has_answer / needs_confirm / reminder — машет письмом и дважды моргает, пока событие не открыто;
//   thinking — постукивает лапой, облачко «…»;  error — разводит лапой и качает головой;
//   drowsy — веки тяжелеют, лапа никнет, клюёт носом;
//   sleeping — ложится на бок на подушку, одеяло наползает сверху, глаза закрыты, храпит: грудь ходит
//     вдох-выдох, на выдохе от головы вылетают «z z Z»;
//   waking — одеяло сползает, садится и потягивается.
// prefers-reduced-motion → без движения, но поза видна: лежит под одеялом, веки, письмо.
import { type ReactNode } from "react";
import { Box, keyframes } from "@mui/material";
import figureImg from "@/shared/assets/mascot/figure.png";
import pawImg from "@/shared/assets/mascot/paw.png";
import eyesClosedImg from "@/shared/assets/mascot/eyes-closed.png";
import eyesHalfImg from "@/shared/assets/mascot/eyes-half.png";
import type { MascotMood } from "../model/types";

const ASPECT = 480 / 526;
// Геометрия исходника 526×480 (scripts/mascot_layers.py): лапа и её запястье, круг-подложка.
const PAW = { left: 2 / 526, top: 207 / 480, width: 126 / 526, height: 120 / 480, pivot: "55% 95%" };
const IN_CIRCLE = "ellipse(41.2% 45.15% at 52.95% 55.63%)"; // маска одеяла и подушки (бывший круг-подложка)
// Поза «лёг спать»: центр фигуры смещается к подушке, фигура поворачивается на бок и чуть уменьшается.
const LYING = "translate(-1%, 11.5%) rotate(-66deg) scale(.8)";

// ---------- тело ----------
const breathe = keyframes`0%,100%{transform:translateY(0) scale(1)}50%{transform:translateY(-2.5%) scale(1.012)}`;
const nod = keyframes`0%,100%{transform:rotate(0) translateY(0)}45%{transform:rotate(3deg) translateY(3%)}55%{transform:rotate(3deg) translateY(3.5%)}70%{transform:rotate(-1deg) translateY(0)}`;
// Храп лёжа: долгий вдох (грудь раздувается), короткий выдох с подрагиванием.
const snore = keyframes`0%{transform:scale(1,1)}55%{transform:scale(1.035,1.05)}62%{transform:scale(1.015,1.02)}66%{transform:scale(1.022,1.03)}100%{transform:scale(1,1)}`;
const wake = keyframes`0%{transform:scale(1,1)}35%{transform:scale(.94,1.1) translateY(-6%)}70%{transform:scale(1.04,.97)}100%{transform:scale(1,1)}`;
const think = keyframes`0%,100%{transform:rotate(-2deg) translateY(-2%)}50%{transform:rotate(-4deg) translateY(-4%)}`;
const hop = keyframes`0%,100%{transform:translateY(0)}12%{transform:translateY(-10%) scale(1.03,.98)}24%{transform:translateY(0) scale(1.04,.95)}32%,100%{transform:translateY(0) scale(1)}`;
const shake = keyframes`0%,100%{transform:translateX(0) rotate(0)}20%{transform:translateX(-5%) rotate(-5deg)}40%{transform:translateX(5%) rotate(5deg)}60%{transform:translateX(-3%) rotate(-3deg)}80%{transform:translateX(3%) rotate(3deg)}`;
// Бейдж: только вверх-вниз, целые px + translate3d — без субпиксельного дрожания текста.
const badgeBob = keyframes`
  0%,100%{transform:translate3d(0,0,0)}
  50%{transform:translate3d(0,-4px,0)}
`;

// ---------- лапа (вращение вокруг запястья) ----------
const pawIdle = keyframes`0%,100%{transform:rotate(0)}50%{transform:rotate(-3deg)}`;
const pawHello = keyframes`0%,100%{transform:rotate(0)}15%{transform:rotate(-22deg)}30%{transform:rotate(14deg)}45%{transform:rotate(-22deg)}60%{transform:rotate(14deg)}75%{transform:rotate(-12deg)}`;
// Машет письмом: серия взмахов, пауза с поднятым письмом, снова взмахи — пока событие не открыто.
const pawLetter = keyframes`0%{transform:rotate(0)}6%{transform:rotate(-24deg)}12%{transform:rotate(16deg)}18%{transform:rotate(-24deg)}24%{transform:rotate(16deg)}30%{transform:rotate(-20deg)}36%{transform:rotate(10deg)}42%,100%{transform:rotate(-6deg) translateY(-4%)}`;
const pawTap = keyframes`0%,100%{transform:rotate(-8deg)}50%{transform:rotate(4deg)}`;
const pawShrug = keyframes`0%,100%{transform:rotate(0)}30%,70%{transform:rotate(-34deg) translateX(-6%)}`;
const pawStretch = keyframes`0%{transform:rotate(-40deg) translateY(20%)}60%{transform:rotate(8deg) translateY(-10%)}100%{transform:rotate(0)}`;

// ---------- лицо и прочее ----------
// Моргание — на мгновение показываем закрытые глаза; двойное — «эй, тут для тебя кое-что есть».
const blink = keyframes`0%,93%,100%{opacity:0}94.5%,97%{opacity:1}`;
const blinkTwice = keyframes`0%,78%,100%{opacity:0}79.5%,82%{opacity:1}84%{opacity:0}86%,88.5%{opacity:1}90.5%{opacity:0}`;
const zzz = keyframes`0%{opacity:0;transform:translate(0,0) scale(.5) rotate(-10deg)}15%{opacity:1}100%{opacity:0;transform:translate(90%,-170%) scale(1.3) rotate(12deg)}`;
const dots = keyframes`0%,20%{opacity:.2}50%{opacity:1}100%{opacity:.2}`;
const letterWiggle = keyframes`0%,100%{transform:rotate(14deg)}50%{transform:rotate(4deg)}`;
// Одеяло дышит вместе с ним — край чуть поднимается на вдохе.
const blanketBreath = keyframes`0%,100%{transform:translateY(0) rotate(-13deg)}55%{transform:translateY(-2.5%) rotate(-13deg)}`;

const LETTER: MascotMood[] = ["has_answer", "needs_confirm", "reminder"];

const BODY: Partial<Record<MascotMood, string>> = {
  idle: `${breathe} 5s ease-in-out infinite`,
  greeting: `${breathe} 5s ease-in-out infinite`,
  drowsy: `${nod} 3.6s ease-in-out infinite`,
  sleeping: `${snore} 4.2s ease-in-out 1.6s infinite`,
  waking: `${wake} .8s ease-out .5s 1`,
  thinking: `${think} 2.4s ease-in-out infinite`,
  has_answer: `${hop} 5s ease-out infinite`,
  needs_confirm: `${breathe} 5s ease-in-out infinite`,
  reminder: `${hop} 5s ease-out infinite`,
  error: `${shake} .7s ease-in-out 1`,
};
/** Одна плавная качка для всех настроений — резкие hop/nod давали дрожь цифры. */
const BADGE_BOB = `${badgeBob} 5s cubic-bezier(0.45, 0, 0.55, 1) infinite`;
const PAW_MOTION: Partial<Record<MascotMood, string>> = {
  idle: `${pawIdle} 5s ease-in-out infinite`,
  greeting: `${pawHello} 1.6s ease-in-out 1`,
  has_answer: `${pawLetter} 5s ease-in-out infinite`,
  needs_confirm: `${pawLetter} 6s ease-in-out infinite`,
  reminder: `${pawLetter} 5s ease-in-out infinite`,
  thinking: `${pawTap} .9s ease-in-out infinite`,
  error: `${pawShrug} 1.2s ease-in-out 1`,
  waking: `${pawStretch} .9s ease-out .5s 1`,
};
/** Поза лапы: никнет в дремоте; во сне — под одеялом. */
const PAW_POSE: Partial<Record<MascotMood, string>> = { drowsy: "rotate(-22deg) translateY(8%)", sleeping: "rotate(-40deg) translateY(25%)" };
/** Глаза: открыты, полуприкрыты (дремлет), закрыты (спит). Слои из scripts/mascot_layers.py точно по фигуре. */
const EYES: Partial<Record<MascotMood, "half" | "closed">> = { drowsy: "half", sleeping: "closed" };

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

export const Mascot3D = ({ size, mood = "idle", badge, mail }: {
  size: number; mood?: MascotMood; badge?: ReactNode;
  /** Письмо в лапе, пока есть уведомление (бейдж) — даже если маскот уже idle/drowsy. */
  mail?: boolean;
}) => {
  const w = size * 1.2;
  const h = w * ASPECT;

  const asleep = mood === "sleeping";
  const eyes = EYES[mood] ?? "open";
  // Письмо: по mood события ИЛИ пока висит бейдж уведомлений (mail).
  const letter = !asleep && (Boolean(mail) || LETTER.includes(mood));
  const blinkAnim = letter ? `${blinkTwice} 2.6s ease-in-out infinite` : `${blink} 6s ease-in-out infinite`;
  const pawMotion = letter
    ? (LETTER.includes(mood) ? PAW_MOTION[mood] : PAW_MOTION.has_answer)
    : PAW_MOTION[mood];
  const stripe = Math.max(3, Math.round(size * 0.07));
  // На мелком размере (мобилка) filter даёт «мыло» — на десктопе оставляем тень.
  const compact = size < 90;
  return (
    <Box data-mood={mood} aria-hidden={!badge}
      sx={{
        position: "relative", width: w, height: h, flex: "none",
        "@media (prefers-reduced-motion: reduce)": { "& *": { animation: "none !important", transition: "none !important" } },
      }}>
      <Box sx={{ position: "relative", width: "100%", height: "100%" }}>
        {/* Подушка: появляется под головой, когда ложится (маска — бывший круг-подложка) */}
        <Box aria-hidden data-pillow={asleep} sx={{ position: "absolute", inset: 0, clipPath: IN_CIRCLE, pointerEvents: "none" }}>
          <Box sx={{
            position: "absolute", left: "4%", top: "54%", width: "44%", height: "24%", borderRadius: "48% 52% 46% 54% / 55% 55% 45% 45%",
            background: "radial-gradient(70% 70% at 45% 35%, #ffffff 0%, #f1f6ff 60%, #d9e6f8 100%)",
            border: `${Math.max(1, Math.round(size / 48))}px solid #9db6dc`,
            boxShadow: "inset 0 -4px 8px rgba(40,80,150,.22), 0 3px 6px rgba(0,40,120,.18)",
            // шов посередине подушки
            "&::after": { content: '""', position: "absolute", left: "12%", right: "12%", top: "48%", borderTop: "1px dashed #a9c0e2" },
            opacity: asleep ? 1 : 0, transform: asleep ? "scale(1)" : "scale(.6) translateY(30%)", transition: "opacity .6s ease .5s, transform .6s ease .5s",
          }} />
        </Box>
        {/* Персонаж: ложится на бок и встаёт — одним плавным движением */}
        <Box sx={{
          position: "absolute", inset: 0, transformOrigin: "50% 50%",
          transform: asleep ? LYING : undefined,
          transition: asleep ? "transform 1.5s cubic-bezier(.55,.05,.35,1.15)" : "transform .8s cubic-bezier(.3,1.3,.5,1)",
        }}>
          <Box sx={{ position: "relative", width: "100%", height: "100%", transformOrigin: "50% 75%", animation: BODY[mood] }}>
            <Box sx={{
              position: "relative", width: "100%", height: "100%",
              filter: asleep
                ? (compact ? "none" : "saturate(.9) brightness(.97)")
                : (compact ? "none" : "drop-shadow(0 6px 10px rgba(0, 60, 160, .25))"),
            }}>
              <Box component="img" src={figureImg} alt="" draggable={false}
                sx={{ width: "100%", height: "100%", display: "block", objectFit: "contain", maxWidth: "none", userSelect: "none" }} />
              {/* Глаза: мех брови опускается на глаз; моргает, тяжелеют в дремоте, закрыты во сне */}
              <Box component="img" src={eyesHalfImg} alt="" aria-hidden data-eyes-half={eyes === "half"} draggable={false} sx={{
                position: "absolute", inset: 0, width: "100%", height: "100%", pointerEvents: "none",
                opacity: eyes === "half" ? 1 : 0, transition: "opacity .7s ease-in-out",
              }} />
              <Box component="img" src={eyesClosedImg} alt="" aria-hidden data-eyes={eyes} draggable={false} sx={{
                position: "absolute", inset: 0, width: "100%", height: "100%", pointerEvents: "none",
                opacity: eyes === "closed" ? 1 : 0, transition: "opacity .7s ease-in-out",
                animation: eyes === "open" ? blinkAnim : undefined,
              }} />
              {/* Лапа: во сне прячется под одеялом с задержкой; при пробуждении сразу появляется — иначе рука «пропадает» на весь waking */}
              <Box data-paw={mood} sx={{
                position: "absolute", left: `${PAW.left * 100}%`, top: `${PAW.top * 100}%`, width: `${PAW.width * 100}%`, height: `${PAW.height * 100}%`,
                transformOrigin: PAW.pivot,
                transform: PAW_POSE[mood] ?? "none",
                opacity: asleep ? 0 : 1,
                transition: asleep
                  ? "transform .9s ease-in-out, opacity .35s ease .85s"
                  : "transform .55s ease-out, opacity .2s ease",
              }}>
                <Box sx={{ position: "relative", width: "100%", height: "100%", transformOrigin: PAW.pivot, animation: pawMotion }}>
                  <Box component="img" src={pawImg} alt="" draggable={false} sx={{ width: "100%", height: "100%", display: "block", userSelect: "none" }} />
                  {letter && <Letter alert={mood === "reminder" || Boolean(mail && mood !== "has_answer" && mood !== "needs_confirm")} />}
                </Box>
              </Box>
            </Box>
          </Box>
        </Box>
        {/* Одеяло: наползает, когда ложится, сползает, когда встаёт; на вдохе край приподнимается */}
        <Box aria-hidden data-blanket={asleep} sx={{ position: "absolute", inset: 0, clipPath: IN_CIRCLE, pointerEvents: "none" }}>
          <Box sx={{
            position: "absolute", left: "27%", top: "70%", width: "90%", height: "70%", transformOrigin: "0 0",
            borderRadius: `${Math.round(size * 0.12)}px ${Math.round(size * 0.06)}px 0 0`,
            background: `repeating-linear-gradient(115deg, #ffc178 0 ${stripe}px, #ffdcaa ${stripe}px ${stripe * 1.6}px)`,
            borderTop: `${Math.max(3, Math.round(size * 0.06))}px solid #ffecd2`, boxShadow: "0 -2px 6px rgba(120,60,0,.18)",
            transform: asleep ? "translateY(0) rotate(-13deg)" : "translateY(60%) rotate(-13deg)",
            transition: asleep ? "transform .9s cubic-bezier(.3,.9,.4,1) .8s" : "transform .5s ease-in",
            animation: asleep ? `${blanketBreath} 4.2s ease-in-out 1.7s infinite` : undefined,
          }} />
        </Box>
      </Box>
      {/* Бейдж вне позы персонажа: качка на самом бейдже — иначе на мобилке transform:translateZ(0) глушил animation. */}
      {badge && (
        <Box sx={{
          position: "absolute", inset: 0, pointerEvents: "none", zIndex: 2,
          "& > *": {
            animation: BADGE_BOB,
            willChange: "transform",
            backfaceVisibility: "hidden",
            WebkitFontSmoothing: "antialiased",
          },
        }}>
          {badge}
        </Box>
      )}
      {/* Храп: на каждом выдохе от головы вылетают «z z Z» */}
      {asleep && ["z", "z", "Z"].map((z, i) => (
        <Box key={i} component="span" aria-hidden sx={{
          position: "absolute", left: `${30 + i * 5}%`, top: `${30 - i * 6}%`, fontWeight: 800, color: "#2a62de", fontFamily: "inherit",
          fontSize: Math.max(10, Math.round(size * (0.15 + i * 0.06))), textShadow: "0 1px 0 #fff, 0 0 4px rgba(255,255,255,.9)", opacity: 0,
          animation: `${zzz} 4.2s ease-out ${2.3 + i * 0.4}s infinite`,
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
    </Box>
  );
};
