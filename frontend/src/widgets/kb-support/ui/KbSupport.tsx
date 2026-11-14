// Поддержка базы знаний (SC-21, SC-22, SCR-L3): открытые вопросы, черновики без «Ок» (BR-33), «устарело»,
// дубли от плановой проверки (BR-38). Видит владелец базы — по умолчанию руководитель отдела, он может
// назначить другого сотрудника (BR-34). Руководитель видит, кто владелец, даже если передал базу.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Stack, Typography } from "@mui/material";
import { api, type User } from "@/shared/api";
import { Block, Row } from "@/shared/ui";
import { PersonDialog } from "@/widgets/person-picker";

type Owner = { owner_id: string; can_change: boolean; is_owner: boolean; default_id: string };
type Maintenance = {
  owner_id: string;
  open_questions: Array<{ id: string; title: string; asked_by: string; reason: string }>;
  drafts: Array<{ id: string; title: string; author_id: string; since: string }>;
  stale: Array<{ page_id: string; title: string; author_id: string; marks: number }>;
  duplicates: Array<{ id: string; titles: string[]; proposed_title: string; note?: string; decision?: string }>;
  next_check: string;
  last_check?: string;
};

const ruDate = (d: string) => (d ? new Date(`${d}T12:00:00`).toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" }) : "");

export const KbSupport = ({ login, users, onAction }: {
  login: string; users: User[]; onAction: (path: string, method: string, body: unknown, message: string) => Promise<boolean>;
}) => {
  const [owner, setOwner] = useState<Owner | null>(null);
  const [kb, setKb] = useState<Maintenance | null>(null);
  const [assigning, setAssigning] = useState<string | null>(null);
  const [choosing, setChoosing] = useState(false);
  const name = (id: string) => users.find((u) => u.id === id)?.name ?? id;
  const reload = useCallback(() => {
    api<Owner>(login, "/api/kb/owner").then((o) => {
      setOwner(o);
      if (o.is_owner) api<Maintenance>(login, "/api/kb/maintenance").then(setKb).catch(() => undefined);
      else setKb(null);
    }).catch(() => undefined);
  }, [login]);
  useEffect(reload, [reload]);
  const act = async (path: string, method: string, body: unknown, msg: string) => { if (await onAction(path, method, body, msg)) reload(); };

  if (!owner || (!owner.is_owner && !owner.can_change)) return null;
  const pending = kb ? kb.open_questions.length + kb.drafts.length + kb.stale.length + kb.duplicates.filter((d) => !d.decision).length : 0;
  const ownerLine = <Stack direction="row" alignItems="center" gap={1.5} flexWrap="wrap" mb={3}>
    <Typography variant="body2" color="text.secondary">
      Владелец базы: <Box component="b" sx={{ color: "text.primary" }}>{owner.is_owner ? "ты" : name(owner.owner_id)}</Box>
      {owner.owner_id === owner.default_id ? " — руководитель отдела" : ""}
    </Typography>
    {owner.can_change && <Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={() => setChoosing(true)}>Передать другому</Button>}
  </Stack>;

  return <>
    <Block title="Поддержка базы знаний" aside={kb && <Typography variant="body2" color="text.secondary">
      {pending ? `требует внимания: ${pending}` : "всё в порядке"} · плановая проверка {ruDate(kb.next_check)}</Typography>}>
      {ownerLine}
      {!owner.is_owner ? <Typography color="text.secondary">Открытые вопросы, черновики и дубли сейчас разбирает {name(owner.owner_id)}. Вернуть базу себе — «Передать другому».</Typography>
        : !kb ? <Typography color="text.secondary">Загружаю…</Typography> : <Stack gap={3}>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Открытые вопросы</Typography>
          {kb.open_questions.length ? kb.open_questions.map((q) => <Row key={q.id} title={q.title} sub={`${name(q.asked_by)} · ${q.reason}`}
            end={<Button variant="contained" size="small" sx={{ minHeight: 36 }} onClick={() => setAssigning(q.id)}>Назначить</Button>} />)
            : <Typography variant="body2" color="text.secondary">Открытых вопросов нет.</Typography>}
        </Box>
        <Box>
          <Stack direction="row" justifyContent="space-between" alignItems="baseline" gap={2} flexWrap="wrap" mb={1}>
            <Typography variant="h3" component="h3">Дубли от плановой проверки</Typography>
            <Stack direction="row" alignItems="baseline" gap={1.5}>
              <Typography variant="body2" color="text.secondary">{kb.last_check ? `последняя — ${ruDate(kb.last_check)}` : "раз в 4 месяца"}</Typography>
              <Button variant="outlined" size="small" sx={{ minHeight: 36 }} onClick={() => void act("/api/kb/check", "POST", {}, "Проверил базу: похожие страницы — в списке")}>Проверить сейчас</Button>
            </Stack>
          </Stack>
          {kb.duplicates.length ? kb.duplicates.map((d) => <Row key={d.id} title={d.titles.join("  ≈  ")}
            sub={d.decision ? (d.decision === "merge" ? `Объединено${d.proposed_title ? `: «${d.proposed_title}»` : ""}` : "Отмечено: не дубль")
              : d.proposed_title ? `Предлагаю общий заголовок: «${d.proposed_title}»${d.note ? ` · ${d.note}` : ""}` : `Похоже, это разные темы: ${d.note ?? "проверь сам"}`}
            end={!d.decision && <>
              <Button variant="contained" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/duplicates/${d.id}`, "POST", { decision: "merge" }, "Страницы объединены, авторы уведомлены")}>Объединить</Button>
              <Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/duplicates/${d.id}`, "POST", { decision: "not_duplicate" }, "Отмечено: не дубль")}>Не дубль</Button>
            </>} />) : <Typography variant="body2" color="text.secondary">Похожих страниц не нашлось.</Typography>}
        </Box>
        <Box>
          <Typography variant="h3" component="h3" mb={1}>Черновики без «Ок» автора</Typography>
          <Typography variant="body2" color="text.secondary" mb={1}>Через 3 рабочих дня автору приходит напоминание, через 6 — черновик попадает сюда.</Typography>
          {kb.drafts.length ? kb.drafts.map((d) => <Row key={d.id} title={d.title} sub={`${name(d.author_id)} · с ${ruDate(d.since)}`}
            end={<Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={() => void act(`/api/kb/drafts/${d.id}/remind`, "POST", {}, "Напомнил автору")}>Напомнить</Button>} />)
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
      onPick={(to) => { const id = assigning; setAssigning(null); if (id) void act(`/api/questions/${id}/assign`, "POST", { to }, "Вопрос назначен"); }} />
    <PersonDialog open={choosing} title="Владелец базы знаний" confirm="Передать" allowAuto={false}
      text="Ему будут приходить открытые вопросы, черновики без «Ок» и сводка плановой проверки. Вернуть себе можно в любой момент."
      people={users.filter((u) => !u.roles.includes("intern") && u.id !== owner.owner_id)}
      onClose={() => setChoosing(false)}
      onPick={(to) => { setChoosing(false); void act("/api/kb/owner", "PUT", { user_id: to }, "Владелец базы назначен, ему пришло уведомление"); }} />
  </>;
};
