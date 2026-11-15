// SC-20, SCR-L2: действующий план и согласуемая редакция — разные состояния.
import { useEffect, useMemo, useRef, useState } from "react";
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, Skeleton, Stack, TextField, Typography, useMediaQuery, useTheme } from "@mui/material";
import { ApiError, approvePlan, editPlanDraft, getPlanDraft, planRevisionAction, type PlanDraft, type PlanDraftEdit } from "@/shared/api";
import { shortDateTime } from "@/shared/lib";
import { DialogClose, StatusTag } from "@/shared/ui";
import { PlanDiff, PlanHistory } from "./PlanHistory";

const OWNER: Record<string, string> = { intern: "стажёр", buddy: "бадди", lead: "руководитель" };
const REVIEW: Record<string, string> = { editing: "На доработке", review: "На согласовании", returned: "Возвращена бадди" };
export const planState = (d: Pick<PlanDraft, "status" | "auto_activate_at" | "approved_by">) => {
  if (d.status === "approved") return { tag: <StatusTag status="approved" label="Утверждён" />, text: d.approved_by ? `Утвердил(а) ${d.approved_by}` : "План утверждён" };
  if (d.status === "auto_activated") return { tag: <StatusTag status="pending" label="Не утверждён" />, text: "Активирован по шаблону: руководитель не принял решение за 2 рабочих дня" };
  return { tag: <StatusTag status="Draft" label="Черновик" />, text: d.auto_activate_at ? `Без решения активируется ${shortDateTime(d.auto_activate_at)}` : "Ожидает доработки и согласования" };
};
export type PlanDraftDialogProps = { open: boolean; login: string; internId: string; onClose: () => void; onDone: (message: string) => void };

