// Стажёр: «Мой день» и «Мой план». Главное за 3 секунды — сколько шагов сегодня и что горит.
import type { ReactNode } from "react";
import { Box, Button, Checkbox, Stack, Typography, useTheme } from "@mui/material";
import { brand } from "../theme";
import type { PlanData, PlanStep } from "../api";
import { dueText, longDate, plural } from "../helpers";
import type { DataState } from "../state";
import { Block, Empty, PageHead, Row, StatusTag } from "../components/ui";

const StepRow = ({ step, today, onMarkDone, onHelp }: { step: PlanStep; today: string; onMarkDone?: (s: PlanStep) => void; onHelp?: (s: PlanStep) => void }) => (
  <Row
    start={onMarkDone ? <Checkbox inputProps={{ "aria-label": `Отметить: ${step.title}` }} checked={step.status === "Done"} disabled={step.status === "Done"} onChange={() => onMarkDone(step)} sx={{ ml: -1 }} /> : undefined}
    title={step.title}
    sub={<Box component="span" sx={step.status === "Overdue" ? { color: "error.main", fontWeight: 600 } : undefined}>{dueText(step, today)}</Box>}
    end={<>
      {step.status !== "Today" && <Box sx={{ display: { xs: "none", sm: "block" } }}><StatusTag status={step.status} /></Box>}
      {onHelp && step.status !== "Done" && <Button variant="text" size="small" sx={{ minHeight: 36, px: 1.5 }} onClick={() => onHelp(step)}>Помоги</Button>}
    </>}
  />
);

