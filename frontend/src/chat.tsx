// Чат с Напарником: Markdown без HTML-вставок, читаемые черновики с «Подтвердить / Отмена»,
// квитанция после подтверждения, «Повторить» при сбое (BR-11, BR-14, BR-43, BR-70, SC-05, SC-15).
import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";
import { Avatar, Box, Button, Chip, CircularProgress, Drawer, IconButton, Paper, Stack, TextField, Typography } from "@mui/material";
import { CloseRounded, SendRounded } from "@mui/icons-material";
import mascotCat from "../mascot-cat.png";
import { ApiError, api, type ChatReply } from "./api";

export type ChatMessage = { role: "user" | "assistant"; text: string; reply?: ChatReply; retryText?: string };
export type Draft = ChatReply["drafts"][number];
export type Lookup = { userName: (id: string) => string; systemTitle: (id: string) => string; stepTitle: (id: string) => string };

// ---------- Markdown: жирный, код, списки, цитаты — только React-узлы ----------

const inline = (text: string): ReactNode[] =>
  text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).filter(Boolean).map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**")) return <b key={i}>{part.slice(2, -2)}</b>;
    if (part.startsWith("`") && part.endsWith("`")) return <Box key={i} component="code" sx={{ px: 0.5, bgcolor: "rgba(0,0,0,.06)", fontSize: 13 }}>{part.slice(1, -1)}</Box>;
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
      ? <Box key={blocks.length} sx={{ borderLeft: "3px solid #0af", pl: 1.5, my: 0.5 }}>{body}</Box>
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

type DraftState = { status: "new" | "busy" | "done" | "cancelled" | "error"; note?: string; retry?: boolean };

const DraftCard = ({ draft, login, look, onDone }: { draft: Draft; login: string; look: Lookup; onDone: () => void }) => {
  const [st, setSt] = useState<DraftState>({ status: "new" });
  const view = describeDraft(draft, look);
  const confirm = async () => {
    setSt({ status: "busy" });
    try {
      const res = await api<{ receipt?: Record<string, string> }>(login, `/api/drafts/${draft.id}/confirm`, { method: "POST", body: "{}" });
      const r = res.receipt ?? {};
      const parts = [r.id, STATUS_RU[r.status] ?? r.status, r.approver, r.to && `${r.to}, ${r.delivery}`].filter(Boolean);
      setSt({ status: "done", note: `Готово: ${parts.join(" · ")}` });
      onDone();
    } catch (e) {
      const unavailable = e instanceof ApiError && e.status === 503;
      setSt({ status: "error", note: e instanceof Error ? e.message : "Не получилось", retry: unavailable });
    }
  };
  if (st.status === "done") return <Paper sx={{ p: 1.5, mt: 1, borderRadius: 3, bgcolor: "#e4f6eb" }}><Typography fontWeight={800}>✅ {st.note}</Typography></Paper>;
  if (st.status === "cancelled") return <Paper variant="outlined" sx={{ p: 1.5, mt: 1, borderRadius: 3 }}><Typography color="text.secondary">Отменено — ничего не отправлено.</Typography></Paper>;
  return (
    <Paper sx={{ p: 1.5, mt: 1, borderRadius: 3, bgcolor: "#fff6d8" }}>
      <Typography variant="overline" fontWeight={800}>Черновик — проверь перед отправкой</Typography>
      <Typography fontWeight={800}>{view.title}</Typography>
      {view.rows.filter(([, v]) => v).map(([k, v]) => <Typography key={k} variant="body2"><Box component="span" color="text.secondary">{k}: </Box>{v}</Typography>)}
      {view.text && <Paper variant="outlined" sx={{ p: 1, mt: 1, bgcolor: "#fff" }}><Markdown text={view.text} /></Paper>}
      {st.status === "error" && <Typography variant="body2" color="error" mt={1}>{st.note}{st.retry ? " Черновик сохранён." : ""}</Typography>}
      <Stack direction="row" spacing={1} mt={1.5}>
        <Button variant="contained" fullWidth disabled={st.status === "busy" || (st.status === "error" && !st.retry)} onClick={() => void confirm()}>
          {st.status === "busy" ? "Отправляю…" : st.status === "error" && st.retry ? "Повторить" : "Подтвердить"}
        </Button>
        <Button variant="outlined" fullWidth disabled={st.status === "busy"} onClick={() => setSt({ status: "cancelled" })}>Отмена</Button>
      </Stack>
    </Paper>
  );
};

