// Вопросы людям. Адресат отвечает прямо здесь (US-10), может назначить встречу (US-14), сказать «не ко мне» или
// переадресовать (BR-17); автор проверяет страницу в базе перед публикацией (BR-30).
import { useState } from "react";
import { SearchRounded } from "@mui/icons-material";
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, InputAdornment, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import { api, type ChatTurn, type KbDraft, type Meeting, type Question } from "@/shared/api";
import { answerDue, type DataState, type Role } from "@/shared/lib";
import { brand } from "@/shared/theme";
import { AiTag, Block, DialogClose, dialogTitleSx, Empty, PageHead, Row, StatusTag } from "@/shared/ui";
import { Markdown } from "@/widgets/chat";
import { PersonDialog } from "@/widgets/person-picker";

type Action = (path: string, method: string, body: unknown, message: string) => Promise<boolean>;

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
      <DialogTitle sx={dialogTitleSx}>
        <Box component="span" sx={{ flex: 1, minWidth: 0 }}>Обсудить голосом</Box>
        <DialogClose onClose={onClose} />
      </DialogTitle>
      <DialogContent>
        <Typography color="text.secondary" mb={2}>Выбери время на 30 минут — я подобрал, когда свободны оба. Встречу стоит записать: из записи я соберу ответ для базы знаний.</Typography>
        {error && <Typography color="error.main" role="alert">{error}</Typography>}
        {!slots && !error && <Typography color="text.secondary">Ищу свободное время…</Typography>}
        <Stack gap={1}>{slots?.map((x) => <Button key={x} variant="outlined" onClick={() => onPick(x)} sx={{ justifyContent: "flex-start" }}>{when(x)}</Button>)}</Stack>
        {slots && !slots.length && <Typography color="text.secondary">Общего свободного времени в ближайшие дни нет — лучше ответить текстом.</Typography>}
      </DialogContent>
      <DialogActions sx={{ p: 2 }}><Button variant="text" onClick={onClose}>Отмена</Button></DialogActions>
    </Dialog>
  );
};

const questionTitle = (q: Question) => (q.text_hidden ? "Текст скрыт настройками стажёра" : q.title || q.text || "Вопрос");

/** Заголовок с заглавной — список не «пляшет» из‑за разного регистра. */
const displayTitle = (q: Question) => {
  const raw = questionTitle(q);
  return raw ? raw.charAt(0).toUpperCase() + raw.slice(1) : raw;
};

/** Статус вопроса; просроченный (SC-23) — красная плашка «Просрочен» вместо «Ждёт ответа». */
const QuestionStatus = ({ q }: { q: Question }) => q.status === "Sent" && q.overdue ? <StatusTag status="Overdue" label="Просрочен" /> : <StatusTag status={q.status} />;