/** Прогресс по этапам плана (SCR-I1): у каждого этапа своя полоса, текущий — подписан, просрочки — красным. */
const StageBars = ({ plan }: { plan: PlanData }) => {
  const { t } = useTheme();
  const stages = plan.stages.map((st) => {
    const steps = plan.steps.filter((s) => s.stage === st.id);
    return { st, total: steps.length, done: steps.filter((s) => s.status === "Done").length, overdue: steps.filter((s) => s.status === "Overdue").length };
  }).filter((x) => x.total > 0);
  const current = stages.find((x) => x.done < x.total);
  return <Box sx={{ display: "grid", gridTemplateColumns: `repeat(${stages.length}, minmax(0, 1fr))`, gap: 0.75, mt: 2 }}>
    {stages.map((x) => (
      <Box key={x.st.id} title={`${x.st.title}: ${x.done} из ${x.total}`}>
        <Box sx={{ height: 8, bgcolor: t.surface, position: "relative", overflow: "hidden" }}>
          <Box sx={{ position: "absolute", inset: 0, width: `${(x.done / x.total) * 100}%`, bgcolor: x.overdue ? brand.red : brand.green }} />
        </Box>
        <Typography variant="caption" sx={{ display: "block", mt: 0.75, color: x === current ? "text.primary" : "text.secondary", fontWeight: x === current ? 600 : 400, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {x.st.title}
        </Typography>
        <Typography variant="caption" color="text.secondary">{x.done}/{x.total}{x.overdue ? ` · просрочено ${x.overdue}` : ""}</Typography>
      </Box>))}
  </Box>;
};

export const Progress = ({ plan, onOpen }: { plan?: PlanData; onOpen?: () => void }) => (
  <Block title="Путь адаптации" aside={onOpen && <Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={onOpen}>Весь план</Button>}>
    {!plan ? <Empty text="План ещё готовится — бадди и руководитель его утвердят." variant={2} /> : <>
      <Stack direction="row" alignItems="baseline" gap={1.5}>
        <Typography sx={{ fontSize: 44, fontWeight: 300, lineHeight: 1 }}>{plan.progress_pct}%</Typography>
        <Typography color="text.secondary">{plan.steps.filter((s) => s.status === "Done").length} из {plan.steps.length} шагов</Typography>
      </Stack>
      <StageBars plan={plan} />
      <Typography variant="body2" color="text.secondary" mt={1.5}>{plan.next_step ? `Дальше: ${plan.next_step.title}` : "Все шаги пройдены"}</Typography>
    </>}
  </Block>
);

export const DayPage = ({ data, onMarkDone, onAsk, onOpenPlan, onOpenQuestions, waiting }: {
  data: DataState; onMarkDone: (s: PlanStep) => void; onAsk: (text: string) => void; onOpenPlan: () => void; onOpenQuestions?: () => void; waiting: ReactNode;
}) => {
  const first = data.me.name.split(" ")[0];
  const steps = [...(data.today?.steps ?? [])].sort((a, b) => Number(b.status === "Overdue") - Number(a.status === "Overdue"));
  const open = steps.filter((s) => s.status !== "Done");
  const events = data.today?.events ?? [];
  return <>
    <PageHead context={`${longDate(data.me.today)} · ${data.me.week}-я неделя стажировки`}
      title={open.length ? `${first}, на сегодня ${open.length} ${plural(open.length, "шаг", "шага", "шагов")}` : `${first}, на сегодня всё сделано`} />
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1.6fr) minmax(280px, 1fr)" }, gap: { xs: 4, md: 5 } }}>
      <Stack gap={5}>
        <Block title="Сегодня">
          {steps.length ? steps.map((s) => <StepRow key={s.id} step={s} today={data.me.today} onMarkDone={onMarkDone} onHelp={(x) => onAsk(`Помоги с шагом: ${x.title}`)} />)
            : <Empty text="Шагов на сегодня нет. Можно заглянуть в план или базу знаний." />}
        </Block>
        <Progress plan={data.plan} onOpen={onOpenPlan} />
      </Stack>
      <Stack gap={5}>
        <Block title="Ближайшее" tone="surface">
          {events.length ? events.map((e) => (
            <Stack key={`${e.start}-${e.title}`} direction="row" gap={2} alignItems="baseline" py={0.75}>
              <Typography sx={{ fontSize: 22, fontWeight: 300, minWidth: 64 }}>{e.start}</Typography>
              <Typography>{e.title}</Typography>
            </Stack>)) : <Typography color="text.secondary">Сегодня без встреч.</Typography>}
        </Block>
        <Block title="Мои вопросы" aside={onOpenQuestions && <Button variant="text" size="small" sx={{ minHeight: 36 }} onClick={onOpenQuestions}>Все</Button>}>
          {data.questions.length ? data.questions.slice(-3).reverse().map((q) => (
            <Row key={q.id} title={q.title || q.text || "Вопрос"} sub={`Кому: ${q.addressee}`} end={<StatusTag status={q.status} />} onClick={onOpenQuestions} />))
            : <Empty text="Вопросов людям пока нет. Спроси Напарника — он сначала поищет в базе." variant={1} />}
        </Block>
        {waiting}
      </Stack>
    </Box>
  </>;
};

export const PlanPage = ({ plan, title, context, today, onMarkDone, picker }: {
  plan?: PlanData; title: string; context: string; today: string; onMarkDone?: (s: PlanStep) => void; picker?: ReactNode;
}) => <>
  <PageHead context={context} title={title} action={picker} />
  {!plan ? <Empty text="План ещё не утверждён." variant={2} /> : <Stack gap={5}>
    <Progress plan={plan} />
    {plan.stages.map((stage) => {
      const steps = plan.steps.filter((s) => s.stage === stage.id && s.owner === "intern");
      if (!steps.length) return null;
      const done = steps.filter((s) => s.status === "Done").length;
      return <Block key={stage.id} title={stage.title} aside={<Typography variant="body2" color="text.secondary">{done} из {steps.length}</Typography>}>
        {steps.map((s) => <StepRow key={s.id} step={s} today={today} onMarkDone={onMarkDone} />)}
      </Block>;
    })}
  </Stack>}
</>;