// ---------- окно чата ----------

export const ChatDrawer = ({ open, onClose, text, setText, messages, loading, onSend, login, look, onAction }: {
  open: boolean; onClose: () => void; text: string; setText: (text: string) => void; messages: ChatMessage[]; loading: boolean;
  onSend: (text?: string) => void; login: string; look: Lookup; onAction: () => void;
}) => {
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => { end.current?.scrollIntoView({ behavior: "smooth" }); }, [messages, loading, open]);
  return (
    <Drawer anchor="right" open={open} onClose={onClose} PaperProps={{ sx: { width: { xs: "100%", sm: 440 }, borderRadius: { sm: "28px 0 0 28px" } } }}>
      <Stack height="100%">
        <Stack direction="row" alignItems="center" spacing={1.5} sx={{ p: 2.5, bgcolor: "#f0f6ff" }}>
          <Avatar src={mascotCat} sx={{ width: 58, height: 58, bgcolor: "#dff1ff" }} />
          <Box flex={1}><Typography fontWeight={800}>Напарник</Typography><Typography variant="caption" color="success.main">● рядом и готов помочь</Typography></Box>
          <IconButton onClick={onClose} aria-label="Закрыть чат"><CloseRounded /></IconButton>
        </Stack>
        <Stack spacing={2} sx={{ p: 2.5, flex: 1, overflowY: "auto" }}>
          {messages.map((m, index) => (
            <Box key={`${m.role}-${index}`} alignSelf={m.role === "user" ? "flex-end" : "flex-start"} maxWidth="92%">
              <Paper sx={{ p: 1.5, borderRadius: 4, bgcolor: m.role === "user" ? "primary.main" : "#f2f2f2", color: m.role === "user" ? "white" : "text.primary" }}>
                {m.role === "user" ? <Typography>{m.text}</Typography> : <Markdown text={m.text} />}
              </Paper>
              {m.reply?.sources.map((s) => (
                <Paper variant="outlined" key={s.page_id} sx={{ p: 1.5, mt: 1, borderRadius: 3 }}>
                  <Typography fontWeight={800}>{s.title}</Typography>
                  <Typography variant="caption" color="text.secondary">{s.author} · {s.updated_at}</Typography>
                  {s.label && <Chip label={s.label} color="warning" size="small" sx={{ mt: 1, display: "flex", width: "fit-content" }} />}
                </Paper>
              ))}
              {m.reply?.drafts.map((d) => <DraftCard key={d.id} draft={d} login={login} look={look} onDone={onAction} />)}
              {m.retryText && <Button size="small" sx={{ mt: 1 }} onClick={() => onSend(m.retryText)}>Повторить</Button>}
            </Box>
          ))}
          {loading && <Stack direction="row" spacing={1} alignItems="center"><CircularProgress size={20} /><Typography color="text.secondary">Думаю…</Typography></Stack>}
          <div ref={end} />
        </Stack>
        <Stack direction="row" spacing={1} sx={{ p: 2, borderTop: "1px solid #eee" }}>
          <TextField autoFocus fullWidth multiline maxRows={4} value={text} onChange={(e) => setText(e.target.value)} placeholder="Спроси о команде или задачах…"
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); onSend(); } }} />
          <IconButton color="primary" onClick={() => onSend()} disabled={!text.trim() || loading} aria-label="Отправить"><SendRounded /></IconButton>
        </Stack>
      </Stack>
    </Drawer>
  );
};
