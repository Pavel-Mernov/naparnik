import { Box, IconButton, Tooltip } from "@mui/material";
import { DarkModeOutlined, LightModeOutlined } from "@mui/icons-material";
import type { Mode } from "@/shared/theme";

// Обе иконки лежат друг на друге: при смене темы одна укатывается с поворотом, другая выкатывается.
// Вид кнопки прежний — меняется только переход между иконками.
const icon = (shown: boolean, turn: number) => ({
  position: "absolute",
  inset: 0,
  transition: "transform 450ms cubic-bezier(0.34, 1.56, 0.64, 1), opacity 300ms ease",
  transform: shown ? "rotate(0deg) scale(1)" : `rotate(${turn}deg) scale(0.4)`,
  opacity: shown ? 1 : 0,
  "@media (prefers-reduced-motion: reduce)": { transition: "opacity 150ms ease", transform: "none" },
}) as const;

/** Переключатель светлой / тёмной темы. */
export const ThemeToggle = ({ mode, onToggle }: { mode: Mode; onToggle: () => void }) => (
  <Tooltip title={mode === "light" ? "Тёмная тема" : "Светлая тема"}>
    <IconButton aria-label="Сменить тему" onClick={onToggle}>
      <Box component="span" sx={{ position: "relative", display: "block", width: 24, height: 24 }}>
        <DarkModeOutlined aria-hidden data-testid="theme-icon-moon" sx={icon(mode === "light", -120)} />
        <LightModeOutlined aria-hidden data-testid="theme-icon-sun" sx={icon(mode === "dark", 120)} />
      </Box>
    </IconButton>
  </Tooltip>
);
