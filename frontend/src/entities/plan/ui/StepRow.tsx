import { Box, Button, Checkbox, Stack, Typography, useTheme } from "@mui/material";
import type { PlanStep } from "@/shared/api";
import { dueText } from "@/shared/lib";
import { Row, StatusTag } from "@/shared/ui";

/** Шаг плана: list item Akita + StatusTag (без цветных полосок). */
export const StepRow = ({ step, today, onMarkDone, onHelp, onTeam }: {
  step: PlanStep; today: string; onMarkDone?: (s: PlanStep) => void; onHelp?: (s: PlanStep) => void;
  /** Шаг «Познакомься с командой» открывает карту команды (SCR-I5). */
  onTeam?: () => void;
}) => {
  const { t, size, space, type } = useTheme();
  const overdue = step.status === "Overdue";
  const actionBtn = {
    minHeight: { xs: size.control, sm: size.controlSm },
    px: 1.5,
    bgcolor: "transparent",
    "&:hover": { bgcolor: t.surfaceHover },
  };
  const actions = (
    <>
      <StatusTag status={step.status} />
      {onTeam && step.action === "team_map" && (
        <Button variant="text" size="small" onClick={onTeam} sx={actionBtn}>Карта команды</Button>
      )}
      {onHelp && step.status !== "Done" && (
        <Button variant="text" size="small" onClick={() => onHelp(step)} sx={actionBtn}>Помоги</Button>
      )}
    </>
  );

  return (
    <Row
      align="flex-start"
      start={onMarkDone ? (
        <Checkbox
          inputProps={{ "aria-label": `Отметить: ${step.title}` }}
          checked={step.status === "Done"}
          disabled={step.status === "Done"}
          onChange={() => onMarkDone(step)}
          sx={{ ml: { xs: 0, sm: -1 }, mt: "2px" }}
        />
      ) : undefined}
      title={
        <Box>
          <Typography fontWeight={type.weight.medium} sx={{ overflowWrap: "break-word" }}>{step.title}</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: `${space[1]}px` }}>
            {dueText(step, today)}
          </Typography>
          {/* Мобилка: статус и действия под текстом, в одной колонке с заголовком */}
          <Stack
            direction="row"
            alignItems="center"
            flexWrap="wrap"
            gap={`${space[2]}px`}
            sx={{ mt: `${space[2]}px`, display: { xs: "flex", sm: "none" } }}
          >
            {actions}
          </Stack>
        </Box>
      }
      end={
        <Stack
          direction="row"
          alignItems="center"
          flexWrap="wrap"
          gap={`${space[2]}px`}
          sx={{ display: { xs: "none", sm: "flex" }, pt: "2px" }}
        >
          {actions}
        </Stack>
      }
      sx={overdue ? { bgcolor: t.badSoft } : undefined}
    />
  );
};
