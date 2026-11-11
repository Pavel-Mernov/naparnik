// Объёмный значок маскота: светлая «сцена»-круг со светом и тенью, маскот стоит в ней,
// голова и лапа выходят за верхний край круга — эффект «выглядывает». Исходник персонажа не меняем (08-DESIGN 2.1).
import { Box, useTheme } from "@mui/material";
import mascotCat from "../../mascot-cat.png";

// Пропорции исходника mascot-cat.png: 1448×1086, мордочка — около 57% ширины слева.
const ASPECT = 1086 / 1448;
const FACE_X = 0.57;

export const MascotBadge = ({ size, float }: { size: number; float?: boolean }) => {
  const { t, palette } = useTheme();
  const dark = palette.mode === "dark";
  const W = size * 1.25;           // область отрисовки шире круга: уши и лапа не обрезаются сверху
  const H = size * 1.12;
  const d = size * 0.86;           // диаметр «сцены»
  const r = d / 2, cx = W / 2, cy = H - r;
  const imgW = d * 1.5, imgH = imgW * ASPECT;
  // Ниже центра круга видно только то, что внутри круга; выше — всё: так голова «выглядывает» из сцены.
  const clip = `path("M0 0 H${W} V${cy} H${cx + r} A${r} ${r} 0 0 1 ${cx - r} ${cy} H0 Z")`;
  return (
    <Box aria-hidden sx={{ position: "relative", width: W, height: H, flex: "none" }}>
      <Box sx={{
        position: "absolute", left: cx - r, top: cy - r, width: d, height: d, borderRadius: "50%",
        background: dark
          ? "radial-gradient(circle at 34% 28%, #6A8FC4 0%, #2C4B7A 48%, #142540 100%)"
          : "radial-gradient(circle at 34% 28%, #FFFFFF 0%, #E3F2FF 40%, #8FCBFF 100%)",
        boxShadow: dark
          ? "inset 0 -6px 12px rgba(0,0,0,.45), inset 0 4px 8px rgba(255,255,255,.14), 0 10px 22px rgba(0,0,0,.55)"
          : `inset 0 -7px 14px rgba(0,110,220,.28), inset 0 5px 9px rgba(255,255,255,.95), 0 12px 24px rgba(0,80,180,.30), 0 0 0 3px ${t.bg}`,
      }} />
      <Box sx={{
        position: "absolute", inset: 0, clipPath: clip,
        "& img": float ? { animation: "mascot-breathe 5s ease-in-out infinite" } : undefined,
        "@keyframes mascot-breathe": { "0%, 100%": { transform: "translateY(0)" }, "50%": { transform: "translateY(-3px)" } },
      }}>
        <Box component="img" src={mascotCat} alt="" sx={{
          position: "absolute", width: imgW, height: imgH, left: cx - imgW * FACE_X, top: cy + r - imgH + d * 0.02, display: "block",
          filter: "drop-shadow(0 4px 5px rgba(0, 40, 120, .28))", transition: "transform .2s ease",
        }} />
      </Box>
    </Box>
  );
};
