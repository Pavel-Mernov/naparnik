// Чат с Напарником: Markdown без HTML-вставок, читаемые черновики с «Подтвердить / Отмена»,
// квитанция после подтверждения, «Повторить» при сбое (BR-11, BR-14, BR-43, BR-70, SC-05, SC-15).
import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { Box, Button, Checkbox, CircularProgress, Drawer, FormControlLabel, IconButton, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import { CloseRounded, SendRounded } from "@mui/icons-material";
import { MascotBadge } from "./components/Mascot";
import { api, type ChatReply } from "./api";

export type ChatMessage = { role: "user" | "assistant"; text: string; reply?: ChatReply; retryText?: string };
export type Draft = ChatReply["drafts"][number];
export type Lookup = { userName: (id: string) => string; systemTitle: (id: string) => string; stepTitle: (id: string) => string };

// ---------- Markdown: жирный, код, списки, цитаты — только React-узлы ----------

const inline = (text: string): ReactNode[] =>
  text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).filter(Boolean).map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**")) return <b key={i}>{part.slice(2, -2)}</b>;
    if (part.startsWith("`") && part.endsWith("`")) return <Box key={i} component="code" sx={{ px: 0.5, bgcolor: "action.hover", fontSize: 13 }}>{part.slice(1, -1)}</Box>;
    return <Fragment key={i}>{part}</Fragment>;
  });

