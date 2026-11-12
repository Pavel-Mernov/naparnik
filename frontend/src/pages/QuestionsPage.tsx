// Вопросы людям. Адресат отвечает прямо здесь (US-10), автор проверяет страницу в базе перед публикацией (BR-30).
import { useState } from "react";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import { api, type KbDraft, type Meeting, type Question } from "../api";
import type { DataState, Role } from "../state";
import { AiTag, Block, Empty, PageHead, Row, StatusTag } from "../components/ui";
import { Markdown } from "../chat";
import { PersonDialog } from "../components/PersonDialog";

const questionTitle = (q: Question) => (q.text_hidden ? "Текст скрыт настройками стажёра" : q.title || q.text || "Вопрос");

const when = (iso: string) => {
  const d = new Date(iso);
  return `${d.toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long" })}, ${d.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })}`;
};

/** Выбор слота встречи (US-14): 3 варианта в рабочие часы обоих (BR-41). */
const SlotDialog = ({ q, login, onClose, onPick }: { q: Question | null; login: string; onClose: () => void; onPick: (slot: string) => void }) => {
  const [slots, setSlots] = useState<string[] | null>(null);
  const [error, setError] = useState("");
  const [seen, setSeen] = useState<string | null>(null);
  if (q && seen !== q.id) {
    setSeen(q.id); setSlots(null); setError("");
    api<{ slots: string[] }>(login, `/api/questions/${q.id}/meeting-slots`).then((r) => setSlots(r.slots)).catch((e) => setError(e instanceof Error ? e.message : "Не получилось"));
  }
  return (
    <Dialog open={Boolean(q)} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Обсудить голосом</DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>Выбери время на 30 минут — я подобрал, когда свободны оба. Встречу стоит записать: из записи я соберу ответ для базы знаний.</Typography>
        {error && <Typography color="error.main" role="alert">{error}</Typography>}
        {!slots && !error && <Typography color="text.secondary">Ищу свободное время…</Typography>}
        <Stack gap={1}>{slots?.map((s) => <Button key={s} variant="outlined" onClick={() => onPick(s)} sx={{ justifyContent: "flex-start" }}>{when(s)}</Button>)}</Stack>
        {slots && !slots.length && <Typography color="text.secondary">Общего свободного времени в ближайшие дни нет — лучше ответить текстом.</Typography>}
      </DialogContent>
      <DialogActions sx={{ p: 2 }}><Button variant="text" onClick={onClose}>Отмена</Button></DialogActions>
    </Dialog>
  );
};

