// Вопросы людям. Адресат отвечает прямо здесь (US-10), автор проверяет страницу в базе перед публикацией (BR-30).
import { useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import type { ChatTurn, KbDraft, Question } from "@/shared/api";
import { answerDue, type DataState, type Role } from "@/shared/lib";
import { AiTag, Block, Empty, PageHead, Row, StatusTag } from "@/shared/ui";
import { Markdown } from "@/widgets/chat";

const questionTitle = (q: Question) => (q.text_hidden ? "Текст скрыт настройками стажёра" : q.title || q.text || "Вопрос");

/** Статус вопроса; просроченный (SC-23) — красная плашка «Просрочен» вместо «Ждёт ответа». */
const QuestionStatus = ({ q }: { q: Question }) => q.status === "Sent" && q.overdue ? <StatusTag status="Overdue" label="Просрочен" /> : <StatusTag status={q.status} />;

const Incoming = ({ q, from, onAnswer }: { q: Question; from: string; onAnswer: (q: Question, text: string) => Promise<boolean> }) => {
  const { t } = useTheme();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <Box sx={{ border: `2px solid ${t.line}`, p: { xs: 2, md: 2.5 }, mb: 2 }}>
      <Stack direction="row" justifyContent="space-between" gap={2} mb={1}>
        <Typography variant="body2" color="text.secondary">{from}{q.urgent ? " · срочно" : ""}{answerDue(q) ? ` · ${answerDue(q)}` : ""}</Typography>
        <QuestionStatus q={q} />
      </Stack>
      <Typography variant="h3" component="h3" mb={1}>{questionTitle(q)}</Typography>
      {q.text && q.text !== q.title && <Box sx={{ color: "text.secondary", mb: 2 }}><Markdown text={q.text} /></Box>}
      <TextField fullWidth multiline minRows={3} label="Твой ответ" value={text} onChange={(e) => setText(e.target.value)}
        helperText="Ответ сразу увидит стажёр, а я оформлю его страницей в базе — опубликую после твоего «Ок»." />
      <Button variant="contained" sx={{ mt: 1.5 }} disabled={!text.trim() || busy}
        onClick={async () => { setBusy(true); if (await onAnswer(q, text)) setText(""); setBusy(false); }}>
        {busy ? "Отправляю…" : "Ответить"}
      </Button>
    </Box>
  );
};

const DraftDialog = ({ draft, folder, onClose, onApprove }: { draft: KbDraft | null; folder?: string; onClose: () => void; onApprove: (d: KbDraft, title: string, body: string) => void }) => {
  const mobile = useMediaQuery(useTheme().breakpoints.down("sm"));
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [seen, setSeen] = useState<string | null>(null);
  if (draft && seen !== draft.id) { setSeen(draft.id); setTitle(draft.title); setBody(draft.body); }
  return (
    <Dialog open={Boolean(draft)} onClose={onClose} fullWidth maxWidth="sm" fullScreen={mobile}>
      <DialogTitle>Проверь страницу перед публикацией</DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>Я оформил твой ответ страницей «Вопрос-ответ». Автор — ты{folder ? `, папка «${folder}»` : ""}. Можно поправить.</Typography>
        <Stack gap={2}>
          <TextField label="Заголовок" value={title} onChange={(e) => setTitle(e.target.value)} fullWidth />
          <TextField label="Текст" value={body} onChange={(e) => setBody(e.target.value)} fullWidth multiline minRows={5} />
        </Stack>
      </DialogContent>
      <DialogActions sx={{ p: 2, gap: 1 }}>
        <Button variant="text" onClick={onClose}>Позже</Button>
        <Button variant="contained" disabled={!title.trim() || !body.trim()} onClick={() => draft && onApprove(draft, title, body)}>Ок, опубликовать</Button>
      </DialogActions>
    </Dialog>
  );
};

const askedAt = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString("ru-RU", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
};

