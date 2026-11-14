// Черновик плана адаптации (SC-20, US-09, BR-81): бадди правит сроки и убирает лишние шаги,
// руководитель утверждает одной кнопкой. Без решения за 2 рабочих дня план активируется по шаблону — «не утверждён».
// Права решает бэкенд (can_edit, can_approve); экран только показывает доступные действия.
import { useEffect, useMemo, useState } from "react";
import {
  Alert, Box, Button, Dialog, DialogActions, DialogContent, Skeleton, Stack, TextField, Typography, useMediaQuery, useTheme,
} from "@mui/material";
import { approvePlan, editPlanDraft, getPlanDraft, type PlanDraft, type PlanDraftEdit } from "@/shared/api";
import { shortDateTime } from "@/shared/lib";
import { DialogClose, StatusTag } from "@/shared/ui";

const OWNER: Record<string, string> = { intern: "стажёр", buddy: "бадди", lead: "руководитель" };

/** Плашка и строка-пояснение по состоянию плана. */
export const planState = (d: Pick<PlanDraft, "status" | "auto_activate_at" | "approved_by">) => {
  if (d.status === "approved") return { tag: <StatusTag status="approved" label="Утверждён" />, text: d.approved_by ? `Утвердил(а) ${d.approved_by}` : "План утверждён" };
  if (d.status === "auto_activated") return { tag: <StatusTag status="pending" label="Не утверждён" />, text: "Активирован по шаблону: руководитель не принял решение за 2 рабочих дня" };
  return { tag: <StatusTag status="Draft" label="Черновик" />, text: d.auto_activate_at ? `Без решения активируется по шаблону ${shortDateTime(d.auto_activate_at)}` : "Ждёт правок бадди и утверждения руководителя" };
};

export type PlanDraftDialogProps = {
  open: boolean;
  login: string;
  internId: string;
  onClose: () => void;
  /** Действие выполнено: сообщение для снекбара; родитель перезагружает данные. */
  onDone: (message: string) => void;
};

