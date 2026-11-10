// Вопросы людям. Адресат отвечает прямо здесь (US-10), автор проверяет страницу в базе перед публикацией (BR-30).
import { useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import type { KbDraft, Question } from "../api";
import type { DataState, Role } from "../state";
import { AiTag, Block, Empty, PageHead, Row, StatusTag } from "../components/ui";
import { Markdown } from "../chat";

const questionTitle = (q: Question) => (q.text_hidden ? "Текст скрыт настройками стажёра" : q.title || q.text || "Вопрос");

const Incoming = ({ q, from, onAnswer }: { q: Question; from: string; onAnswer: (q: Question, text: string) => Promise<boolean> }) => {
  const { t } = useTheme();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <Box sx={{ border: `2px solid ${t.line}`, p: { xs: 2, md: 2.5 }, mb: 2 }}>
      <Stack direction="row" justifyContent="space-between" gap={2} mb={1}>
        <Typography variant="body2" color="text.secondary">{from}{q.urgent ? " · срочно" : ""}</Typography>
        <StatusTag status={q.status} />
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

export const QuestionsPage = ({ data, role, onAsk, onAnswer, onApprove }: {
  data: DataState; role: Role; onAsk: () => void;
  onAnswer: (q: Question, text: string) => Promise<boolean>; onApprove: (d: KbDraft, title: string, body: string) => Promise<void>;
}) => {
  const [draft, setDraft] = useState<KbDraft | null>(null);
  const name = (id: string) => data.users.find((u) => u.id === id)?.name ?? "Стажёр";
  const incoming = data.questions.filter((q) => q.addressee_id === data.me.id && q.status === "Sent");
  const others = data.questions.filter((q) => !incoming.includes(q));
  const title = role === "intern" ? "Мои вопросы" : role === "buddy" ? "Вопросы" : "Вопросы мне";
  const context = incoming.length ? `Ждут твоего ответа: ${incoming.length}` : role === "intern" ? "Сначала Напарник ищет в базе, потом зовёт человека" : "Новых вопросов нет";
  return <>
    <PageHead context={context} title={title} action={role === "intern" && <Button variant="contained" onClick={onAsk}>Задать вопрос</Button>} />
    <Stack gap={5}>
      {data.drafts.length > 0 && <Block title="Проверь, я занёс твой ответ в базу" aside={<AiTag />}>
        {data.drafts.map((d) => <Row key={d.id} title={d.title} sub="Черновик страницы — опубликую после твоего «Ок»" onClick={() => setDraft(d)}
          end={<Button variant="text" size="small" sx={{ minHeight: 36 }}>Проверить</Button>} />)}
      </Block>}
      {incoming.length > 0 && <Block title="Ждут ответа">{incoming.map((q) => <Incoming key={q.id} q={q} from={`От ${name(q.asked_by)}`} onAnswer={onAnswer} />)}</Block>}
      <Block title={role === "buddy" ? "Маршрут вопросов стажёров" : "Все вопросы"}>
        {others.length ? others.map((q) => (
          <Box key={q.id} sx={{ py: 1.5, borderBottom: 1, borderColor: "divider" }}>
            <Stack direction="row" justifyContent="space-between" gap={2}>
              <Box minWidth={0}>
                <Typography fontWeight={500}>{questionTitle(q)}</Typography>
                <Typography variant="body2" color="text.secondary">{q.asked_by === data.me.id ? `Кому: ${q.addressee}` : `${name(q.asked_by)} → ${q.addressee}`}{q.urgent ? " · срочно" : ""}</Typography>
              </Box>
              <StatusTag status={q.status} />
            </Stack>
            {q.answer && <Box sx={{ mt: 1.5, pl: 2, borderLeft: 3, borderColor: "success.main" }}>
              <Typography variant="body2" fontWeight={600} mb={0.5}>Ответ · {q.addressee}</Typography>
              <Markdown text={q.answer} />
            </Box>}
          </Box>)) : <Empty text={role === "intern" ? "Вопросов людям пока нет. Спроси Напарника — он сначала поищет в базе." : "Здесь появятся вопросы, на которые ты ответил."} action={role === "intern" ? <Button variant="text" onClick={onAsk}>Спросить</Button> : undefined} />}
      </Block>
    </Stack>
    <DraftDialog draft={draft} folder={data.folders.find((f) => f.id === draft?.folder)?.title} onClose={() => setDraft(null)}
      onApprove={async (d, ti, b) => { await onApprove(d, ti, b); setDraft(null); }} />
  </>;
};