/** Что стало с вопросом Напарнику: ответ из базы, черновик, упрощённый режим. */
const askedSummary = (x: ChatTurn) => {
  const drafts = x.reply.drafts;
  if (drafts.some((d) => d.status === "confirmed")) return "Напарник оформил — подтверждено";
  if (drafts.some((d) => !d.status || d.status === "pending")) return "Черновик ждёт подтверждения";
  if (x.reply.sources.length) return `Ответ из базы: ${x.reply.sources[0].title}`;
  if (x.reply.degraded) return "Ответ в упрощённом режиме";
  return "Ответил Напарник";
};

/** Вопросы Напарнику — сохраняются на сервере, новые сверху; ответ раскрывается по нажатию. */
export const AskedList = ({ asked, onOpenChat }: { asked: ChatTurn[]; onOpenChat?: () => void }) => {
  const [open, setOpen] = useState("");
  const { t } = useTheme();
  return <>
    {[...asked].reverse().map((x) => (
      <Box key={x.id} sx={{ py: 1.5, borderBottom: 1, borderColor: "divider", "&:last-child": { borderBottom: "none" } }}>
        <Box component="button" onClick={() => setOpen(open === x.id ? "" : x.id)} aria-expanded={open === x.id}
          sx={{ all: "unset", cursor: "pointer", display: "block", width: "100%", "&:focus-visible": { outline: `2px solid ${t.ai}` } }}>
          <Typography fontWeight={500}>{x.question}</Typography>
          <Typography variant="body2" color="text.secondary">{askedSummary(x)}{askedAt(x.at) ? ` · ${askedAt(x.at)}` : ""}</Typography>
        </Box>
        {open === x.id && <Box sx={{ mt: 1.5, pl: 2, borderLeft: 3, borderColor: t.ai }}>
          <Markdown text={x.reply.text} />
          {x.reply.sources.map((s) => <Typography key={s.page_id} variant="caption" color="text.secondary" sx={{ display: "block" }}>
            Источник: {s.title} · {s.author} · {s.updated_at}{s.label ? ` · ${s.label}` : ""}</Typography>)}
          {onOpenChat && <Button variant="text" size="small" sx={{ mt: 1, minHeight: 36 }} onClick={onOpenChat}>Открыть чат</Button>}
        </Box>}
      </Box>))}
  </>;
};

