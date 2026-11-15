// Страница бадди (FSD: pages/buddy).
import { Box, Button, Link, Stack, Typography, useTheme } from "@mui/material";
import { brand } from "@/shared/theme";
import { plural, type DataState } from "@/shared/lib";
import { Block, Empty, PageHead, Row, StatusTag } from "@/shared/ui";

/** onProfile — открыть карточку стажёра (SCR-I5a); без него имя — просто текст. */
/** onPlanDraft — черновик плана: бадди правит сроки и шаги до утверждения (SC-20). */
export const BuddyPage = ({ data, onPlan, onQuestions, onProfile, onPlanDraft }: { data: DataState; onPlan: (id: string) => void; onQuestions: () => void; onProfile?: (id: string) => void; onPlanDraft?: (id: string) => void }) => {
  const { t, space, type, size } = useTheme();
  const stuck = data.managedPlans.filter((i) => i.plan?.steps.some((s) => s.status === "Overdue"));
  const drafts = data.managedPlans.filter((i) => !i.plan || i.plan.plan_status === "draft" || i.plan.plan_status === "auto_activated");
  const onTrack = data.managedPlans.length - stuck.length;
  const routed = data.questions.filter((q) => q.asked_by !== data.me.id && q.status === "Sent").length;
  const actionBtn = {
    minHeight: { xs: size.control, sm: size.controlSm },
    px: 1.5,
    bgcolor: "transparent",
    "&:hover": { bgcolor: t.surfaceHover },
  };

  const tile = (value: number, label: string, soft: "badSoft" | "waitSoft" | "okSoft" | "todaySoft", onClick?: () => void) => (
    <Box
      component={onClick ? "button" : "div"}
      type={onClick ? "button" : undefined}
      onClick={onClick}
      sx={{
        font: "inherit",
        textAlign: "left",
        border: "none",
        cursor: onClick ? "pointer" : "default",
        bgcolor: soft === "badSoft" && value === 0 ? t.surface : t[soft],
        p: `${space[4]}px`,
        minHeight: { xs: 88, md: 96 },
        color: t.text,
        "&:hover": onClick ? { outline: `2px solid ${t.text}` } : undefined,
        "&:focus-visible": onClick ? { outline: `2px solid ${t.link}` } : undefined,
      }}
    >
      <Typography sx={{ fontSize: { xs: 28, md: 36 }, fontWeight: type.weight.regular, lineHeight: 1.1 }}>{value}</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: `${space[1]}px` }}>{label}</Typography>
    </Box>
  );

  return <>
    <PageHead
      context={`Бадди · ${data.me.department}`}
      title={stuck.length ? `${stuck.map((i) => i.name.split(" ")[0]).join(", ")}: есть просрочки` : "У стажёров всё идёт по плану"}
      action={data.managedPlans.length === 0 && routed > 0 ? (
        <Button
          variant="text"
          onClick={onQuestions}
          sx={{
            minHeight: size.controlSm,
            px: `${space[3]}px`,
            bgcolor: t.waitSoft,
            "&:hover": { bgcolor: t.waitSoft, outline: `2px solid ${t.text}` },
          }}
        >
          Вопросы в пути: {routed}
        </Button>
      ) : undefined}
    />

    {data.managedPlans.length > 0 && (
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(3, 1fr)" },
          gap: { xs: 1.5, md: 2 },
          mb: { xs: 2, md: 5 },
        }}
      >
        {tile(stuck.length, plural(stuck.length, "просрочка", "просрочки", "просрочек"), "badSoft")}
        {tile(routed, plural(routed, "вопрос в пути", "вопроса в пути", "вопросов в пути"), "waitSoft", routed > 0 ? onQuestions : undefined)}
        {tile(onTrack, plural(onTrack, "идёт по плану", "идут по плану", "идут по плану"), "okSoft")}
      </Box>
    )}

    <Block title="Мои стажёры" aside={drafts.length > 0 ? <StatusTag status="Draft" label={`Черновиков: ${drafts.length}`} /> : undefined}>
      {data.managedPlans.length ? data.managedPlans.map((i) => {
        const overdue = i.plan?.steps.filter((s) => s.status === "Overdue") ?? [];
        const isDraft = !i.plan || i.plan.plan_status === "draft" || i.plan.plan_status === "auto_activated";
        const status = overdue.length
          ? <StatusTag status="Overdue" label="Застрял" />
          : isDraft
            ? <StatusTag status="pending" label="Черновик" />
            : <StatusTag status="Done" label="По плану" />;
        const planAction = i.plan || !onPlanDraft
          ? (
            <Button variant="text" size="small" onClick={() => onPlan(i.id)} sx={actionBtn}>
              Открыть план
            </Button>
          )
          : (
            <Button data-testid={`plan-control-${i.id}`} variant="contained" size="small" onClick={() => onPlanDraft(i.id)} sx={{ minHeight: { xs: size.control, sm: size.controlSm } }}>
              Проверить черновик
            </Button>
          );
        return (
          <Row
            key={i.id}
            align="flex-start"
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
                  {status}
                </Stack>
                <Typography variant="body2" color="text.secondary">
                  {i.plan
                    ? (overdue.length
                      ? overdue.map((s) => s.title).join("; ")
                      : `Дальше: ${i.plan.next_step?.title ?? "все шаги пройдены"}`)
                    : "План готовится: черновик у тебя, утвердит руководитель"}
                  {i.plan?.plan_status === "auto_activated" ? " · план не утверждён руководителем" : ""}
                </Typography>
                {i.plan && (
                  <Stack direction="row" alignItems="center" gap={`${space[3]}px`} mt={`${space[3]}px`} sx={{ width: "100%" }}>
                    <Box sx={{ flex: 1, height: 8, bgcolor: t.bg, minWidth: 0 }}>
                      <Box
                        sx={{
                          height: "100%",
                          width: `${i.plan.progress_pct}%`,
                          bgcolor: overdue.length ? brand.red : brand.green,
                          minWidth: i.plan.progress_pct > 0 ? 4 : 0,
                        }}
                      />
                    </Box>
                    <Typography variant="body2" sx={{ width: 40, textAlign: "right", flex: "none", fontVariantNumeric: "tabular-nums" }}>{i.plan.progress_pct}%</Typography>
                  </Stack>
                )}
              </Box>
            }
            end={<Stack gap={1} alignItems="flex-end" sx={{ textAlign: "right", width: { xs: "100%", sm: 220 }, minWidth: { sm: 220 }, flex: "none", boxSizing: "border-box" }}>{planAction}{i.plan && onPlanDraft && <Button data-testid={`plan-control-${i.id}`} variant="text" size="small" onClick={() => onPlanDraft(i.id)}>Редакции и согласование</Button>}</Stack>}
          />
        );
      }) : <Empty text="За тобой пока не закреплены стажёры." />}
    </Block>
  </>;
};
