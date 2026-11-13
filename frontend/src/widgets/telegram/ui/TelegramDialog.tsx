// Подключение Telegram (BR-50): одноразовый код из веба → боту. В Telegram приходит только факт события
// и ссылка сюда — без текстов вопросов и ответов (BR-51), и только в рабочие часы (BR-52).
import { useEffect, useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Link, Stack, Typography, useTheme } from "@mui/material";
import { api } from "@/shared/api";

type Status = { connected: boolean; mode: string; bot?: string };
type Code = { code: string; link: string; bot: string; mode: string };

export const TelegramDialog = ({ open, login, onClose, onChange }: { open: boolean; login: string; onClose: () => void; onChange: (msg: string) => void }) => {
  const { t } = useTheme();
  const [status, setStatus] = useState<Status | null>(null);
  const [code, setCode] = useState<Code | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) { setCode(null); setError(""); return; }
    let alive = true;
    const check = () => api<{ telegram: Status }>(login, "/api/notifications").then((r) => { if (alive) setStatus(r.telegram); }).catch(() => undefined);
    void check();
    const timer = setInterval(check, 3000); // пока окно открыто — ждём, когда бот подтвердит код
    return () => { alive = false; clearInterval(timer); };
  }, [open, login]);

  const getCode = async () => {
    setError("");
    try { setCode(await api<Code>(login, "/api/telegram/link", { method: "POST", body: "{}" })); }
    catch (e) { setError(e instanceof Error ? e.message : "Не получилось"); }
  };
  const unlink = async () => {
    await api(login, "/api/telegram/link", { method: "DELETE" }).catch(() => undefined);
    setStatus((s) => (s ? { ...s, connected: false } : s));
    onChange("Telegram отключён");
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Уведомления в Telegram</DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>
          Бот пришлёт, что ответили на вопрос, изменился статус заявки или скоро встреча — и ссылку сюда. Тексты вопросов и ответов остаются в Напарнике.
        </Typography>
        {status?.connected ? (
          <Box sx={{ bgcolor: t.okSoft, p: 2 }}><Typography fontWeight={600}>✓ Telegram подключён</Typography>
            <Typography variant="body2" color="text.secondary">Уведомления приходят в рабочие часы, срочные — сразу.</Typography></Box>
        ) : code ? (
          <Stack gap={1.5}>
            <Typography variant="body2" color="text.secondary">Отправь боту этот код — он действует 10 минут:</Typography>
            <Box sx={{ bgcolor: t.surface, p: 2, textAlign: "center", fontSize: 28, fontWeight: 600, letterSpacing: "0.08em", fontFamily: "ui-monospace, monospace" }}>{code.code}</Box>
            {code.link
              ? <Button variant="contained" component={Link} href={code.link} target="_blank" rel="noopener">Открыть @{code.bot}</Button>
              : <Typography variant="body2" color="text.secondary">Бот на сервере в режиме журнала (нет TG_BOT_TOKEN) — сообщения пишутся в лог.</Typography>}
            <Typography variant="caption" color="text.secondary">Жду подтверждения от бота…</Typography>
          </Stack>
        ) : (
          <Button variant="contained" onClick={() => void getCode()} disabled={status?.mode === "off"}>Получить код</Button>
        )}
        {status?.mode === "off" && <Typography variant="body2" color="text.secondary" mt={1}>Telegram на сервере не настроен.</Typography>}
        {error && <Typography color="error.main" mt={1} role="alert">{error}</Typography>}
      </DialogContent>
      <DialogActions sx={{ p: 2, gap: 1 }}>
        {status?.connected && <Button variant="text" onClick={() => void unlink()}>Отключить</Button>}
        <Button variant={status?.connected ? "contained" : "text"} onClick={onClose}>Готово</Button>
      </DialogActions>
    </Dialog>
  );
};