export const QuestionsPage = ({ data, role, asked = [], onAsk, onOpenChat, onAnswer, onApprove }: {
  data: DataState; role: Role; asked?: ChatTurn[]; onAsk: () => void; onOpenChat?: () => void;
  onAnswer: (q: Question, text: string) => Promise<boolean>; onApprove: (d: KbDraft, title: string, body: string) => Promise<void>;
}) => {
  const [draft, setDraft] = useState<KbDraft | null>(null);
  const name = (id: string) => data.users.find((u) => u.id === id)?.name ?? "Стажёр";
  const incoming = data.questions.filter((q) => q.addressee_id === data.me.id && q.status === "Sent");
  const [filter, setFilter] = useState("");
  const [search, setSearch] = useState("");
  const all = data.questions.filter((q) => !incoming.includes(q));
  const others = all.filter((q) => (!filter || q.status === filter)
    && (!search.trim() || `${q.title ?? ""} ${q.text ?? ""} ${q.addressee}`.toLowerCase().includes(search.trim().toLowerCase())));
  const { t } = useTheme();
  const chip = (id: string, label: string) => <Box component="button" key={id || "all"} onClick={() => setFilter(id)} aria-pressed={filter === id} sx={{
    font: "inherit", fontSize: 14, border: "none", px: 1.5, py: 0.75, whiteSpace: "nowrap",
    bgcolor: filter === id ? t.inverse : t.surface, color: filter === id ? t.onInverse : t.text,
  }}>{label} <Box component="span" sx={{ opacity: 0.6 }}>{id ? all.filter((q) => q.status === id).length : all.length}</Box></Box>;
  const title = role === "intern" ? "Мои вопросы" : role === "buddy" ? "Вопросы" : "Вопросы мне";
  const context = incoming.length ? `Ждут твоего ответа: ${incoming.length}` : role === "intern" ? "Сначала Напарник ищет в базе, потом зовёт человека" : "Новых вопросов нет";
  // Одна CTA: в пустом состоянии — внутри Empty (фокус), когда список есть — в шапке (как у Авито)
  const askBtn = () => <Button variant="contained" onClick={onAsk}>Спросить Напарника</Button>;
  return <>
    <PageHead context={context} title={title} action={role === "intern" && (all.length > 0 || asked.length > 0) ? askBtn() : undefined} />
    <Stack gap={5}>
      {data.drafts.length > 0 && <Block title="Проверь, я занёс твой ответ в базу" aside={<AiTag />}>
        {data.drafts.map((d) => <Row key={d.id} title={d.title} sub="Черновик страницы — опубликую после твоего «Ок»" onClick={() => setDraft(d)}
          end={<Button variant="text" size="small" sx={{ minHeight: 36, bgcolor: "transparent", "&:hover": { bgcolor: t.surfaceHover } }}>Проверить</Button>} />)}
      </Block>}
      {incoming.length > 0 && <Block title="Ждут ответа">{incoming.map((q) => <Incoming key={q.id} q={q} from={`От ${name(q.asked_by)}`} onAnswer={onAnswer} />)}</Block>}
      <Block title={role === "buddy" ? "Маршрут вопросов стажёров" : all.length ? "Все вопросы" : asked.length ? "Вопросы людям" : "Пока пусто"}>
        {all.length > 0 && <Stack direction={{ xs: "column", sm: "row" }} gap={1} mb={1.5}>
          <TextField size="small" placeholder="Найти вопрос" value={search} onChange={(e) => setSearch(e.target.value)} aria-label="Поиск вопросов" sx={{ flex: 1 }} />
          <Stack direction="row" gap={0.75} flexWrap="wrap">{chip("", "Все")}{chip("Sent", "Ждут ответа")}{chip("Answered", "Есть ответ")}</Stack>
        </Stack>}
        {others.length ? others.map((q) => (
          <Box key={q.id} sx={{ py: 1.5, borderBottom: 1, borderColor: "divider", "&:last-child": { borderBottom: "none" } }}>
            <Stack direction="row" justifyContent="space-between" gap={2}>
              <Box minWidth={0}>
                <Typography fontWeight={500}>{questionTitle(q)}</Typography>
                <Typography variant="body2" color="text.secondary">{q.asked_by === data.me.id ? `Кому: ${q.addressee}` : `${name(q.asked_by)} → ${q.addressee}`}{q.urgent ? " · срочно" : ""}{answerDue(q) ? ` · ${answerDue(q)}` : ""}</Typography>
              </Box>
              <QuestionStatus q={q} />
            </Stack>
            {q.answer && <Box sx={{ mt: 1.5, pl: 2, borderLeft: 3, borderColor: "success.main" }}>
              <Typography variant="body2" fontWeight={600} mb={0.5}>Ответ · {q.addressee}</Typography>
              <Markdown text={q.answer} />
              {q.page_id && <Typography variant="caption" color="text.secondary">Ответ сохранён в базе знаний — следующему стажёру Напарник ответит сразу</Typography>}
            </Box>}
            {q.reason && !q.answer && <Typography variant="caption" color="text.secondary">Почему {q.addressee}: {q.reason}</Typography>}
          </Box>)) : <Empty
            text={role === "intern" ? "Вопросов людям пока нет. Сначала спроси Напарника — он поищет в базе." : "Здесь появятся вопросы, на которые ты ответил."}
            action={role === "intern" && !asked.length ? askBtn() : undefined}
          />}
      </Block>
      {asked.length > 0 && <Block title="Спрашивал у Напарника" aside={<AiTag />}>
        <AskedList asked={asked} onOpenChat={onOpenChat} />
      </Block>}
    </Stack>
    <DraftDialog draft={draft} folder={data.folders.find((f) => f.id === draft?.folder)?.title} onClose={() => setDraft(null)}
      onApprove={async (d, ti, b) => { await onApprove(d, ti, b); setDraft(null); }} />
  </>;
};
