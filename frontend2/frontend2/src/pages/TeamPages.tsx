// Руководитель — кто отстаёт и что согласовать. Бадди — где застрял стажёр. Главное — в первой строке.
import { Box, Button, LinearProgress, Stack, Typography, useTheme } from "@mui/material";
import type { TeamOverview } from "../api";
import { plural, requestTitle } from "../helpers";
import type { DataState } from "../state";
import { Block, Empty, PageHead, Row, StatusTag } from "../components/ui";

const Metric = ({ value, label, alert }: { value: number; label: string; alert?: boolean }) => {
  const { t } = useTheme();
  return (
    <Box sx={{ bgcolor: alert && value > 0 ? t.badSoft : t.surface, p: 2 }}>
      <Typography sx={{ fontSize: 36, fontWeight: 300, lineHeight: 1.1 }}>{value}</Typography>
      <Typography variant="body2" color="text.secondary">{label}</Typography>
    </Box>
  );
};

export const TeamPage = ({ overview, onRequests }: { overview?: TeamOverview; onRequests: () => void }) => {
  if (!overview) return <Empty text="Обзор команды пока недоступен." />;
  const { totals } = overview;
  const behind = overview.interns.filter((i) => i.overdue_steps > 0);
  const pending = overview.requests.filter((r) => r.status === "pending");
  const title = behind.length ? `${behind.length} ${plural(behind.length, "стажёр отстаёт", "стажёра отстают", "стажёров отстают")} от плана` : "Все стажёры идут по плану";
  return <>
    <PageHead context={overview.department} title={title}
      action={pending.length > 0 && <Button variant="contained" onClick={onRequests}>Согласовать заявки: {pending.length}</Button>} />
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 1.5, mb: 5 }}>
      <Metric value={totals.interns} label={plural(totals.interns, "стажёр", "стажёра", "стажёров")} />
      <Metric value={totals.overdue_steps} label="просроченных шагов" alert />
      <Metric value={totals.open_questions} label="вопросов без ответа" />
      <Metric value={totals.pending_requests} label="заявок ждут решения" alert />
    </Box>
    <Block title="Стажёры">
      {overview.interns.length ? overview.interns.map((i) => (
        <Box key={i.id} sx={{ py: 2, borderBottom: 1, borderColor: "divider" }}>
          <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" gap={1}>
            <Box>
              <Typography fontWeight={600}>{i.name}</Typography>
              <Typography variant="body2" color="text.secondary">{i.week}-я неделя · бадди {i.buddy} · {i.stage || (i.plan_status === "draft" ? "план готовится" : "этапы пройдены")}</Typography>
            </Box>
            <Stack direction="row" gap={1} alignItems="center">
              {i.overdue_steps > 0 && <StatusTag status="Overdue" label={`Просрочено: ${i.overdue_steps}`} />}
              {i.plan_status !== "approved" && <StatusTag status="Draft" label="План на согласовании" />}
            </Stack>
          </Stack>
          <Stack direction="row" alignItems="center" gap={1.5} mt={1.5}>
            <LinearProgress variant="determinate" value={i.progress_pct} aria-label={`Прогресс ${i.name}`} sx={{ flex: 1 }} />
            <Typography variant="body2" sx={{ minWidth: 40, textAlign: "right" }}>{i.progress_pct}%</Typography>
          </Stack>
        </Box>)) : <Empty text="В отделе пока нет стажёров." />}
    </Block>
    {pending.length > 0 && <Box mt={5}><Block title="Ждут решения">
      {pending.map((r) => <Row key={r.id} title={`${r.employee_name}: ${requestTitle({ ...r, approver_id: "", created_at: "", source: "" })}`} sub={r.id} onClick={onRequests} end={<StatusTag status="pending" />} />)}
    </Block></Box>}
  </>;
};

export const BuddyPage = ({ data, onPlan, onQuestions }: { data: DataState; onPlan: (id: string) => void; onQuestions: () => void }) => {
  const stuck = data.managedPlans.filter((i) => i.plan?.steps.some((s) => s.status === "Overdue"));
  const routed = data.questions.filter((q) => q.asked_by !== data.me.id && q.status === "Sent").length;
  return <>
    <PageHead context={`Бадди · ${data.me.department}`}
      title={stuck.length ? `${stuck.map((i) => i.name.split(" ")[0]).join(", ")}: есть просрочки` : "У стажёров всё идёт по плану"}
      action={routed > 0 && <Button variant="text" onClick={onQuestions}>Вопросы в пути: {routed}</Button>} />
    <Block title="Мои стажёры">
      {data.managedPlans.length ? data.managedPlans.map((i) => {
        const overdue = i.plan?.steps.filter((s) => s.status === "Overdue") ?? [];
        return <Box key={i.id} sx={{ py: 2, borderBottom: 1, borderColor: "divider" }}>
          <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" gap={1.5}>
            <Box>
              <Typography fontWeight={600}>{i.name}</Typography>
              <Typography variant="body2" color="text.secondary">{i.plan ? (overdue.length ? `Застрял: ${overdue.map((s) => s.title).join("; ")}` : `Дальше: ${i.plan.next_step?.title ?? "все шаги пройдены"}`) : "План готовится"}</Typography>
            </Box>
            <Button variant="text" onClick={() => onPlan(i.id)} sx={{ alignSelf: { sm: "center" } }}>Открыть план</Button>
          </Stack>
          {i.plan && <Stack direction="row" alignItems="center" gap={1.5} mt={1.5}>
            <LinearProgress variant="determinate" value={i.plan.progress_pct} aria-label={`Прогресс ${i.name}`} sx={{ flex: 1 }} />
            <Typography variant="body2" sx={{ minWidth: 40, textAlign: "right" }}>{i.plan.progress_pct}%</Typography>
          </Stack>}
        </Box>;
      }) : <Empty text="За тобой пока не закреплены стажёры." />}
    </Block>
  </>;
};
