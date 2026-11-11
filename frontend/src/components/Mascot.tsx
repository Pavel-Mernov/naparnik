// Плюшевый маскот-помощник для плавающей кнопки чата.
import { Box, useTheme } from "@mui/material";
import mascotBlue from "../../mascot-blue.png";

export const MascotBadge = ({ size, float }: { size: number; float?: boolean }) => {
  const { palette } = useTheme();
  const dark = palette.mode === "dark";
  const width = size * 1.28;
  const height = size * 1.18;

  return (
    <Box aria-hidden sx={{
      position: "relative", width, height, flex: "none",
      "@keyframes mascot-breathe": {
        "0%, 100%": { transform: "translateY(0) rotate(0deg)" },
        "50%": { transform: "translateY(-4px) rotate(-1deg)" },
      },
    }}>
      <Box sx={{
        position: "absolute", width: size * 0.9, height: size * 0.9, borderRadius: "50%",
        right: size * 0.02, bottom: 0,
        bgcolor: dark ? "#244A76" : "#BADAFB",
        boxShadow: dark ? "8px 8px 0 rgba(0,0,0,.38)" : "8px 8px 0 rgba(0,0,0,.14)",
      }} />
      <Box component="img" src={mascotBlue} alt="" sx={{
        position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "contain", objectPosition: "center bottom",
        filter: "drop-shadow(0 7px 7px rgba(0, 46, 120, .28))",
        transformOrigin: "center bottom", transition: "transform .2s ease",
        animation: float ? "mascot-breathe 5s ease-in-out infinite" : undefined,
      }} />
    </Box>
  );
};