export const Markdown = ({ text }: { text: string }) => {
  const blocks: ReactNode[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  const flush = () => {
    if (!list) return;
    const Tag = list.ordered ? "ol" : "ul";
    blocks.push(<Box key={blocks.length} component={Tag} sx={{ my: 0.5, pl: 2.5 }}>{list.items.map((it, i) => <li key={i}>{inline(it)}</li>)}</Box>);
    list = null;
  };
  for (const raw of text.split("\n")) {
    const line = raw.trimEnd();
    const item = line.match(/^\s*([-*•]|\d+[.)])\s+(.*)$/);
    if (item) {
      const ordered = /\d/.test(item[1]);
      if (!list || list.ordered !== ordered) { flush(); list = { ordered, items: [] }; }
      list.items.push(item[2]);
      continue;
    }
    flush();
    if (!line.trim()) continue;
    const quote = line.match(/^>\s?(.*)$/);
    const body = inline(quote ? quote[1] : line.replace(/^#{1,6}\s+/, ""));
    blocks.push(quote
      ? <Box key={blocks.length} sx={{ borderLeft: 3, borderColor: "divider", pl: 1.5, my: 0.5 }}>{body}</Box>
      : <Typography key={blocks.length} sx={{ my: 0.5 }}>{body}</Typography>);
  }
  flush();
  return <>{blocks}</>;
};

// ---------- черновик действия ----------

const ABSENCE: Record<string, string> = { day_off: "Отгул", vacation: "Отпуск", sick_leave: "Больничный" };
const STATUS_RU: Record<string, string> = { pending: "на согласовании", approved: "согласовано", granted: "выдан", rejected: "отклонено", Sent: "отправлен", Done: "готово" };
const ruDate = (iso: string) => (/^\d{4}-\d{2}-\d{2}$/.test(iso) ? new Date(`${iso}T12:00:00`).toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long" }) : iso);

// BR-14: человек видит, что именно уйдёт, а не имя инструмента.
export const describeDraft = (d: Draft, look: Lookup): { title: string; rows: Array<[string, string]>; text?: string } => {
  const a = d.args as Record<string, string | boolean | undefined>;
  const s = (k: string) => (typeof a[k] === "string" ? (a[k] as string) : "");
  switch (d.tool) {
    case "create_absence_request":
      return { title: `${ABSENCE[s("type")] ?? "Отсутствие"}: заявка в сервис отсутствий`, rows: [["Дата", ruDate(s("date"))], ["Комментарий", s("comment")]] };
    case "request_access":
      return { title: "Заявка на доступ", rows: [["Система", look.systemTitle(s("system"))], ["Зачем", s("reason")]] };
    case "send_question":
      return { title: `Вопрос: ${look.userName(s("addressee_id"))}`, rows: [["Почему", s("reason")], ["Срочность", a.urgent ? "срочно — сразу" : "обычный — в сводке"]], text: s("text") };
    case "mark_step_done":
      return { title: "Отметить шаг выполненным", rows: [["Шаг", look.stepTitle(s("step_id"))]] };
    default:
      return { title: "Действие", rows: [] };
  }
};

// Сервер подтверждает состояние карточки: правки и отмена переживают перезапуск.
export const DraftCard = ({ draft, login, look, onDone }: { draft: Draft; login: string; look: Lookup; onDone: (draft?: Draft) => void }) => {
  const { t } = useTheme();
  const [current, setCurrent] = useState(draft);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const [urgent, setUrgent] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const sync = async (signal: AbortSignal) => {
    const r = await api<{ draft: Draft }>(login, `/api/drafts/${draft.id}`, { signal });
    if (!signal.aborted) { setCurrent(r.draft); setReady(true); }
    return r.draft;
  };
  useEffect(() => {
    const c = new AbortController(); controller.current = c;
    setReady(false); setEditing(false); setError("");
    void sync(c.signal).catch((e) => { if (!c.signal.aborted) setError(e instanceof Error ? e.message : "Не получилось загрузить черновик"); });
    return () => c.abort();
  }, [login, draft.id]);
  const run = async (method: "PATCH" | "POST" | "DELETE", body: unknown) => {
    const signal = controller.current?.signal;
    if (busy || !ready || !signal || signal.aborted) return;
    setBusy(true); setError("");
    try {
      await api(login, `/api/drafts/${draft.id}${method === "POST" ? "/confirm" : ""}`, { method, body: JSON.stringify(body), signal });
      const r = await api<{ draft: Draft }>(login, `/api/drafts/${draft.id}`, { signal });
      if (signal.aborted) return;
      setCurrent(r.draft); setEditing(false); onDone(r.draft);
    } catch (e) {
      if (signal.aborted) return;
      setError(e instanceof Error ? e.message : "Не получилось выполнить действие");
      // Запрос мог выполниться, даже если ответ потерян. Сначала перечитываем состояние.
      setReady(false);
      try {
        const saved = await sync(signal);
        if (!signal.aborted) { setEditing(false); if (saved.status !== "pending") onDone(saved); }
      } catch { /* повтор загрузки ниже */ }
    } finally { if (!signal.aborted) setBusy(false); }
  };
  const view = describeDraft(current, look);
  const version = current.version ?? 1;
  if (ready && current.status === "confirmed") {
    const r = current.result?.receipt ?? {};
    const parts = [r.id, STATUS_RU[r.status] ?? r.status, r.approver, r.to && `${r.to}, ${r.delivery}`].filter(Boolean);
    return <Box sx={{ p: 1.5, mt: 1, bgcolor: t.okSoft }}><Typography fontWeight={600}>✓ Готово: {parts.join(" · ")}</Typography></Box>;
  }
  if (ready && current.status === "cancelled") return <Box sx={{ p: 1.5, mt: 1, border: `2px solid ${t.line}` }}><Typography color="text.secondary">Отменено — ничего не отправлено.</Typography></Box>;
  return (
    <Box sx={{ p: 1.5, mt: 1, bgcolor: t.aiSoft }}>
      <Typography variant="caption" sx={{ color: t.ai, fontWeight: 700 }}>Черновик — проверь перед отправкой</Typography>
      <Typography fontWeight={600} mt={0.5}>{view.title}</Typography>
      {view.rows.filter(([, v]) => v).map(([k, v]) => <Typography key={k} variant="body2"><Box component="span" color="text.secondary">{k}: </Box>{v}</Typography>)}
      {editing ? <Stack gap={1} mt={1.5}>
        <TextField label="Текст вопроса" value={text} onChange={(e) => setText(e.target.value)} fullWidth multiline minRows={3} disabled={busy}
          slotProps={{ htmlInput: { maxLength: 8000 } }} helperText="Исправь формулировку перед отправкой. Не добавляй паспортные или банковские данные." />
        <FormControlLabel control={<Checkbox checked={urgent} disabled={busy} onChange={(e) => setUrgent(e.target.checked)} />} label="Срочно — без ответа не могу продолжить работу" />
        <Stack direction={{ xs: "column", sm: "row" }} gap={1}>
          <Button variant="contained" disabled={busy || !text.trim()} onClick={() => void run("PATCH", { version, text, urgent })}>Сохранить изменения</Button>
          <Button disabled={busy} onClick={() => setEditing(false)}>Не сохранять</Button>
        </Stack>
      </Stack> : view.text && <Box sx={{ p: 1.25, mt: 1, bgcolor: t.bg }}><Markdown text={view.text} /></Box>}
      {error && <Typography variant="body2" color="error.main" mt={1} role="alert">{error}</Typography>}
      {!ready && <Button onClick={() => { const signal=controller.current?.signal; if(signal) void sync(signal).catch((e) => { if(!signal.aborted) setError(e instanceof Error ? e.message : "Не получилось"); }); }}>Обновить черновик</Button>}
      {current.tool === "send_question" && !editing && <Button variant="text" disabled={!ready || busy} onClick={() => { setText(String(current.args.text ?? "")); setUrgent(Boolean(current.args.urgent)); setEditing(true); }}>Изменить вопрос</Button>}
      <Stack direction="row" spacing={1} mt={1.5}>
        <Button variant="contained" fullWidth disabled={!ready || busy || editing} onClick={() => void run("POST", { version })}>
          {busy ? "Выполняю…" : "Подтвердить"}
        </Button>
        <Button variant="outlined" fullWidth disabled={!ready || busy} onClick={() => void run("DELETE", { version })}>Отмена</Button>
      </Stack>
    </Box>
  );
};

// ---------- окно чата ----------

/** Отметка полезности ответа из базы (SCR-I2). «Не помогло» сразу ведёт к человеку. */
const Feedback = ({ onNotHelpful, onRate }: { onNotHelpful: () => void; onRate: (value: string) => void }) => {
  const [picked, setPicked] = useState("");
  const options: Array<[string, string]> = [["helped", "Помогло"], ["not", "Не помогло"], ["stale", "Устарело"]];
  const thanks: Record<string, string> = { helped: "Спасибо, учту", not: "Ищу, кто поможет…", stale: "Спасибо, передам владельцу базы" };
  if (picked) return <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.75 }}>{thanks[picked]}</Typography>;
  return <Stack direction="row" gap={0.75} mt={0.75} flexWrap="wrap">
    {options.map(([id, label]) => <Button key={id} variant="outlined" size="small" sx={{ minHeight: 32, py: 0.25, px: 1.25, fontSize: 13 }}
      onClick={() => { setPicked(id); onRate(id === "not" ? "not_helped" : id); if (id === "not") onNotHelpful(); }}>{label}</Button>)}
  </Stack>;
};

const QUICK = ["Что мне сегодня делать?", "Что такое витрина?", "Нужен отгул в пятницу", "Нужен доступ к Redash"];

/** На компьютере — панель справа, на телефоне — шторка снизу (как в дизайн-системе Авито). */
export const ChatDrawer = ({ open, onClose, text, setText, messages, loading, onSend, login, look, onAction }: {
  open: boolean; onClose: () => void; text: string; setText: (text: string) => void; messages: ChatMessage[]; loading: boolean;
  onSend: (text?: string) => void; login: string; look: Lookup; onAction: () => void;
}) => {
  const theme = useTheme();
  const { t } = theme;
  const mobile = useMediaQuery(theme.breakpoints.down("sm"));
  const end = useRef<HTMLDivElement>(null);
  const [pending, setPending] = useState<Draft[]>([]);
  const [pendingError, setPendingError] = useState("");
  const [pendingRetry, setPendingRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    setPending([]); setPendingError("");
    if (open) void api<{ drafts: Draft[] }>(login, "/api/drafts", { signal: c.signal })
      .then((r) => { if (!c.signal.aborted) setPending(r.drafts); })
      .catch(() => { if (!c.signal.aborted) setPendingError("Не получилось восстановить сохранённые черновики."); });
    return () => c.abort();
  }, [open, login, pendingRetry]);
  const inMessages = new Set(messages.flatMap((m) => m.reply?.drafts.map((d) => d.id) ?? []));
  useEffect(() => { end.current?.scrollIntoView({ behavior: "smooth" }); }, [messages, loading, open]);
  return (
    <Drawer anchor={mobile ? "bottom" : "right"} open={open} onClose={onClose} ModalProps={{ keepMounted: true }}
      slotProps={{ paper: { sx: mobile ? { height: "88dvh", borderTop: `2px solid ${t.line}` } : { width: 440, borderLeft: `2px solid ${t.line}` } } }}>
      <Stack height="100%" sx={{ bgcolor: t.bg }}>
        {mobile && <Box aria-hidden sx={{ width: 40, height: 4, bgcolor: t.line, mx: "auto", mt: 1 }} />}
        <Stack direction="row" alignItems="center" spacing={1.5} sx={{ px: 2, py: 1.5, borderBottom: `2px solid ${t.line}` }}>
          <MascotBadge size={48} />
          <Box flex={1}>
            <Typography fontWeight={600}>Напарник</Typography>
            <Typography variant="caption" sx={{ color: t.ai }}>{loading ? "думает…" : "ИИ-помощник · без подтверждения ничего не отправит"}</Typography>
          </Box>
          <IconButton onClick={onClose} aria-label="Закрыть чат"><CloseRounded /></IconButton>
        </Stack>
        <Stack spacing={2} sx={{ p: 2, flex: 1, overflowY: "auto" }}>
          {pendingError && <Box><Typography color="error.main" role="alert">{pendingError}</Typography><Button onClick={() => setPendingRetry((n) => n + 1)}>Повторить загрузку</Button></Box>}
          {pending.filter((d) => !inMessages.has(d.id)).map((d) => <DraftCard key={`${login}/${d.id}`} draft={d} login={login} look={look} onDone={onAction} />)}
          {messages.map((m, index) => (
            <Box key={`${m.role}-${index}`} alignSelf={m.role === "user" ? "flex-end" : "flex-start"} maxWidth="92%">
              <Box sx={{ px: 1.75, py: 1.25, bgcolor: m.role === "user" ? t.inverse : t.surface, color: m.role === "user" ? t.onInverse : t.text }}>
                {m.role === "user" ? <Typography>{m.text}</Typography> : <Markdown text={m.text} />}
              </Box>
              {m.reply?.sources.map((s) => (
                <Box key={s.page_id} sx={{ px: 1.5, py: 1, mt: 0.75, borderLeft: `3px solid ${t.text}`, bgcolor: t.bg }}>
                  <Typography variant="body2" fontWeight={600}>{s.title}</Typography>
                  <Typography variant="caption" color="text.secondary">{s.author} · {s.updated_at}{s.label ? ` · ${s.label}` : ""}</Typography>
                </Box>
              ))}
              {m.reply?.drafts.map((d) => <DraftCard key={d.id} draft={d} login={login} look={look} onDone={onAction} />)}
              {m.reply && m.reply.sources.length > 0 && !m.reply.drafts.length && <Feedback onNotHelpful={() => onSend(`Не помогло, нужен человек: ${messages[index - 1]?.text ?? ""}`)}
                onRate={(value) => { void api(login, "/api/feedback", { method: "POST", body: JSON.stringify({ page_id: m.reply?.sources[0]?.page_id, value }) }).catch(() => undefined); }} />}
              {m.retryText && <Button variant="text" size="small" sx={{ mt: 1, minHeight: 36 }} onClick={() => onSend(m.retryText)}>Повторить</Button>}
            </Box>
          ))}
          {messages.length === 1 && !loading && <Stack direction="row" flexWrap="wrap" gap={1}>
            {QUICK.map((q) => <Button key={q} variant="outlined" size="small" sx={{ minHeight: 36 }} onClick={() => onSend(q)}>{q}</Button>)}
          </Stack>}
          {loading && <Stack direction="row" spacing={1} alignItems="center"><CircularProgress size={18} sx={{ color: t.ai }} /><Typography color="text.secondary">Думаю…</Typography></Stack>}
          <div ref={end} />
        </Stack>
        <Stack direction="row" spacing={1} sx={{ p: 1.5, borderTop: `2px solid ${t.line}` }}>
          <TextField autoFocus fullWidth multiline maxRows={4} value={text} onChange={(e) => setText(e.target.value)} placeholder="Спроси о команде или задачах"
            slotProps={{ htmlInput: { "aria-label": "Сообщение Напарнику" } }} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); onSend(); } }} />
          <IconButton onClick={() => onSend()} disabled={!text.trim() || loading} aria-label="Отправить"
            sx={{ bgcolor: t.inverse, color: t.onInverse, alignSelf: "flex-end", "&:hover": { bgcolor: t.inverse }, "&.Mui-disabled": { bgcolor: t.surface } }}><SendRounded /></IconButton>
        </Stack>
      </Stack>
    </Drawer>
  );
};
