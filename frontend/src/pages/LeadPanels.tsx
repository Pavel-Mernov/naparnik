// Руководителю (SCR-L1, SCR-L3): польза Напарника в цифрах, нагрузка бадди и экспертов (M-05, M-06),
// поддержка базы — открытые вопросы, черновики без «Ок», «устарело», дубли (SC-21, SC-22).
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Stack, Typography, useTheme } from "@mui/material";
import { api, type User } from "../api";
import { Block, Empty, Row } from "../components/ui";
import { PersonDialog } from "../components/PersonDialog";

type LoadRow = { id: string; name: string; role: string; received: number; answered: number; meetings: number; pages: number; minutes: number };
type Metrics = { kb_answers: number; questions_to_people: number; kb_share: number; median_answer_minutes: number; pages_published: number; csat: number; load: LoadRow[] };
type Maintenance = {
  open_questions: Array<{ id: string; title: string; asked_by: string; reason: string }>;
  drafts: Array<{ id: string; title: string; author_id: string; since: string }>;
  stale: Array<{ page_id: string; title: string; author_id: string; marks: number }>;
  duplicates: Array<{ id: string; titles: string[]; proposed_title: string; note?: string; decision?: string }>;
  next_check: string;
};

const pct = (x: number) => `${Math.round(x * 100)}%`;
const mins = (m: number) => (m < 60 ? `${Math.round(m)} мин` : `${Math.floor(m / 60)} ч ${Math.round(m % 60)} мин`);

export const LeadPanels = ({ login, users, onAction }: {
  login: string; users: User[]; onAction: (path: string, method: string, body: unknown, message: string) => Promise<boolean>;
}) => {
  const { t } = useTheme();
  const [m, setM] = useState<Metrics | null>(null);
  const [kb, setKb] = useState<Maintenance | null>(null);
  const [assigning, setAssigning] = useState<string | null>(null);
  const name = (id: string) => users.find((u) => u.id === id)?.name ?? id;
  const reload = useCallback(() => {
    api<Metrics>(login, "/api/metrics").then(setM).catch(() => undefined);
    api<Maintenance>(login, "/api/kb/maintenance").then(setKb).catch(() => undefined);
  }, [login]);
  useEffect(reload, [reload]);
  const act = async (path: string, body: unknown, msg: string) => { if (await onAction(path, "POST", body, msg)) reload(); };

  const tile = (value: string, label: string, hint: string) => (
    <Box sx={{ bgcolor: t.surface, p: 2 }}>
      <Typography sx={{ fontSize: 30, fontWeight: 300, lineHeight: 1.1 }}>{value}</Typography>
      <Typography variant="body2">{label}</Typography>
      <Typography variant="caption" color="text.secondary">{hint}</Typography>
    </Box>
  );
  const pending = kb ? kb.open_questions.length + kb.drafts.length + kb.stale.length + kb.duplicates.filter((d) => !d.decision).length : 0;

  return <Stack gap={5} mt={5}>
    <Block title="Польза Напарника">
      {m ? <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 1.5 }}>
        {tile(pct(m.kb_share), "вопросов закрыла база", `${m.kb_answers} из базы, ${m.questions_to_people} — людям`)}
        {tile(m.median_answer_minutes ? mins(m.median_answer_minutes) : "—", "медиана ответа человека", "от вопроса до ответа")}
        {tile(String(m.pages_published), "новых страниц в базе", "ответы людей стали знанием")}
        {tile(m.csat ? pct(m.csat) : "—", "ответов «помогло»", "оценки стажёров")}
      </Box> : <Typography color="text.secondary">Считаю…</Typography>}
    </Block>

    <Block title="Нагрузка бадди и экспертов" aside={<Typography variant="body2" color="text.secondary">оценка: ответ ≈ 10 мин, встреча ≈ 30 мин</Typography>}>
      {m?.load.length ? m.load.map((r) => <Row key={r.id} title={`${r.name} · ${r.role}`}
        sub={`вопросов: ${r.received} · ответил(а): ${r.answered} · встреч: ${r.meetings} · страниц в базе: ${r.pages}`}
        end={<Typography fontWeight={600}>{mins(r.minutes)}</Typography>} />) : <Empty text="Нагрузка появится, когда стажёры начнут задавать вопросы людям." />}
    </Block>

    <Block title="Поддержка базы знаний" aside={kb && <Typography variant="body2" color="text.secondary">{pending ? `требует внимания: ${pending}` : "всё в порядке"} · плановая проверка {kb.next_check}</Typography>}>
      {!kb ? <Typography color="text.secondary">Загружаю…</Typography> : <Stack gap={3}>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Открытые вопросы</Typography>
          {kb.open_questions.length ? kb.open_questions.map((q) => <Row key={q.id} title={q.title} sub={`${name(q.asked_by)} · ${q.reason}`}
            end={<Button variant="contained" size="small" sx={{ minHeight: 36 }} onClick={() => setAssigning(q.id)}>Назначить</Button>} />)
            : <Typography variant="body2" color="text.secondary">Открытых вопросов нет.</Typography>}
        </Box>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Дубли от плановой проверки</Typography>
          {kb.duplicates.map((d) => <Row key={d.id} title={d.titles.join("  ≈  ")}
            sub={d.decision ? (d.decision === "merge" ? `Объединено${d.proposed_title ? `: «${d.proposed_title}»` : ""}` : "Отмечено: не дубль")
              : d.proposed_title ? `Предлагаю общий заголовок: «${d.proposed_title}»` : `Похоже, это разные темы: ${d.note ?? "проверь сам"}`}
            end={!d.decision && <>
              <Button variant="contained" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/duplicates/${d.id}`, { decision: "merge" }, "Страницы объединены, авторы уведомлены")}>Объединить</Button>
              <Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/duplicates/${d.id}`, { decision: "not_duplicate" }, "Отмечено: не дубль")}>Не дубль</Button>
            </>} />)}
        </Box>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Черновики без «Ок» автора</Typography>
          {kb.drafts.length ? kb.drafts.map((d) => <Row key={d.id} title={d.title} sub={`${name(d.author_id)} · с ${d.since}`}
            end={<Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/drafts/${d.id}/remind`, {}, "Напомнил автору")}>Напомнить</Button>} />)
            : <Typography variant="body2" color="text.secondary">Все черновики подтверждены.</Typography>}
        </Box>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Отмечено «устарело»</Typography>
          {kb.stale.length ? kb.stale.map((p) => <Row key={p.page_id} title={p.title} sub={`${name(p.author_id)} · отметок: ${p.marks} · автор уже получил уведомление`} />)
            : <Typography variant="body2" color="text.secondary">Стажёры не отмечали страницы устаревшими.</Typography>}
        </Box>
      </Stack>}
    </Block>

    <PersonDialog open={Boolean(assigning)} title="Кто ответит?" confirm="Назначить" allowAuto={false}
      text="Вопрос уйдёт выбранному сотруднику, стажёр увидит, что им занялись."
      people={users.filter((u) => !u.roles.includes("intern") && u.login !== login)}
      onClose={() => setAssigning(null)}
      onPick={(to) => { const id = assigning; setAssigning(null); if (id) void act(`/api/questions/${id}/assign`, { to }, "Вопрос назначен"); }} />
  </Stack>;
};
