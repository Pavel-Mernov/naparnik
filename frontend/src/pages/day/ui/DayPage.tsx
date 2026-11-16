// Страница «Мой день» (FSD: pages/day).
import type { ReactNode } from "react";
import { Box, Button, Stack, Typography, useTheme } from "@mui/material";
import type { ChatTurn, PlanStep } from "@/shared/api";
import { longDate, plural } from "@/shared/lib";
import type { DataState } from "@/shared/lib";
import { Block, Empty, PageHead, Row, StatusTag } from "@/shared/ui";
import { Progress, StepRow } from "@/entities/plan";

const TodayBlock = ({ steps, today, onMarkDone, onHelp, onTeam }: {
  steps: PlanStep[]; today: string; onMarkDone: (s: PlanStep) => void; onHelp: (s: PlanStep) => void; onTeam?: () => void;
}) => (
  <Block title="Сегодня">
    {!steps.length ? <Empty text="Шагов на сегодня нет. Можно заглянуть в план или базу знаний." /> : (
      steps.map((s) => <StepRow key={s.id} step={s} today={today} onMarkDone={onMarkDone} onHelp={onHelp} onTeam={onTeam} />)
    )}
  </Block>
);

export const DayPage = ({ data, asked = [], onMarkDone, onAsk, onOpenPlan, onOpenQuestions, onTeam, waiting }: {
  data: DataState; asked?: ChatTurn[]; onTeam?: () => void; onMarkDone: (s: PlanStep) => void; onAsk: (text: string) => void; onOpenPlan: () => void; onOpenQuestions?: () => void; waiting: ReactNode;
}) => {
  const { t, size } = useTheme();
  const first = data.me.name.split(" ")[0];
  const steps = [...(data.today?.steps ?? [])].sort((a, b) => Number(b.status === "Overdue") - Number(a.status === "Overdue"));
  const open = steps.filter((s) => s.status !== "Done");
  const events = data.today?.events ?? [];
  return <>
    <PageHead context={`${longDate(data.me.today)} · ${data.me.week}-я неделя стажировки`}
      title={open.length ? `${first}, на сегодня ${open.length} ${plural(open.length, "шаг", "шага", "шагов")}` : `${first}, на сегодня всё сделано`} />
    <Box sx={{
      display: "grid",
      gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1.6fr) minmax(280px, 1fr)" },
      gridTemplateAreas: {
        xs: `"today" "side" "progress"`,
        md: `"today side" "progress side"`,
      },
      gap: { xs: 2, md: 5 },
      alignItems: "start",
    }}>
      <Box sx={{ gridArea: "today", minWidth: 0 }}>
        <TodayBlock
          steps={steps}
          today={data.me.today}
          onMarkDone={onMarkDone}
          onHelp={(x) => onAsk(`Помоги с шагом: ${x.title}`)}
          onTeam={onTeam}
        />
      </Box>
      <Box sx={{ gridArea: "progress", minWidth: 0 }}>
        <Progress plan={data.plan} onOpen={onOpenPlan} />
      </Box>
      <Stack gap={{ xs: 2, md: 5 }} sx={{ gridArea: "side", minWidth: 0 }}>
        <Block title="Ближайшее" tone="surface">
          {events.length ? events.map((e, i) => (
            <Stack key={`${e.start}-${e.title}`} gap={2.5}
              sx={i > 0 ? { pt: 2.5, borderTop: `1px solid ${t.line}` } : undefined}>
              <Stack direction="row" alignItems="flex-end" gap={2} flexWrap="wrap">
                <Typography sx={(theme) => ({
                  fontSize: { xs: theme.type.size.h2, md: theme.type.size.display },
                  fontWeight: theme.type.weight.medium,
                  letterSpacing: theme.type.tracking.snug,
                  lineHeight: 1,
                  flex: "none",
                })}>{e.start}</Typography>
                <Typography fontWeight={500} color="text.secondary" sx={{ lineHeight: 1.2, pb: "2px", minWidth: 0, flex: "1 1 120px" }}>{e.title}</Typography>
              </Stack>
              <Button
                variant="contained"
                component="a"
                href="https://zoom.us/j/9001001002?pwd=demo"
                target="_blank"
                rel="noopener noreferrer"
                fullWidth
                sx={{ minHeight: size.control }}
              >
                Войти в конференцию
              </Button>
            </Stack>)) : <Typography color="text.secondary">Сегодня без встреч.</Typography>}
        </Block>
        <Block title="Вопросы" aside={onOpenQuestions && <Button variant="text" size="small" sx={{ minHeight: { xs: size.control, sm: size.controlSm } }} onClick={onOpenQuestions}>Все</Button>}>
          {data.questions.slice(-3).reverse().map((q) => (
            <Row
              key={q.id}
              title={q.title || q.text || "Вопрос"}
              sub={`Кому: ${q.addressee}`}
              end={<StatusTag status={q.status} />}
              onClick={onOpenQuestions}
            />))}
          {/* Вопросы людям важнее; свободные строки занимают последние вопросы Напарнику — они тоже не теряются. */}
          {(data.questions.length >= 3 ? [] : asked.slice(-(3 - data.questions.length))).reverse().map((x) => (
            <Row
              key={x.id}
              title={x.question}
              sub={x.reply.sources.length ? `Напарник ответил по базе: ${x.reply.sources[0].title}` : "Ответил Напарник"}
              end={<StatusTag status="Answered" label="Напарник" />}
              onClick={onOpenQuestions}
            />))}
          {!data.questions.length && !asked.length && <Empty text="Вопросов пока нет. Спроси Напарника — он сначала поищет в базе." icon={false} />}
        </Block>
        {waiting}
      </Stack>
    </Box>
  </>;
};
