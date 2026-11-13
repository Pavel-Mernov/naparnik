// Настройки (SCR-I8): режим подсказок, видимость вопросов для бадди, напоминания, «не беспокоить».
// Каждое изменение сохраняется сразу; рабочие часы — из оргструктуры, только показываются.
import { useEffect, useState } from "react";
import {
  Box, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, Radio, RadioGroup, Stack, Switch, Typography,
  useMediaQuery, useTheme,
} from "@mui/material";
import { api, type Settings } from "@/shared/api";

const MODES: Array<[Settings["proactivity"], string, string]> = [
  ["daily_plan", "План на день", "Утром — шаги на сегодня, плюс напоминания о встречах и дедлайнах"],
  ["deadlines_and_meetings", "Только дедлайны и встречи", "Без утреннего плана"],
  ["off", "Выключено", "Напарник пишет, только когда спросишь"],
];
const QUIET: Array<[number, string]> = [[1, "На час"], [3, "На 3 часа"], [8, "До конца дня"]];

const Section = ({ title, children }: { title: string; children: React.ReactNode }) => (
  <Box component="section" aria-label={title}>
    <Typography fontWeight={600} mb={0.75}>{title}</Typography>
    {children}
  </Box>
);

export const SettingsDialog = ({ open, login, intern, onClose, onTelegram }: {
  open: boolean; login: string; intern: boolean; onClose: () => void; onTelegram: () => void;
}) => {
  const theme = useTheme();
  const mobile = useMediaQuery(theme.breakpoints.down("sm"));
  const [s, setS] = useState<Settings | null>(null);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    const c = new AbortController();
    setS(null); setStatus(""); setError("");
    api<{ settings: Settings }>(login, "/api/settings", { signal: c.signal })
      .then((r) => setS(r.settings))
      .catch((e) => { if (!c.signal.aborted) setError(e instanceof Error ? e.message : "Не удалось загрузить настройки"); });
    return () => c.abort();
  }, [open, login]);

  const save = async (patch: Record<string, unknown>) => {
    setBusy(true); setError(""); setStatus("");
    try {
      const r = await api<{ settings: Settings }>(login, "/api/settings", { method: "PATCH", body: JSON.stringify(patch) });
      setS(r.settings); setStatus("Сохранено");
    } catch (e) { setError(e instanceof Error ? e.message : "Не удалось сохранить"); }
    finally { setBusy(false); }
  };
  const toggleReminder = (code: string, on: boolean) => {
    if (!s) return;
    void save({ reminders: on ? [...new Set([...s.reminders, code])] : s.reminders.filter((x) => x !== code) });
  };
  const quietTill = s?.quiet_until ? new Date(s.quiet_until).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" }) : "";

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" fullScreen={mobile}>
      <DialogTitle>Настройки</DialogTitle>
      <DialogContent>
        {!s && !error && <Typography color="text.secondary">Загружаю…</Typography>}
        {s && <Stack gap={3}>
          {intern && <Section title="Как Напарник подсказывает">
            <RadioGroup value={s.proactivity} onChange={(e) => void save({ proactivity: e.target.value })}>
              {MODES.map(([value, label, hint]) => <FormControlLabel key={value} value={value} disabled={busy} control={<Radio />}
                label={<Box py={0.5}><Typography>{label}</Typography><Typography variant="body2" color="text.secondary">{hint}</Typography></Box>} />)}
            </RadioGroup>
          </Section>}
          {intern && <Section title="Что видит бадди">
            <FormControlLabel disabled={busy} control={<Switch checked={s.question_text_visible_to_buddy}
              onChange={(e) => void save({ question_text_visible_to_buddy: e.target.checked })} />}
              label="Показывать бадди текст моих вопросов экспертам" />
            <Typography variant="body2" color="text.secondary">Если выключить, бадди увидит только адресата и статус — без текста вопроса и ответа.</Typography>
          </Section>}
          <Section title="Напоминания о встречах и событиях">
            <Stack direction="row" gap={2} flexWrap="wrap">
              <FormControlLabel disabled={busy} control={<Checkbox checked={s.reminders.includes("1d")} onChange={(e) => toggleReminder("1d", e.target.checked)} />} label="За день" />
              <FormControlLabel disabled={busy} control={<Checkbox checked={s.reminders.includes("1h")} onChange={(e) => toggleReminder("1h", e.target.checked)} />} label="За час" />
            </Stack>
          </Section>
          <Section title="Не беспокоить">
            {s.quiet_until ? <Stack direction="row" gap={1.5} alignItems="center" flexWrap="wrap">
              <Typography>Включено до {quietTill}. Срочное всё равно придёт.</Typography>
              <Button variant="outlined" size="small" disabled={busy} onClick={() => void save({ quiet_hours: 0 })}>Выключить</Button>
            </Stack> : <>
              <Typography variant="body2" color="text.secondary" mb={1}>Несрочные уведомления в Telegram подождут, срочные придут сразу.</Typography>
              <Stack direction="row" gap={1} flexWrap="wrap">
                {QUIET.map(([h, label]) => <Button key={h} variant="outlined" size="small" disabled={busy} onClick={() => void save({ quiet_hours: h })}>{label}</Button>)}
              </Stack>
            </>}
          </Section>
          <Section title="Рабочие часы">
            <Typography>{s.working_hours || "не указаны"}</Typography>
            <Typography variant="body2" color="text.secondary">Из оргструктуры: в это время приходят уведомления и подбираются слоты встреч.</Typography>
          </Section>
          <Section title="Telegram">
            <Button variant="text" size="small" sx={{ px: 0 }} onClick={onTelegram}>Подключить или отключить бота</Button>
          </Section>
        </Stack>}
        {error && <Typography color="error.main" role="alert" mt={2}>{error}</Typography>}
      </DialogContent>
      <DialogActions sx={{ p: 2, justifyContent: "space-between" }}>
        <Typography variant="body2" color="text.secondary" role="status" sx={{ pl: 1 }}>{status}</Typography>
        <Button variant="contained" onClick={onClose}>Готово</Button>
      </DialogActions>
    </Dialog>
  );
};