const Incoming = ({ q, from, meeting, onAnswer, onMeeting, onFinish, onLeaveOpen, onNotMine, notMineLabel = "Не ко мне" }: {
  q: Question; from: string; meeting?: Meeting; onAnswer: (q: Question, text: string) => Promise<boolean>;
  onMeeting: (q: Question) => void; onFinish: (m: Meeting, recorded: boolean) => void; onLeaveOpen: (q: Question) => void;
  onNotMine?: (q: Question) => void; notMineLabel?: string;
}) => {
  const { t, space, stroke, type, size } = useTheme();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const scheduled = q.status === "MeetingScheduled" && meeting;
  const noRecording = q.status === "NoRecording";
  const actionBtn = {
    minHeight: { xs: size.control, sm: size.controlSm },
    width: { xs: "100%", sm: "auto" } as const,
  };
  return (
    <Box sx={{
      py: `${space[5]}px`,
      borderBottom: `${stroke.hairline}px solid ${t.line}`,
      "&:first-of-type": { pt: 0 },
      "&:last-child": { borderBottom: "none", pb: 0 },
    }}>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        gap={`${space[3]}px`}
        mb={`${space[3]}px`}
      >
        <Typography variant="body2" color="text.secondary" sx={{ minWidth: 0, overflowWrap: "anywhere" }}>
          {from}{q.urgent ? " · срочно" : ""}{answerDue(q) ? ` · ${answerDue(q)}` : ""}
        </Typography>
        <QuestionStatus q={q} />
      </Stack>

      {/* Сообщение стажёра: цветной кубик как у StatusTag (ok/bad/wait), без линий и подложек. */}
      <Stack direction="row" alignItems="flex-start" gap={`${space[3]}px`} mb={`${space[4]}px`}>
        <Box
          aria-hidden
          sx={{
            width: 8,
            height: 8,
            flex: "none",
            mt: "7px",
            bgcolor: q.overdue ? brand.red : brand.blue,
          }}
        />
        <Box minWidth={0} flex={1}>
          <Typography
            variant="h3"
            component="h3"
            sx={{
              overflowWrap: "anywhere",
              fontSize: { xs: type.size.lg, md: type.size.h3 },
              fontWeight: type.weight.medium,
              lineHeight: type.line.snug,
            }}
          >
            {displayTitle(q)}
          </Typography>
          {q.text && q.text !== q.title && (
            <Box sx={{ color: "text.secondary", mt: `${space[2]}px` }}><Markdown text={q.text} /></Box>
          )}
        </Box>
      </Stack>

      {q.hint && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: `${space[3]}px` }}>
          {q.hint}
        </Typography>
      )}

      {scheduled ? (
        <Stack gap={`${space[3]}px`} mt={`${space[2]}px`}>
          <Typography fontWeight={type.weight.semibold}>Встреча: {when(meeting.start)}</Typography>
          <Typography variant="body2" color="text.secondary">
            Не забудь включить запись — я напомню за 5 минут. После встречи отметь, как прошло:
          </Typography>
          <Stack direction={{ xs: "column", sm: "row" }} gap={`${space[2]}px`}>
            <Button variant="contained" onClick={() => onFinish(meeting, true)} sx={actionBtn}>Встреча записана — собрать ответ</Button>
            <Button variant="text" onClick={() => onFinish(meeting, false)} sx={actionBtn}>Записи не было</Button>
          </Stack>
          <Typography variant="caption" color="text.secondary">
            В Авито это придёт от ВКС само; в демо отмечаешь ты. Расшифровка — демо-данные.
          </Typography>
        </Stack>
      ) : (
        <Stack gap={`${space[3]}px`} mt={`${space[3]}px`}>
          {noRecording && (
            <Typography variant="body2" color="text.secondary">
              Записи встречи нет — ответ не попадёт в базу. Ответишь текстом? Пары строк хватит.
            </Typography>
          )}
          <Box>
            <Typography
              component="label"
              htmlFor={`answer-${q.id}`}
              variant="body2"
              fontWeight={type.weight.semibold}
              sx={{ display: "block", mb: `${space[2]}px` }}
            >
              Твой ответ
            </Typography>
            <TextField
              id={`answer-${q.id}`}
              fullWidth
              multiline
              minRows={3}
              placeholder="Напиши ответ своими словами"
              value={text}
              onChange={(e) => setText(e.target.value)}
              // Единственный белый акцент — зона ввода на серой плашке.
              sx={{ "& .MuiInputBase-root": { bgcolor: t.bg } }}
            />
          </Box>
          <Typography variant="body2" color="text.secondary">
            Ответ сразу увидит стажёр, а я оформлю его страницей в базе — опубликую после твоего «Ок».
          </Typography>
          <Stack direction={{ xs: "column", sm: "row" }} gap={`${space[2]}px`} alignItems={{ sm: "center" }} flexWrap="wrap">
            <Button
              variant="contained"
              disabled={!text.trim() || busy}
              sx={actionBtn}
              onClick={async () => { setBusy(true); if (await onAnswer(q, text)) setText(""); setBusy(false); }}
            >
              {busy ? "Отправляю…" : "Ответить"}
            </Button>
            {noRecording
              ? <Button variant="text" onClick={() => onLeaveOpen(q)} sx={actionBtn}>Нет, оставить открытым</Button>
              : <Button variant="text" onClick={() => onMeeting(q)} sx={actionBtn}>Проще голосом — назначить встречу</Button>}
            {!noRecording && onNotMine && (
              <Button variant="text" onClick={() => onNotMine(q)} sx={actionBtn}>{notMineLabel}</Button>
            )}
          </Stack>
        </Stack>
      )}
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
      <DialogTitle sx={dialogTitleSx}>
        <Box component="span" sx={{ flex: 1, minWidth: 0 }}>Проверь страницу перед публикацией</Box>
        <DialogClose onClose={onClose} />
      </DialogTitle>
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

