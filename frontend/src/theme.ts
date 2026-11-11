// Токены Напарника — по логике дизайн-системы Авито (Хабр, «Как устроена библиотека дизайн-системы Авито»):
// именованные цвета и тексты, «наугад ничего не берём». Палитра — цвета логотипа Авито и нейтральные
// с сайта start.avito.ru: чёрный текст, серые плашки, линии 2 px, без скруглений и теней.
// Контраст проверен: синий #00AAFF — только заливкой под чёрным текстом (8.2:1), для ссылок — #0A7BC2 (4.5:1).
import { createTheme, type Theme } from "@mui/material";

export type Mode = "light" | "dark";

export const brand = {
  blue: "#00AAFF",
  blueLight: "#BADAFB",
  green: "#04E061",
  red: "#FF4053",
  violet: "#965EEB",
} as const;

type Tokens = {
  bg: string; surface: string; surfaceHover: string; line: string; text: string; muted: string;
  link: string; inverse: string; onInverse: string;
  ai: string; aiSoft: string; okSoft: string; badSoft: string; waitSoft: string; todaySoft: string;
};

export const tokens: Record<Mode, Tokens> = {
  light: {
    bg: "#F2F2F2", surface: "#FFFFFF", surfaceHover: "#E9E9E9", line: "#DADCD9", text: "#000000", muted: "#626362",
    link: "#0A7BC2", inverse: "#000000", onInverse: "#FFFFFF",
    ai: "#6A35C9", aiSoft: "#EFE7FC", okSoft: "#D9F9E6", badSoft: "#FFE3E6", waitSoft: "#E3EFFD", todaySoft: brand.blueLight,
  },
  dark: {
    bg: "#0F0F0F", surface: "#1C1C1C", surfaceHover: "#262626", line: "#2E2E2E", text: "#F3F3F3", muted: "#A6A7A6",
    link: "#5CC6FF", inverse: "#FFFFFF", onInverse: "#000000",
    ai: "#C3A2F7", aiSoft: "#2B2140", okSoft: "#12331F", badSoft: "#3D1A1F", waitSoft: "#1A2A3D", todaySoft: "#1D3A57",
  },
};

export const font = '"Onest", "Golos Text", system-ui, -apple-system, "Segoe UI", sans-serif';

declare module "@mui/material/styles" {
  interface Theme { t: Tokens }
  interface ThemeOptions { t?: Tokens }
}

export const makeTheme = (mode: Mode): Theme => {
  const t = tokens[mode];
  return createTheme({
    t,
    palette: {
      mode,
      primary: { main: t.inverse, contrastText: t.onInverse },
      secondary: { main: brand.violet },
      success: { main: brand.green, contrastText: "#000" },
      error: { main: brand.red, contrastText: "#000" },
      background: { default: t.bg, paper: t.bg },
      text: { primary: t.text, secondary: t.muted },
      divider: t.line,
    },
    shape: { borderRadius: 0 },
    typography: {
      fontFamily: font,
      fontSize: 15,
      h1: { fontSize: "clamp(36px, 5vw, 64px)", fontWeight: 700, letterSpacing: "-0.045em", lineHeight: 0.98 },
      h2: { fontSize: 24, fontWeight: 700, letterSpacing: "-0.025em", lineHeight: 1.15 },
      h3: { fontSize: 18, fontWeight: 700, lineHeight: 1.25 },
      body1: { fontSize: 16, lineHeight: 1.5 },
      body2: { fontSize: 14, lineHeight: 1.45 },
      caption: { fontSize: 13, lineHeight: 1.4 },
      button: { textTransform: "none", fontWeight: 700, fontSize: 15 },
    },
    components: {
      MuiCssBaseline: {
        styleOverrides: {
          body: { backgroundColor: t.bg, color: t.text },
          "a, button, [role=button], label[for], input[type=checkbox]": { cursor: "pointer" },
          ":focus-visible": { outline: `2px solid ${t.link}`, outlineOffset: 2 },
          "@media (prefers-reduced-motion: reduce)": { "*": { animation: "none !important", transition: "none !important" } },
        },
      },
      MuiButton: {
        defaultProps: { disableElevation: true, disableRipple: true },
        styleOverrides: {
          root: { borderRadius: 0, minHeight: 46, padding: "11px 20px", transition: "background-color .15s ease, transform .15s ease", "&:active": { transform: "translateY(1px)" } },
          containedPrimary: { "&:hover": { backgroundColor: mode === "light" ? "#262626" : "#D9D9D9" } },
          text: { color: t.text, backgroundColor: t.surface, "&:hover": { backgroundColor: t.surfaceHover } },
          outlined: { borderWidth: 2, borderColor: t.line, color: t.text, "&:hover": { borderWidth: 2, borderColor: t.text, backgroundColor: "transparent" } },
        },
      },
      MuiIconButton: { defaultProps: { disableRipple: true }, styleOverrides: { root: { borderRadius: 0, width: 44, height: 44, "&:hover": { backgroundColor: t.surface } } } },
      MuiPaper: { defaultProps: { elevation: 0 }, styleOverrides: { root: { backgroundImage: "none" } } },
      MuiDialog: { styleOverrides: { paper: { borderRadius: 0, border: `2px solid ${t.line}` } } },
      MuiOutlinedInput: {
        // Поле ввода в Авито — серый блок
        styleOverrides: {
          root: { borderRadius: 0, backgroundColor: t.surface, "& fieldset": { border: "none" }, "&.Mui-focused": { outline: `2px solid ${t.text}` } },
        },
      },
      MuiInputLabel: { styleOverrides: { root: { color: t.muted } } },
      MuiChip: { styleOverrides: { root: { borderRadius: 0, fontWeight: 600 } } },
      MuiCheckbox: { defaultProps: { disableRipple: true }, styleOverrides: { root: { color: t.muted, "&.Mui-checked": { color: t.text } } } },
      MuiLinearProgress: { styleOverrides: { root: { height: 6, borderRadius: 0, backgroundColor: t.surface }, bar: { backgroundColor: brand.green } } },
      MuiSnackbarContent: { styleOverrides: { root: { borderRadius: 0, backgroundColor: t.inverse, color: t.onInverse, fontFamily: font } } },
      MuiTooltip: { styleOverrides: { tooltip: { borderRadius: 0, backgroundColor: t.inverse, color: t.onInverse, fontSize: 13 } } },
      MuiMenu: { styleOverrides: { paper: { borderRadius: 0, border: `2px solid ${t.line}` } } },
      MuiPopover: { styleOverrides: { paper: { borderRadius: 0, border: `2px solid ${t.line}` } } },
    },
  });
};
