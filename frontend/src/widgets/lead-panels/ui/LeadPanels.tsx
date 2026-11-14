// Руководителю (SCR-L1, SCR-L3): польза Напарника в цифрах, нагрузка бадди и экспертов (M-05, M-06),
// поддержка базы — виджет kb-support.
import { useCallback, useEffect, useState } from "react";
import { Box, Stack, Typography, useTheme } from "@mui/material";
import { api, type User } from "@/shared/api";
import { Block, Empty, Row } from "@/shared/ui";
import { KbSupport } from "@/widgets/kb-support";

type LoadRow = { id: string; name: string; role: string; received: number; answered: number; meetings: number; pages: number; minutes: number };
type Metrics = { kb_answers: number; questions_to_people: number; kb_share: number; median_answer_minutes: number; pages_published: number; csat: number; load: LoadRow[] };

const pct = (x: number) => `${Math.round(x * 100)}%`;
const mins = (m: number) => (m < 60 ? `${Math.round(m)} мин` : `${Math.floor(m / 60)} ч ${Math.round(m % 60)} мин`);

export const LeadPanels = ({ login, users, onAction }: {
  login: string; users: User[]; onAction: (path: string, method: string, body: unknown, message: string) => Promise<boolean>;
}) => {
  const { t } = useTheme();
  const [m, setM] = useState<Metrics | null>(null);
  const reload = useCallback(() => {
    api<Metrics>(login, "/api/metrics").then(setM).catch(() => undefined);
  }, [login]);
  useEffect(reload, [reload]);

  const tile = (value: string, label: string, hint: string) => (
    <Box sx={{ bgcolor: t.surface, p: 2 }}>
      <Typography sx={{ fontSize: 30, fontWeight: 300, lineHeight: 1.1 }}>{value}</Typography>
      <Typography variant="body2">{label}</Typography>
      <Typography variant="caption" color="text.secondary">{hint}</Typography>
    </Box>
  );

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

    <KbSupport login={login} users={users} onAction={onAction} />
  </Stack>;
};