/** Точка статуса — единственный цветной акцент в строке. */
const askedDot = (x: ChatTurn) => {
  const drafts = x.reply.drafts;
  if (drafts.some((d) => d.status === "confirmed")) return brand.green;
  if (drafts.some((d) => !d.status || d.status === "pending")) return brand.amber;
  if (x.reply.sources.length) return brand.blue;
  if (x.reply.degraded) return brand.amber;
  return brand.violet;
};

/** Вопросы Напарнику — сохраняются на сервере, новые сверху; ответ раскрывается по нажатию. */
export const AskedList = ({ asked, onOpenChat }: { asked: ChatTurn[]; onOpenChat?: () => void }) => {
  const [open, setOpen] = useState("");
  const { t, space, stroke, type, size } = useTheme();
  return <>
    {[...asked].reverse().map((x) => {
      const expanded = open === x.id;
      return (
        <Box
          key={x.id}
          sx={{
            py: `${space[4]}px`,
            borderBottom: `${stroke.hairline}px solid ${t.line}`,
            "&:last-child": { borderBottom: "none" },
          }}
        >
          <Box
            component="button"
            type="button"
            onClick={() => setOpen(expanded ? "" : x.id)}
            aria-expanded={expanded}
            sx={{
              all: "unset",
              boxSizing: "border-box",
              cursor: "pointer",
              display: "block",
              width: "100%",
              "&:focus-visible": { outline: `${stroke.thick}px solid ${t.link}`, outlineOffset: 2 },
            }}
          >
            <Stack direction="row" alignItems="flex-start" gap={`${space[3]}px`}>
              <Box aria-hidden sx={{ width: 8, height: 8, mt: "7px", flex: "none", bgcolor: askedDot(x) }} />
              <Box minWidth={0} flex={1}>
                <Typography fontWeight={type.weight.medium} sx={{ overflowWrap: "anywhere" }}>{x.question}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: `${space[1]}px` }}>
                  {askedSummary(x)}{askedAt(x.at) ? ` · ${askedAt(x.at)}` : ""}
                </Typography>
              </Box>
            </Stack>
          </Box>
          {expanded && (
            <Stack gap={`${space[3]}px`} sx={{ mt: `${space[3]}px`, ml: `${space[3] + 8}px` }}>
              <Markdown text={x.reply.text} />
              {x.reply.sources.map((s) => (
                <Typography key={s.page_id} variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  Источник: {s.title} · {s.author} · {s.updated_at}{s.label ? ` · ${s.label}` : ""}
                </Typography>
              ))}
              {onOpenChat && (
                <Button
                  variant="text"
                  size="small"
                  onClick={onOpenChat}
                  sx={{
                    alignSelf: "flex-start",
                    minHeight: size.controlSm,
                    px: `${space[3]}px`,
                    bgcolor: t.bg,
                    "&:hover": { bgcolor: t.surfaceHover },
                  }}
                >
                  Открыть чат
                </Button>
              )}
            </Stack>
          )}
        </Box>
      );
    })}
  </>;
};