export const PlanDraftDialog = ({ open, login, internId, onClose, onDone }: PlanDraftDialogProps) => {
  const theme = useTheme();
  const { t, type } = theme;
  const mobile = useMediaQuery(theme.breakpoints.down("sm"));
  const [draft, setDraft] = useState<PlanDraft | null>(null);
  const [edits, setEdits] = useState<Record<string, PlanDraftEdit>>({});
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open || !internId) return;
    const controller = new AbortController();
    setDraft(null); setEdits({}); setComment(""); setError("");
    getPlanDraft(login, internId, { signal: controller.signal })
      .then((d) => { if (!controller.signal.aborted) setDraft(d); })
      .catch((e) => { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : "План недоступен"); });
    return () => controller.abort();
  }, [open, login, internId]);

  const steps = useMemo(() => (draft?.steps ?? []).map((s) => {
    const e = edits[s.id];
    return { ...s, due_date: e?.due_date ?? s.due_date, removed: e?.removed ?? Boolean(s.removed) };
  }), [draft, edits]);
  const changed = Object.keys(edits).length > 0 || comment.trim() !== "";
  const kept = steps.filter((s) => !s.removed).length;

  const change = (id: string, patch: PlanDraftEdit) => setEdits((all) => {
    const original = draft?.steps.find((s) => s.id === id);
    const next = { ...all[id], ...patch, step_id: id };
    if (next.due_date === original?.due_date) delete next.due_date;
    if (next.removed === Boolean(original?.removed)) delete next.removed;
    const rest = { ...all };
    delete rest[id];
    return next.due_date === undefined && next.removed === undefined ? rest : { ...rest, [id]: next };
  });

  const save = async () => {
    setBusy(true); setError("");
    try {
      const d = await editPlanDraft(login, internId, Object.values(edits), comment.trim());
      setDraft(d); setEdits({}); setComment("");
      onDone("Правки сохранены — руководитель получил план на утверждение");
    } catch (e) { setError(e instanceof Error ? e.message : "Не получилось сохранить"); }
    finally { setBusy(false); }
  };

  const approve = async () => {
    setBusy(true); setError("");
    try {
      await approvePlan(login, internId);
      onDone("План утверждён — стажёр и бадди получили уведомление");
      onClose();
    } catch (e) { setError(e instanceof Error ? e.message : "Не получилось утвердить"); }
    finally { setBusy(false); }
  };

  const state = draft ? planState(draft) : null;
  const stages = draft ? [...draft.stages].sort((a, b) => a.order - b.order) : [];
  const ungrouped = steps.filter((s) => !stages.some((st) => st.id === s.stage));

  const row = (s: (typeof steps)[number]) => (
    <Stack key={s.id} direction={{ xs: "column", sm: "row" }} alignItems={{ sm: "center" }} gap={{ xs: 1, sm: 2 }}
      sx={{ py: 1.5, borderBottom: `1px solid ${t.line}`, "&:last-child": { borderBottom: "none" }, opacity: s.removed ? 0.55 : 1 }}>
      <Box flex={1} minWidth={0}>
        <Typography fontWeight={500} sx={{ overflowWrap: "anywhere", textDecoration: s.removed ? "line-through" : "none" }}>{s.title}</Typography>
        <Typography variant="body2" color="text.secondary">Делает {OWNER[s.owner] ?? s.owner}{s.removed ? " · убран из плана" : ""}</Typography>
      </Box>
      {draft?.can_edit ? <Stack direction="row" gap={1} alignItems="center" flex="none">
        <TextField type="date" size="small" value={s.due_date} disabled={s.removed || busy} onChange={(e) => change(s.id, { due_date: e.target.value })}
          inputProps={{ "aria-label": `Срок: ${s.title}` }} sx={{ width: 160 }} />
        <Button variant="text" size="small" disabled={busy} onClick={() => change(s.id, { removed: !s.removed })} sx={{ minWidth: 88 }}>
          {s.removed ? "Вернуть" : "Убрать"}
        </Button>
      </Stack> : <Typography variant="body2" color="text.secondary" sx={{ flex: "none" }}>
        {s.removed ? "—" : `до ${new Date(`${s.due_date}T12:00:00`).toLocaleDateString("ru-RU", { day: "numeric", month: "short" })}`}
      </Typography>}
    </Stack>
  );

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md" fullScreen={mobile} aria-labelledby="plan-draft-title">
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ pl: 3, pr: 2, pt: 2.5, pb: 1 }}>
        <Typography
          id="plan-draft-title"
          component="h2"
          fontWeight={type.weight.semibold}
          fontSize={type.size.h2}
          lineHeight={type.line.snug}
        >
          План адаптации
        </Typography>
        <DialogClose onClose={onClose} />
      </Stack>
      <DialogContent sx={{ pt: 1 }}>
        {error && <Alert severity="error" sx={{ mb: 2, borderRadius: 0 }}>{error}</Alert>}
        {!draft ? (!error && <Box aria-label="Загрузка плана"><Skeleton width="50%" height={36} /><Skeleton width="70%" /><Skeleton height={120} /></Box>) : <>
          <Typography variant="h2" component="h2" sx={{ textWrap: "balance" }}>{draft.intern}</Typography>
          <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mt: 1, mb: 2.5 }}>
            {state?.tag}
            <Typography variant="body2" color="text.secondary">{state?.text}</Typography>
          </Stack>
          {draft.comment && <Box sx={{ mb: 2.5, pl: 2, borderLeft: `3px solid ${t.line}` }}>
            <Typography variant="body2" fontWeight={600}>Комментарий{draft.edited_by ? ` · ${draft.edited_by}` : ""}</Typography>
            <Typography variant="body2">{draft.comment}</Typography>
          </Box>}
          {draft.can_edit && <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Поправь сроки и убери то, что стажёру не нужно. Срок — рабочий день. Стажёр увидит план после утверждения руководителем.
          </Typography>}
          <Stack gap={1.5}>
            {stages.map((st) => {
              const list = steps.filter((s) => s.stage === st.id);
              return list.length ? <Box key={st.id} component="section" sx={{ bgcolor: t.surface, px: { xs: 2, md: 3 }, py: 1.5 }}>
                <Typography variant="h3" component="h3" sx={{ mb: 0.5 }}>{st.title}</Typography>
                {list.map(row)}
              </Box> : null;
            })}
            {ungrouped.length > 0 && <Box sx={{ bgcolor: t.surface, px: { xs: 2, md: 3 }, py: 1.5 }}>{ungrouped.map(row)}</Box>}
          </Stack>
          {draft.can_edit && <TextField fullWidth multiline minRows={2} label="Комментарий руководителю" value={comment} onChange={(e) => setComment(e.target.value)}
            disabled={busy} sx={{ mt: 2.5 }} helperText="Необязательно. Например: убрал шаг про Tableau — у стажёра его не будет." />}
        </>}
      </DialogContent>
      {draft && (draft.can_edit || draft.can_approve) && <DialogActions sx={{ p: 2, gap: 1, flexWrap: "wrap" }}>
        <Typography variant="body2" color="text.secondary" sx={{ mr: "auto", pl: 1 }}>{kept} из {steps.length} шагов в плане</Typography>
        {draft.can_edit && <Button variant="contained" disabled={busy || !changed} onClick={() => void save()}>Сохранить правки</Button>}
        {draft.can_approve && <Button variant="contained" disabled={busy} onClick={() => void approve()}>Утвердить план</Button>}
      </DialogActions>}
    </Dialog>
  );
};
