// Страница «Мой план» (FSD: pages/plan).
import type { ReactNode } from "react";
import { Button, Stack, Typography } from "@mui/material";
import type { PlanData, PlanStep } from "@/shared/api";
import { Block, Empty, PageHead } from "@/shared/ui";
import { Progress, StepRow } from "@/entities/plan";
import { PlanHistory } from "@/widgets/plan-draft";

export const PlanPage = ({ plan, title, context, today, onMarkDone, picker, onTeam, onManage }: {
  plan?: PlanData; title: string; context: string; today: string; onMarkDone?: (s: PlanStep) => void; picker?: ReactNode; onTeam?: () => void; onManage?: () => void;
}) => <>
  <PageHead context={context} title={title} action={<Stack gap={1}>{picker}{onManage && <Button onClick={onManage}>Редакции и согласование</Button>}</Stack>} />
  {!plan ? <Empty text="План ещё не утверждён." variant={2} /> : <Stack gap={{ xs: 2, md: 5 }}>
    <Progress plan={plan} />
    {plan.plan_revision && <Typography variant="body2" color="text.secondary">Действующая редакция: {plan.plan_revision}</Typography>}
    {plan.stages.map((stage) => {
      const steps = plan.steps.filter((s) => s.stage === stage.id && s.owner === "intern");
      if (!steps.length) return null;
      const done = steps.filter((s) => s.status === "Done").length;
      return <Block key={stage.id} title={stage.title} aside={<Typography variant="body2" color="text.secondary">{done} из {steps.length}</Typography>}>
        {steps.map((s) => <StepRow key={s.id} step={s} today={today} onMarkDone={onMarkDone} onTeam={onTeam} />)}
      </Block>;
    })}
    <PlanHistory history={plan.plan_history ?? []} />
  </Stack>}
</>;
