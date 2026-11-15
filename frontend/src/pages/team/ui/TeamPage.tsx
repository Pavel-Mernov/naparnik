// Страница руководителя (FSD: pages/team).
import { Box, Button, Link, Stack, Typography, useTheme } from "@mui/material";
import type { TeamOverview } from "@/shared/api";
import { brand } from "@/shared/theme";
import { plural, requestTitle } from "@/shared/lib";
import { Block, Empty, PageHead, Row, StatusTag } from "@/shared/ui";

const Metric = ({ value, label, soft, onClick }: {
  value: number; label: string; soft: "surface" | "badSoft" | "waitSoft" | "okSoft" | "todaySoft"; onClick?: () => void;
}) => {
  const { t, type, space } = useTheme();
  const bg = soft === "surface" ? t.surface : (soft === "badSoft" && value === 0 ? t.surface : t[soft]);
  return (
    <Box
      component={onClick ? "button" : "div"}
      type={onClick ? "button" : undefined}
      onClick={onClick}
      sx={{
        font: "inherit",
        textAlign: "left",
        border: "none",
        cursor: onClick ? "pointer" : "default",
        bgcolor: bg,
        p: `${space[4]}px`,
        minHeight: { xs: 96, md: "auto" },
        color: t.text,
        "&:hover": onClick ? { outline: `2px solid ${t.text}` } : undefined,
        "&:focus-visible": onClick ? { outline: `2px solid ${t.link}` } : undefined,
      }}
    >
      <Typography sx={{ fontSize: { xs: 28, md: 36 }, fontWeight: type.weight.regular, lineHeight: 1.1 }}>{value}</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: `${space[1]}px` }}>{label}</Typography>
    </Box>
  );
};

/** onProfile — открыть карточку стажёра (SCR-I5a); без него имя — просто текст. */
/** onPlanDraft — открыть черновик плана на утверждение (SC-20). */
export const TeamPage = ({ overview, onRequests, onProfile, onPlanDraft }: { overview?: TeamOverview; onRequests: () => void; onProfile?: (id: string) => void; onPlanDraft?: (id: string) => void }) => {
  const { t, space, type, size } = useTheme();
  if (!overview) return <Empty text="Обзор команды пока недоступен." />;
  const { totals } = overview;
  const behind = overview.interns.filter((i) => i.overdue_steps > 0);
  const unapproved = (s: string) => s === "draft" || s === "auto_activated";
  const pending = overview.requests.filter((r) => r.status === "pending");
  const title = behind.length ? `${behind.length} ${plural(behind.length, "стажёр отстаёт", "стажёра отстают", "стажёров отстают")} от плана` : "Все стажёры идут по плану";
  const actionBtn = {
    minHeight: { xs: size.control, sm: size.controlSm },
    px: 1.5,
    bgcolor: "transparent",
    "&:hover": { bgcolor: t.surfaceHover },
  };
  return <>
    <PageHead context={overview.department} title={title}
      action={pending.length > 0 && <Button variant="contained" onClick={onRequests}>Согласовать заявки: {pending.length}</Button>} />
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: { xs: 1.5, md: 2 }, mb: { xs: 2, md: 5 } }}>
      <Metric value={totals.interns} label={plural(totals.interns, "стажёр", "стажёра", "стажёров")} soft="todaySoft" />
      <Metric value={totals.overdue_steps} label="просроченных шагов" soft="badSoft" />
      <Metric value={totals.open_questions} label="вопросов без ответа" soft="waitSoft" />
      <Metric value={totals.pending_requests} label="заявок ждут решения" soft="badSoft" onClick={pending.length > 0 ? onRequests : undefined} />
    </Box>
    <Block title="Стажёры">
      {overview.interns.length ? overview.interns.map((i) => {
        const showDraft = unapproved(i.plan_status) || Boolean(i.review_status);
        const tags = (
          <>
            {i.overdue_steps > 0 && <StatusTag status="Overdue" label={`Просрочено: ${i.overdue_steps}`} />}
            {showDraft && (
              <StatusTag
                status={i.plan_status === "draft" ? "pending" : "Draft"}
                label={i.plan_status === "draft" ? "План на согласовании" : "План не утверждён"}
              />
            )}
            {!showDraft && i.overdue_steps === 0 && <StatusTag status="Done" label="По плану" />}
          </>
        );
        const planAction = !onPlanDraft ? undefined : showDraft ? (
          <Button
            data-testid={`plan-control-${i.id}`}
            variant="contained"
            size="small"
            onClick={() => onPlanDraft(i.id)}
            sx={{ minHeight: { xs: size.control, sm: size.controlSm }, width: { xs: "100%", sm: "auto" } }}
          >
            Утвердить план
          </Button>
        ) : (
          <Button
            data-testid={`plan-control-${i.id}`}
            variant="text"
            size="small"
            onClick={() => onPlanDraft(i.id)}
            sx={{ ...actionBtn, width: { xs: "100%", sm: "auto" } }}
          >
            План и согласование
          </Button>
        );
        return (
          <Row
            key={i.id}
            align="center"
            stackEndOnMobile
            title={
              <Box>
                <Stack direction="row" alignItems="center" flexWrap="wrap" gap={`${space[2]}px`} mb={`${space[1]}px`}>
                  <Typography fontWeight={type.weight.semibold} sx={{ overflowWrap: "anywhere" }}>
                    {onProfile ? (
                      <Link
                        component="button"
                        onClick={() => onProfile(i.id)}
                        color="inherit"
                        underline="hover"
                        sx={{ font: "inherit", fontWeight: type.weight.semibold, textAlign: "left" }}
                      >
                        {i.name}
                      </Link>
                    ) : i.name}
                  </Typography>
                  {tags}
                </Stack>
                <Typography variant="body2" color="text.secondary">
                  {i.week}-я неделя · бадди {i.buddy} · {i.stage || (i.plan_status === "draft" ? "план готовится" : "этапы пройдены")}
                </Typography>
                <Stack direction="row" alignItems="center" gap={`${space[3]}px`} mt={`${space[3]}px`} sx={{ width: "100%" }}>
                  <Box sx={{ flex: 1, height: 8, bgcolor: t.bg, minWidth: 0 }} aria-label={`Прогресс ${i.name}`}>
                    <Box
                      sx={{
                        height: "100%",
                        width: `${i.progress_pct}%`,
                        bgcolor: i.overdue_steps > 0 ? brand.red : brand.green,
                        minWidth: i.progress_pct > 0 ? 4 : 0,
                      }}
                    />
                  </Box>
                  <Typography variant="body2" sx={{ width: 40, textAlign: "right", flex: "none", fontVariantNumeric: "tabular-nums" }}>{i.progress_pct}%</Typography>
                </Stack>
              </Box>
            }
            end={planAction}
          />
        );
      }) : <Empty text="В отделе пока нет стажёров." />}
    </Block>
    {pending.length > 0 && (
      <Box mt={{ xs: 2, md: 5 }}>
        <Block title="Ждут решения">
          {pending.map((r) => (
            <Row
              key={r.id}
              title={`${r.employee_name}: ${requestTitle({ ...r, approver_id: "", created_at: "", source: "" })}`}
              sub={r.id}
              onClick={onRequests}
              end={<StatusTag status="pending" />}
              align="flex-start"
            />
          ))}
        </Block>
      </Box>
    )}
  </>;
};