const Incoming = ({ q, from, meeting, onAnswer, onMeeting, onFinish, onLeaveOpen, onNotMine, notMineLabel = "Не ко мне" }: {
  q: Question; from: string; meeting?: Meeting; onAnswer: (q: Question, text: string) => Promise<boolean>;
  onMeeting: (q: Question) => void; onFinish: (m: Meeting, recorded: boolean) => void; onLeaveOpen: (q: Question) => void;
  onNotMine?: (q: Question) => void; notMineLabel?: string;
}) => {
  const { t } = useTheme();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const scheduled = q.status === "MeetingScheduled" && meeting;
  const noRecording = q.status === "NoRecording";
  return (
    <Box sx={{ border: `2px solid ${t.line}`, p: { xs: 2, md: 2.5 }, mb: 2 }}>
      <Stack direction="row" justifyContent="space-between" gap={2} mb={1}>
        <Typography variant="body2" color="text.secondary">{from}{q.urgent ? " · срочно" : ""}</Typography>
        <StatusTag status={q.status} />
      </Stack>
      <Typography variant="h3" component="h3" mb={1}>{questionTitle(q)}</Typography>
      {q.hint && <Box sx={{ bgcolor: t.aiSoft, color: "text.primary", px: 1.5, py: 1, mb: 1.5, fontSize: 14 }}>💡 {q.hint}</Box>}
      {q.text && q.text !== q.title && <Box sx={{ color: "text.secondary", mb: 2 }}><Markdown text={q.text} /></Box>}
      {scheduled ? <Box sx={{ bgcolor: t.surface, p: 2 }}>
        <Typography fontWeight={600}>📅 Встреча: {when(meeting.start)}</Typography>
        <Typography variant="body2" color="text.secondary" mb={1.5}>Не забудь включить запись — я напомню за 5 минут. После встречи отметь, как прошло:</Typography>
        <Stack direction={{ xs: "column", sm: "row" }} gap={1}>
          <Button variant="contained" onClick={() => onFinish(meeting, true)}>Встреча записана — собрать ответ</Button>
          <Button variant="text" onClick={() => onFinish(meeting, false)}>Записи не было</Button>
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>В Авито это придёт от ВКС само; в демо отмечаешь ты. Расшифровка — демо-данные.</Typography>
      </Box> : <>
        {noRecording && <Box sx={{ bgcolor: t.aiSoft, px: 1.5, py: 1, mb: 1.5 }}>🎙 Записи встречи нет — ответ не попадёт в базу. Ответишь текстом? Пары строк хватит.</Box>}
        <TextField fullWidth multiline minRows={3} label="Твой ответ" value={text} onChange={(e) => setText(e.target.value)}
          helperText="Ответ сразу увидит стажёр, а я оформлю его страницей в базе — опубликую после твоего «Ок»." />
        <Stack direction={{ xs: "column", sm: "row" }} gap={1} mt={1.5}>
          <Button variant="contained" disabled={!text.trim() || busy}
            onClick={async () => { setBusy(true); if (await onAnswer(q, text)) setText(""); setBusy(false); }}>
            {busy ? "Отправляю…" : "Ответить"}
          </Button>
          {noRecording
            ? <Button variant="text" onClick={() => onLeaveOpen(q)}>Нет, оставить открытым</Button>
            : <Button variant="text" onClick={() => onMeeting(q)}>Проще голосом — назначить встречу</Button>}
          {!noRecording && onNotMine && <Button variant="text" onClick={() => onNotMine(q)}>{notMineLabel}</Button>}
        </Stack>
      </>}
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
        <Typography color="text.secondary" mb={2}>Я оформил твой ответ страницей для базы знаний. Автор — ты{folder ? `, папка «${folder}»` : ""}. Можно поправить.</Typography>
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

export const QuestionsPage = ({ data, role, login, onAsk, onAnswer, onApprove, onAction }: {
  data: DataState; role: Role; login: string; onAsk: () => void;
  onAnswer: (q: Question, text: string) => Promise<boolean>; onApprove: (d: KbDraft, title: string, body: string) => Promise<void>;
  onAction: (path: string, method: string, body: unknown, message: string) => Promise<boolean>;
}) => {
  const [draft, setDraft] = useState<KbDraft | null>(null);
  const [slotFor, setSlotFor] = useState<Question | null>(null);
  const [rerouting, setRerouting] = useState<{ q: Question; kind: "not-mine" | "redirect" } | null>(null);
  const colleagues = data.users.filter((u) => u.id !== data.me.id && !u.roles.includes("intern"));
  const myInterns = new Set(data.me.interns.map((i) => i.id));
  const routeText = (q: Question) => (q.route?.length ? [q.route[0].from, ...q.route.map((r) => r.to)].map((id) => name(id).split(" ")[0]).join(" → ") : "");
  const meetingOf = (q: Question) => data.meetings.find((m) => m.question_id === q.id);
  const name = (id: string) => data.users.find((u) => u.id === id)?.name ?? "Стажёр";
  const incoming = data.questions.filter((q) => q.addressee_id === data.me.id && ["Sent", "MeetingScheduled", "NoRecording"].includes(q.status));
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
  return <>
    <PageHead context={context} title={title} action={role === "intern" && <Button variant="contained" onClick={onAsk}>Задать вопрос</Button>} />
    <Stack gap={5}>
      {data.drafts.length > 0 && <Block title="Проверь, я занёс твой ответ в базу" aside={<AiTag />}>
        {data.drafts.map((d) => <Row key={d.id} title={d.title} sub="Черновик страницы — опубликую после твоего «Ок»" onClick={() => setDraft(d)}
          end={<Button variant="text" size="small" sx={{ minHeight: 36 }}>Проверить</Button>} />)}
      </Block>}
      {incoming.length > 0 && <Block title="Ждут ответа">{incoming.map((q) => <Incoming key={q.id} q={q} from={`От ${name(q.asked_by)}`} meeting={meetingOf(q)} onAnswer={onAnswer}
        onMeeting={setSlotFor}
        onFinish={(m, recorded) => void onAction(`/api/meetings/${m.id}/finish`, "POST", { recorded },
          recorded ? "Собрал ответ со встречи — проверь черновик страницы" : "Понял, записи нет. Можешь ответить текстом")}
        onLeaveOpen={(x) => void onAction(`/api/questions/${x.id}/leave-open`, "POST", {}, "Вопрос передан владельцу базы")}
        notMineLabel={q.hint && myInterns.has(q.asked_by) ? "Переадресовать" : "Не ко мне"}
        onNotMine={(x) => setRerouting({ q: x, kind: q.hint && myInterns.has(q.asked_by) ? "redirect" : "not-mine" })} />)}</Block>}
      <Block title={role === "buddy" ? "Маршрут вопросов стажёров" : "Все вопросы"}>
        {all.length > 0 && <Stack direction={{ xs: "column", sm: "row" }} gap={1} mb={1.5}>
          <TextField size="small" placeholder="Найти вопрос" value={search} onChange={(e) => setSearch(e.target.value)} aria-label="Поиск вопросов" sx={{ flex: 1 }} />
          <Stack direction="row" gap={0.75} sx={{ overflowX: "auto" }}>{chip("", "Все")}{chip("Sent", "Ждут ответа")}{chip("Answered", "Есть ответ")}</Stack>
        </Stack>}
        {others.length ? others.map((q) => (
          <Box key={q.id} sx={{ py: 1.5, borderBottom: 1, borderColor: "divider" }}>
            <Stack direction="row" justifyContent="space-between" gap={2}>
              <Box minWidth={0}>
                <Typography fontWeight={500}>{questionTitle(q)}</Typography>
                <Typography variant="body2" color="text.secondary">{q.asked_by === data.me.id ? `Кому: ${q.addressee}` : `${name(q.asked_by)} → ${q.addressee}`}{q.urgent ? " · срочно" : ""}
                  {q.status === "MeetingScheduled" && meetingOf(q) ? ` · встреча ${when(meetingOf(q)!.start)}` : ""}</Typography>
              </Box>
              <Stack alignItems="flex-end" gap={0.75}>
                <StatusTag status={q.status} />
                {role === "buddy" && myInterns.has(q.asked_by) && ["Sent", "NoRecording"].includes(q.status) && q.addressee_id !== data.me.id &&
                  <Button variant="text" size="small" sx={{ minHeight: 32 }} onClick={() => setRerouting({ q, kind: "redirect" })}>Переадресовать</Button>}
              </Stack>
            </Stack>
            {routeText(q) && <Typography variant="caption" component="div" color="text.secondary">Маршрут: {routeText(q)}</Typography>}
            {q.answer && <Box sx={{ mt: 1.5, pl: 2, borderLeft: 3, borderColor: "success.main" }}>
              <Typography variant="body2" fontWeight={600} mb={0.5}>Ответ · {q.addressee}</Typography>
              <Markdown text={q.answer} />
              {q.page_id && <Typography variant="caption" color="text.secondary">Ответ сохранён в базе знаний — следующему стажёру Напарник ответит сразу</Typography>}
            </Box>}
            {meetingOf(q)?.recording_url && <Button component="a" href={meetingOf(q)!.recording_url} target="_blank" rel="noopener noreferrer" variant="text" sx={{ mt: 1 }}>
              Открыть запись встречи
            </Button>}
            {q.reason && !q.answer && <Typography variant="caption" component="div" color="text.secondary">Почему {q.addressee}: {q.reason}</Typography>}
          </Box>)) : <Empty text={role === "intern" ? "Вопросов людям пока нет. Спроси Напарника — он сначала поищет в базе." : "Здесь появятся вопросы, на которые ты ответил."} action={role === "intern" ? <Button variant="text" onClick={onAsk}>Спросить</Button> : undefined} />}
      </Block>
    </Stack>
    <PersonDialog open={Boolean(rerouting)} people={colleagues.filter((u) => u.id !== rerouting?.q.addressee_id)}
      title={rerouting?.kind === "redirect" ? "Переадресовать вопрос" : "Не ко мне"} allowAuto={rerouting?.kind === "not-mine"}
      text={rerouting?.kind === "redirect" ? "Напарник ошибся с адресатом? Выбери, кто ответит лучше. Стажёр и прежний адресат узнают об этом."
        : "Подскажи, кто ответит лучше, — или пусть Напарник подберёт следующего кандидата. Стажёр увидит, что вопрос передан."}
      confirm={rerouting?.kind === "redirect" ? "Переадресовать" : "Передать"} onClose={() => setRerouting(null)}
      onPick={(to) => { const r = rerouting; setRerouting(null); if (r) void onAction(`/api/questions/${r.q.id}/${r.kind}`, "POST", { to }, "Вопрос передан — стажёр в курсе"); }} />
    <SlotDialog q={slotFor} login={login} onClose={() => setSlotFor(null)}
      onPick={(slot) => { const q = slotFor; setSlotFor(null); if (q) void onAction(`/api/questions/${q.id}/meeting`, "POST", { slot }, "Встреча назначена — стажёр получил приглашение"); }} />
    <DraftDialog draft={draft} folder={data.folders.find((f) => f.id === draft?.folder)?.title} onClose={() => setDraft(null)}
      onApprove={async (d, ti, b) => { await onApprove(d, ti, b); setDraft(null); }} />
  </>;
};
