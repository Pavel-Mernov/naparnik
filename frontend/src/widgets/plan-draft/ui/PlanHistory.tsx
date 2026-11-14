import { Accordion, AccordionDetails, AccordionSummary, Stack, Typography } from "@mui/material";
import { ExpandMoreRounded } from "@mui/icons-material";
import type { PlanChange, PlanDraftStep, PlanHistory as History } from "@/shared/api";
import { shortDateTime } from "@/shared/lib";

const ACTION: Record<string, string> = { created: "Создан шаблон", started: "Начата доработка", edited: "Правки сохранены", submitted: "Отправлен на согласование", returned: "Возвращён бадди", approved: "Утверждён", auto_activated: "Активирован без утверждения", cancelled: "Редакция отменена" };
const day = (value: string) => value ? new Date(`${value}T12:00:00`).toLocaleDateString("ru-RU") : "—";

export const PlanDiff = ({ changes, steps }: { changes: PlanChange[]; steps: PlanDraftStep[] }) => <Stack gap={0.5}>
  {changes.map((c) => <Typography key={c.step_id} variant="body2" sx={{ overflowWrap: "anywhere" }}>
    {steps.find((s) => s.id === c.step_id)?.title ?? c.step_id}: {c.removed ? "убран" : c.was_removed ? `добавлен · срок ${day(c.due_date)}` : `срок ${day(c.old_due_date)} → ${day(c.due_date)}`}
  </Typography>)}
</Stack>;

/** У стажёра здесь только применённые редакции; доступ к снимкам проверяет backend. */
export const PlanHistory = ({ history }: { history: History[] }) => history.length ? <Accordion disableGutters elevation={0} sx={{ minWidth: 0 }}>
  <AccordionSummary expandIcon={<ExpandMoreRounded />}><Typography fontWeight={600}>История плана</Typography></AccordionSummary>
  <AccordionDetails>
    {[...history].reverse().map((h) => <Accordion key={h.version} disableGutters elevation={0}>
      <AccordionSummary expandIcon={<ExpandMoreRounded />}>
        <Stack minWidth={0}>
          <Typography variant="body2" fontWeight={600}>Редакция {h.revision} · {ACTION[h.action] ?? h.action}</Typography>
          <Typography variant="caption" color="text.secondary">{h.by === "system" ? "Напарник" : h.by} · {shortDateTime(h.at)}</Typography>
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        {h.comment && <Typography sx={{ mb: 1, overflowWrap: "anywhere" }}>{h.comment}</Typography>}
        <PlanDiff changes={h.changes} steps={h.steps} />
        <Typography variant="caption" display="block" mt={1}>Снимок: {h.steps.filter((s) => !s.removed).length} шагов</Typography>
        {h.steps.map((s) => <Typography key={s.id} variant="body2" color="text.secondary" sx={{ overflowWrap: "anywhere" }}>{s.title} · {s.removed ? "убран" : day(s.due_date)}</Typography>)}
      </AccordionDetails>
    </Accordion>)}
  </AccordionDetails>
</Accordion> : null;