export const QuestionsPage = ({ data, role, login, asked = [], onAsk, onOpenChat, onAnswer, onApprove, onAction }: {
  data: DataState; role: Role; login: string; asked?: ChatTurn[]; onAsk: () => void; onOpenChat?: () => void;
  onAnswer: (q: Question, text: string) => Promise<boolean>; onApprove: (d: KbDraft, title: string, body: string) => Promise<void>;
  onAction: Action;
}) => {
  const [draft, setDraft] = useState<KbDraft | null>(null);
  const [slotFor, setSlotFor] = useState<Question | null>(null);
  const [rerouting, setRerouting] = useState<{ q: Question; kind: "not-mine" | "redirect" } | null>(null);
  const colleagues = data.users.filter((u) => u.id !== data.me.id && !u.roles.includes("intern"));
  const myInterns = new Set(data.me.interns.map((i) => i.id));
  const name = (id: string) => data.users.find((u) => u.id === id)?.name ?? "Стажёр";
  const routeText = (q: Question) => (q.route?.length ? [q.route[0].from, ...q.route.map((r) => r.to)].map((id) => name(id).split(" ")[0]).join(" → ") : "");
  const meetingOf = (q: Question) => data.meetings.find((m) => m.question_id === q.id);
  const incoming = data.questions.filter((q) => q.addressee_id === data.me.id && ["Sent", "MeetingScheduled", "NoRecording"].includes(q.status));
  const [filter, setFilter] = useState("");
  const [search, setSearch] = useState("");
  const all = data.questions.filter((q) => !incoming.includes(q));
  const others = all.filter((q) => (!filter || q.status === filter)
    && (!search.trim() || `${q.title ?? ""} ${q.text ?? ""} ${q.addressee}`.toLowerCase().includes(search.trim().toLowerCase())));
  const { t, space, stroke, type, size } = useTheme();
  // Фильтры как в «Базе знаний»: inverse / soft surface, без лишней рамки.
  const chip = (id: string, label: string) => {
    const active = filter === id;
    const count = id ? all.filter((q) => q.status === id).length : all.length;
    return (
      <Box component="button" key={id || "all"} type="button" onClick={() => setFilter(id)} aria-pressed={active} sx={{
        font: "inherit", fontSize: type.size.md, fontWeight: type.weight.semibold, border: "none",
        minHeight: size.controlSm, px: `${space[3]}px`, py: `${space[2]}px`, whiteSpace: "nowrap",
        display: "inline-flex", alignItems: "center", gap: `${space[1] + 2}px`,
        bgcolor: active ? t.inverse : t.bg, color: active ? t.onInverse : t.text,
        "&:hover": { bgcolor: active ? t.inverseHover : t.surfaceHover },
      }}>
        {label} <Box component="span" sx={{ opacity: 0.65 }}>{count}</Box>
      </Box>
    );
  };
  // У стажёра как в навигации — «Вопросы» (не «Мои вопросы»).
  const title = role === "intern" ? "Вопросы" : role === "buddy" ? "Вопросы" : "Вопросы мне";
  const context = incoming.length ? `Ждут твоего ответа: ${incoming.length}` : role === "intern" ? "Сначала Напарник ищет в базе, потом зовёт человека" : "Новых вопросов нет";
  // Одна CTA: в пустом состоянии — внутри Empty (фокус), когда список есть — в шапке (как у Авито)
  const askBtn = () => <Button variant="contained" onClick={onAsk}>Спросить Напарника</Button>;
  const questionMeta = (q: Question) => {
    const who = q.asked_by === data.me.id ? `Кому: ${q.addressee}` : `${name(q.asked_by)} → ${q.addressee}`;
    const bits = [who];
    if (q.urgent) bits.push("срочно");
    if (answerDue(q)) bits.push(answerDue(q)!);
    if (q.status === "MeetingScheduled" && meetingOf(q)) bits.push(`встреча ${when(meetingOf(q)!.start)}`);
    if (q.reason && !q.answer) bits.push(`почему: ${q.reason}`);
    return bits.join(" · ");
  };
  return <>
    <PageHead context={context} title={title} action={role === "intern" && (all.length > 0 || asked.length > 0) ? askBtn() : undefined} />
    <Stack gap={{ xs: `${space[4]}px`, md: `${space[8]}px` }}>
      {data.drafts.length > 0 && <Block title="Проверь, я занёс твой ответ в базу" aside={<AiTag />}>
        {data.drafts.map((d) => <Row key={d.id} title={d.title} sub="Черновик страницы — опубликую после твоего «Ок»" onClick={() => setDraft(d)}
          end={<Button variant="text" size="small" sx={{ minHeight: size.controlSm, px: `${space[3]}px`, bgcolor: "transparent", "&:hover": { bgcolor: t.surfaceHover } }}>Проверить</Button>}
          sx={{ mx: `-${space[4]}px` }} />)}
      </Block>}
      {incoming.length > 0 && <Block title="Ждут ответа">{incoming.map((q) => <Incoming key={q.id} q={q} from={`От ${name(q.asked_by)}`} meeting={meetingOf(q)} onAnswer={onAnswer}
        onMeeting={setSlotFor}
        onFinish={(m, recorded) => void onAction(`/api/meetings/${m.id}/finish`, "POST", { recorded },
          recorded ? "Собрал ответ со встречи — проверь черновик страницы" : "Понял, записи нет. Можешь ответить текстом")}
        onLeaveOpen={(x) => void onAction(`/api/questions/${x.id}/leave-open`, "POST", {}, "Вопрос передан владельцу базы")}
        notMineLabel={q.hint && myInterns.has(q.asked_by) ? "Переадресовать" : "Не ко мне"}
        onNotMine={(x) => setRerouting({ q: x, kind: q.hint && myInterns.has(q.asked_by) ? "redirect" : "not-mine" })} />)}</Block>}
      <Block title={role === "buddy" ? "Маршрут вопросов стажёров" : all.length ? "Все вопросы" : asked.length ? "Вопросы людям" : "Пока пусто"}>
        {all.length > 0 && (
          <Stack gap={`${space[3]}px`} mb={`${space[4]}px`}>
            <TextField
              fullWidth
              size="small"
              placeholder="Найти вопрос"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Поиск вопросов"
              slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchRounded sx={{ color: t.muted }} /></InputAdornment> } }}
              sx={{ "& .MuiInputBase-root": { bgcolor: t.bg, minHeight: size.control } }}
            />
            <Stack direction="row" gap={`${space[2]}px`} flexWrap="wrap">
              {chip("", "Все")}
              {chip("Sent", "Ждут ответа")}
              {chip("Answered", "Есть ответ")}
            </Stack>
          </Stack>
        )}
        {others.length ? others.map((q) => (
          <Box
            key={q.id}
            sx={{
              py: `${space[4]}px`,
              borderBottom: `${stroke.hairline}px solid ${t.line}`,
              "&:last-child": { borderBottom: "none" },
            }}
          >
            <Stack
              direction={{ xs: "column", sm: "row" }}
              alignItems={{ xs: "stretch", sm: "flex-start" }}
              gap={`${space[3]}px`}
            >
              <Box flex={1} minWidth={0}>
                <Typography fontWeight={type.weight.medium} sx={{ overflowWrap: "anywhere" }}>{displayTitle(q)}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: `${space[1]}px` }}>{questionMeta(q)}</Typography>
                {routeText(q) && (
                  <Typography variant="caption" component="div" color="text.secondary" sx={{ mt: `${space[1]}px` }}>
                    Маршрут: {routeText(q)}
                  </Typography>
                )}
              </Box>
              <Stack
                direction={{ xs: "row", sm: "column" }}
                alignItems={{ xs: "center", sm: "flex-end" }}
                justifyContent={{ xs: "space-between", sm: "flex-start" }}
                gap={`${space[2]}px`}
                flex={{ xs: "none", sm: "none" }}
                pt={{ sm: `${space[1] / 2}px` }}
              >
                <QuestionStatus q={q} />
                {role === "buddy" && myInterns.has(q.asked_by) && ["Sent", "NoRecording"].includes(q.status) && q.addressee_id !== data.me.id && (
                  <Button
                    variant="text"
                    size="small"
                    sx={{ minHeight: size.controlSm, px: `${space[3]}px`, width: { xs: "auto", sm: "auto" } }}
                    onClick={() => setRerouting({ q, kind: "redirect" })}
                  >
                    Переадресовать
                  </Button>
                )}
              </Stack>
            </Stack>
            {q.answer && (
              <Box sx={{
                mt: `${space[3]}px`,
                p: `${space[3]}px`,
                bgcolor: t.okSoft,
                borderLeft: `${stroke.accent}px solid`,
                borderLeftColor: "success.main",
              }}>
                <Typography variant="body2" fontWeight={type.weight.semibold} mb={`${space[1]}px`}>Ответ · {q.addressee}</Typography>
                <Markdown text={q.answer} />
                {q.page_id && (
                  <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: `${space[2]}px` }}>
                    Ответ сохранён в базе знаний — следующему стажёру Напарник ответит сразу
                  </Typography>
                )}
              </Box>
            )}
            {meetingOf(q)?.recording_url && (
              <Button
                component="a"
                href={meetingOf(q)!.recording_url}
                target="_blank"
                rel="noopener noreferrer"
                variant="text"
                sx={{ mt: `${space[2]}px`, minHeight: size.controlSm, px: `${space[3]}px` }}
              >
                Открыть запись встречи
              </Button>
            )}
          </Box>
        )) : <Empty
            text={role === "intern" ? "Вопросов людям пока нет. Сначала спроси Напарника — он поищет в базе." : "Здесь появятся вопросы, на которые ты ответил."}
            action={role === "intern" && !asked.length ? askBtn() : undefined}
          />}
      </Block>
      {asked.length > 0 && <Block title="Спрашивал у Напарника">
        <AskedList asked={asked} onOpenChat={onOpenChat} />
      </Block>}
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