export const PlanDraftDialog = ({ open, login, internId, onClose, onDone }: PlanDraftDialogProps) => {
  const theme = useTheme(), { t, type } = theme;
  const mobile = useMediaQuery(theme.breakpoints.down("sm"));
  const scope = useRef<AbortController | null>(null);
  const [draft, setDraft] = useState<PlanDraft | null>(null);
  const [edits, setEdits] = useState<Record<string, PlanDraftEdit>>({});
  const [comment, setComment] = useState("");
  const [reason, setReason] = useState("");
  const [returning, setReturning] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const accept = (d: PlanDraft) => { setDraft(d); setEdits({}); setComment(d.comment ?? ""); setReturning(false); setReason(""); };

  useEffect(() => {
    if (!open || !internId) return;
    const c = new AbortController(); scope.current = c;
    setDraft(null); setEdits({}); setComment(""); setReason(""); setReturning(false); setError(""); setBusy(false);
    getPlanDraft(login, internId, { signal: c.signal }).then((d) => { if (!c.signal.aborted) accept(d); })
      .catch((e) => { if (!c.signal.aborted) setError(e instanceof Error ? e.message : "План недоступен"); });
    return () => { c.abort(); };
  }, [open, login, internId]);

  const steps = useMemo(() => (draft?.steps ?? []).map((s) => ({ ...s, due_date: edits[s.id]?.due_date ?? s.due_date, removed: edits[s.id]?.removed ?? Boolean(s.removed) })), [draft, edits]);
  const changed = Object.keys(edits).length > 0 || comment.trim() !== (draft?.comment ?? "");
  const change = (id: string, patch: PlanDraftEdit) => setEdits((all) => {
    const original = draft?.steps.find((s) => s.id === id);
    const next = { ...all[id], ...patch, step_id: id };
    if (next.due_date === original?.due_date) delete next.due_date;
    if (next.removed === Boolean(original?.removed)) delete next.removed;
    const rest = { ...all }; delete rest[id];
    return next.due_date === undefined && next.removed === undefined ? rest : { ...rest, [id]: next };
  });
  const run = async (action: "save" | "approve" | "start" | "submit" | "return" | "cancel") => {
    const signal = scope.current?.signal;
    if (!draft || busy || !signal || signal.aborted) return;
    setBusy(true); setError("");
    try {
      const d = action === "save" ? await editPlanDraft(login, internId, Object.values(edits), comment.trim(), draft.version, signal)
        : action === "approve" ? await approvePlan(login, internId, draft.version, signal)
        : await planRevisionAction(login, internId, action, draft.version, reason.trim(), signal);
      if (signal.aborted) return;
      accept(d);
      const messages = { save: "Правки сохранены — отправь редакцию на согласование", approve: "План утверждён — прогресс сохранён", start: "Новая редакция открыта. Действующий план сохранён", submit: "План отправлен руководителю на согласование", return: "План возвращён бадди с комментарием", cancel: "Редакция отменена. Действующий план сохранён" };
      onDone(messages[action]);
    } catch (e) {
      if (signal.aborted) return;
      const message = e instanceof Error ? e.message : "Не получилось выполнить действие";
      // Ответ мог потеряться после коммита; проверяем результат и версию перед любым повтором.
      if (e instanceof ApiError && (e.status === 409 || e.status === 503)) {
        try { const latest = await getPlanDraft(login, internId, { signal }); if (!signal.aborted) accept(latest); }
        catch { /* сообщение и явная перезагрузка ниже */ }
      }
      if (!signal.aborted) setError(message);
    } finally { if (!signal.aborted) setBusy(false); }
  };
  const refresh = async () => {
    const signal = scope.current?.signal; if (!signal || signal.aborted) return;
    setBusy(true);
    try { const d = await getPlanDraft(login, internId, { signal }); if (!signal.aborted) { accept(d); setError(""); } }
    catch (e) { if (!signal.aborted) setError(e instanceof Error ? e.message : "План недоступен"); }
    finally { if (!signal.aborted) setBusy(false); }
  };
  const state = draft ? planState(draft) : null;
  const stages = draft ? [...draft.stages].sort((a, b) => a.order - b.order) : [];
  const row = (s: (typeof steps)[number]) => <Stack key={s.id} direction={{ xs: "column", sm: "row" }} alignItems={{ sm: "center" }} gap={1} sx={{ py: 1.5, minWidth: 0, borderBottom: `1px solid ${t.line}`, opacity: s.removed ? 0.55 : 1 }}>
    <Box flex={1} minWidth={0}>
      <Typography sx={{ overflowWrap: "anywhere", textDecoration: s.removed ? "line-through" : "none" }}>{s.title}</Typography>
      <Typography variant="body2" color="text.secondary">Делает {OWNER[s.owner] ?? s.owner}{s.done ? " · выполнен, сохранится" : s.removed ? " · убран из плана" : ""}</Typography>
    </Box>
    {draft?.can_edit ? <Stack direction="row" gap={1} alignItems="center" flex="none">
      <TextField type="date" size="small" value={s.due_date} disabled={s.removed || s.done || busy} onChange={(e) => change(s.id, { due_date: e.target.value })}
        slotProps={{ htmlInput: { "aria-label": `Срок: ${s.title}` } }} sx={{ width: 155 }} />
      <Button size="small" disabled={busy || s.done} onClick={() => change(s.id, { removed: !s.removed })}>{s.removed ? "Вернуть" : "Убрать"}</Button>
    </Stack> : <Typography variant="body2" color="text.secondary">{s.removed ? "—" : `до ${s.due_date}`}</Typography>}
  </Stack>;

  return <Dialog open={open} onClose={busy ? undefined : onClose} fullWidth maxWidth="md" fullScreen={mobile} aria-labelledby="plan-draft-title">
    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ pl: 3, pr: 2, pt: 2.5, pb: 1 }}>
      <Typography id="plan-draft-title" component="h2" fontWeight={type.weight.semibold} fontSize={type.size.h2}>План адаптации</Typography>
      <DialogClose onClose={onClose} />
    </Stack>
    <DialogContent sx={{ pt: 1, minWidth: 0 }}>
      {error && <Alert severity="error" sx={{ mb: 2 }} action={<Button disabled={busy} onClick={() => void refresh()}>Обновить</Button>}>{error}</Alert>}
      {!draft ? (!error && <Box aria-label="Загрузка плана"><Skeleton width="50%" height={36} /><Skeleton height={120} /></Box>) : <>
        <Typography variant="h2" component="h2">{draft.intern}</Typography>
        <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" my={2}>{state?.tag}<Typography variant="body2">{state?.text}</Typography></Stack>
        <Typography variant="body2" color="text.secondary" mb={2}>{draft.active_revision ? `Действующая редакция: ${draft.active_revision}` : "Действующего плана пока нет"}{draft.review_status ? ` · Редакция ${draft.revision}: ${REVIEW[draft.review_status] ?? draft.review_status}` : ""}</Typography>
        {draft.review_status && draft.active_revision > 0 && <Alert severity="info" sx={{ mb: 2 }}>До утверждения изменений стажёр работает по прежнему плану. Выполненные шаги сохраняются.</Alert>}
        {draft.return_comment && draft.review_status && <Alert severity="warning" sx={{ mb: 2 }}><Typography fontWeight={600}>Замечания руководителя</Typography><Typography>{draft.return_comment}</Typography></Alert>}
        {draft.comment && !draft.can_edit && <Typography sx={{ mb: 2, overflowWrap: "anywhere" }}>Комментарий бадди: {draft.comment}</Typography>}
        {draft.can_edit && <Typography variant="body2" color="text.secondary" mb={2}>{draft.review_status === "review" ? "Редакция уже на согласовании. После новых правок сохрани и отправь её заново." : "Поправь сроки и убери лишние шаги, затем сохрани и отправь редакцию на согласование."}</Typography>}
        {draft.changes.length > 0 && <Box sx={{ mb: 2 }}><Typography fontWeight={600}>Что изменится после утверждения</Typography><PlanDiff changes={draft.changes} steps={draft.steps} /></Box>}
        {stages.map((stage) => {
          const list = steps.filter((s) => s.stage === stage.id);
          return list.length ? <Box key={stage.id} sx={{ bgcolor: t.surface, px: { xs: 1.5, md: 3 }, py: 1.5, mb: 1.5 }}><Typography variant="h3" component="h3">{stage.title}</Typography>{list.map(row)}</Box> : null;
        })}
        {steps.filter((s) => !stages.some((stage) => stage.id === s.stage)).map(row)}
        {draft.can_edit && <TextField fullWidth multiline minRows={2} label="Комментарий руководителю" value={comment} onChange={(e) => setComment(e.target.value)} disabled={busy}
          slotProps={{ htmlInput: { maxLength: 2000 } }} sx={{ my: 2 }} helperText={draft.active_revision ? "Объясни, зачем меняется действующий план. Комментарий сохранится в истории." : "Комментарий сохранится в истории плана."} />}
        {returning && <Box sx={{ my: 2 }}>
          <Typography component="label" htmlFor="plan-return-reason" variant="body2" color="text.secondary" sx={{ display: "block", mb: 0.75 }}>
            Что нужно доработать
          </Typography>
          <TextField id="plan-return-reason" autoFocus fullWidth multiline minRows={2} value={reason} disabled={busy}
            onChange={(e) => setReason(e.target.value)} slotProps={{ htmlInput: { maxLength: 2000 } }} />
        </Box>}
        <PlanHistory history={draft.history} />
      </>}
    </DialogContent>
    {draft && <DialogActions sx={{ p: 2, gap: 1, flexWrap: "wrap" }}>
      <Typography variant="body2" color="text.secondary" sx={{ mr: "auto" }}>{steps.filter((s) => !s.removed).length} из {steps.length} шагов в плане</Typography>
      {draft.can_start && <Button disabled={busy} onClick={() => void run("start")}>Изменить действующий план</Button>}
      {draft.can_cancel && <Button disabled={busy} onClick={() => void run("cancel")}>Отменить редакцию</Button>}
      {draft.can_edit && <Button variant="contained" disabled={busy || !changed} onClick={() => void run("save")}>Сохранить правки</Button>}
      {draft.can_submit && <Button variant="contained" disabled={busy || changed} onClick={() => void run("submit")}>Отправить на согласование</Button>}
      {draft.can_return && !returning && <Button disabled={busy} onClick={() => setReturning(true)}>Вернуть бадди</Button>}
      {returning && <><Button disabled={busy} onClick={() => setReturning(false)}>Не возвращать</Button><Button variant="contained" disabled={busy || !reason.trim()} onClick={() => void run("return")}>Вернуть с комментарием</Button></>}
      {draft.can_approve && !returning && <Button variant="contained" disabled={busy} onClick={() => void run("approve")}>Утвердить план</Button>}
    </DialogActions>}
  </Dialog>;
};
