import { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, LinearProgress, Stack, TextField, Typography } from "@mui/material";
import { api, unwrap } from "../api";
import { DraftCard, Markdown, type Draft } from "../chat";
import { Block, TestLabel } from "../components/ui";

type Source = { page_id: string; title: string; author: string; updated_at: string; body: string };
type Term = { term: string; definition?: string; summary?: string; label?: string };
type Task = { text: string; dataset: string; environment: string; criteria: string[] };
type Attempt = { number: number; answer: string; review: { passed: boolean; score: number; checks: { criterion: number; passed: boolean; feedback: string; sources?: string[] }[] } };
type Practice = {
  id: string; intern_id: string; intern_name: string; step: { title: string }; offer: string; stage: string;
  offered_today: boolean; available: boolean; task_status: string; task_version: number; approval_due?: string; approval_expired: boolean;
  general_terms: Term[]; team_terms: Term[]; team_rules: { title: string }[]; sources: Source[]; task?: Task; checklist?: string[];
  attempts?: Attempt[]; best_attempt?: Attempt; attempts_count: number; error_topics: string[]; help_available: boolean;
};
type Run = (path: string, body: unknown, key?: string) => Promise<unknown>;

const PracticeCard = ({ item: p, buddy, busy, run, login }: { item: Practice; buddy: boolean; busy: boolean; run: Run; login: string }) => {
  const path = `/api/practices/${p.intern_id}/${p.id}`;
  const [text, setText] = useState(p.task?.text ?? "");
  const [answer, setAnswer] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [sent, setSent] = useState(false);
  const attemptKey = useRef<{ signature: string; key: string } | null>(null);
  const [source, setSource] = useState<Source | null>(null);
  useEffect(() => { setText(p.task?.text ?? ""); }, [p.task_version, p.task?.text]);
  const advance = (action: string) => void run(`${path}/advance`, { action, expected_stage: p.stage });
  const last = p.attempts?.at(-1);
  const passed = p.attempts?.some((a) => a.review.passed);
  const stageTitle: Record<string, string> = { offered: "Перед задачей", general_terms: "Общие термины", team_terms: "Термины и правила команды", training: "Учебное задание", real_task: "К реальной задаче" };
  return <Block title={buddy ? `Практика: ${p.intern_name}` : "Практика перед задачей"}>
    <Stack gap={2}>
      <Typography fontWeight={700}>{p.step.title}</Typography>
      <Typography variant="body2" color="text.secondary">{buddy ? "Одобри учебное задание. Стажёр может пройти практику или сразу взять реальную задачу." : p.offered_today ? p.offer : "Практика к будущему шагу плана — можно подготовиться заранее."}</Typography>
      {!p.available && <Alert severity="info">Материалы пока недоступны. Можно повторить подготовку или перейти к задаче.</Alert>}
      {buddy ? <>
        {p.task_status === "Pending" && <><Typography>Черновик ещё не подготовлен.</Typography><Button disabled={busy} onClick={() => void run(`${path}/generate`, {})}>Подготовить задание</Button></>}
        {p.task && <>
          <TestLabel />
          <Typography variant="body2">Учебные данные: {p.task.dataset}</Typography>
          {p.task_status === "Draft" ? <>
            <TextField fullWidth multiline minRows={4} label="Черновик учебного задания" value={text} onChange={(e) => setText(e.target.value)} />
            <Typography variant="body2" color="text.secondary">Одобрение до {p.approval_due}. Если не успеешь, стажёр продолжит без задания; черновик останется здесь.</Typography>
            <Button variant="contained" disabled={busy || !text.trim()} onClick={() => void run(`${path}/approve`, { text, task_version: p.task_version })}>Одобрить задание</Button>
          </> : <><Typography color="success.main" fontWeight={700}>Задание одобрено</Typography><Markdown text={p.task.text} /></>}
          <Box component="ul" sx={{ pl: 3 }}>{p.task.criteria.map((c) => <li key={c}>{c}</li>)}</Box>
        </>}
        <Typography>Попыток: {p.attempts_count}. Бадди видит лучшую попытку и темы ошибок; остальные ответы и диалог остаются у стажёра.</Typography>
        {p.best_attempt && <Box sx={{ p: 2, bgcolor: "background.default" }}>
          <Typography fontWeight={700}>Лучшая попытка №{p.best_attempt.number} · {p.best_attempt.review.score}%</Typography>
          <Box component="pre" sx={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", fontSize: 14 }}>{p.best_attempt.answer}</Box>
        </Box>}
        {p.error_topics.length > 0 && <Box><Typography fontWeight={700}>Темы ошибок</Typography><Box component="ul" sx={{ pl: 3 }}>{p.error_topics.map((x) => <li key={x}>{x}</li>)}</Box></Box>}
      </> : <>
        <Typography variant="h3">{stageTitle[p.stage]}</Typography>
        {p.stage === "offered" && <Button variant="contained" disabled={busy || !p.available} onClick={() => advance("start")}>Начать практику</Button>}
        {(p.stage === "general_terms" || p.stage === "team_terms") && <>
          {(p.stage === "general_terms" ? p.general_terms : p.team_terms).map((term) => <Box key={term.term}>
            <Typography fontWeight={700}>{term.term}{term.label ? ` · ${term.label}` : " · у нас так говорят"}</Typography>
            <Typography>{term.definition || term.summary}</Typography>
          </Box>)}
          {p.stage === "team_terms" && p.team_rules.map((r) => <Typography key={r.title}>• {r.title}</Typography>)}
          <Button variant="contained" disabled={busy || !p.available} onClick={() => advance("next")}>Дальше</Button>
          <Button disabled={busy} onClick={() => advance("skip_stage")}>Пропустить этот этап</Button>
        </>}
        {p.stage === "training" && <>
          {!p.task ? <Alert severity="info">{p.approval_expired ? "Бадди не ответил за рабочий день. Продолжай без учебного задания." : "Бадди ещё не одобрил задание. Можно подождать или пропустить его."}</Alert> : <>
            <TestLabel /><Markdown text={p.task.text} />
            <Box component="ul" sx={{ pl: 3 }}>{p.task.criteria.map((c) => <li key={c}>{c}</li>)}</Box>
            {!passed && p.attempts_count < 3 && <>
              <TextField fullWidth multiline minRows={6} label="Твоя попытка" value={answer} onChange={(e) => setAnswer(e.target.value)}
                helperText="Учебный SQL, имя таблицы, название графика и папка. Только синтетические данные. Код здесь не исполняется." />
              <Button variant="contained" disabled={busy || !answer.trim()} onClick={async () => {
                const signature = JSON.stringify([answer, p.task_version]);
                if (attemptKey.current?.signature !== signature) attemptKey.current = { signature, key: crypto.randomUUID() };
                const result = await run(`${path}/attempts`, { answer, task_version: p.task_version }, attemptKey.current.key);
                if (result !== undefined) attemptKey.current = null;
              }}>Проверить попытку</Button>
            </>}
            <Typography>Попыток: {p.attempts_count} из 3</Typography>
            {last && <Box aria-label="Обратная связь по попытке">
              <Typography fontWeight={700} color={last.review.passed ? "success.main" : "text.primary"}>{last.review.passed ? "Учебное задание пройдено" : "Есть что поправить"} · {last.review.score}%</Typography>
              {last.review.checks.map((c) => <Box key={c.criterion} sx={{ my: 1 }}><Typography>{c.passed ? "✓" : "•"} {c.feedback}</Typography>
                {c.sources?.map((id) => { const s=p.sources.find((s)=>s.page_id===id);return s && <Button key={id} variant="text" size="small" onClick={()=>setSource(s)}>{s.title}</Button>; })}
              </Box>)}
            </Box>}
          </>}
          {(passed || p.approval_expired) && <Button variant="contained" disabled={busy} onClick={() => advance("next")}>К реальной задаче</Button>}
          <Button disabled={busy} onClick={() => advance("skip_stage")}>Пропустить учебное задание</Button>
          {p.help_available && !sent && <>
            {!draft ? <Button disabled={busy} onClick={async () => { const result = await run(`${path}/help-draft`, {}); if (result) setDraft(result as Draft); }}>Спросить бадди</Button> : <>
              <Typography fontWeight={700}>Черновик вопроса бадди</Typography>
              <DraftCard draft={draft} login={login} look={{ userName: () => "Бадди", systemTitle: (id) => id, stepTitle: () => p.step.title }}
                onDone={(d) => { if (d?.status === "confirmed") setSent(true); else if (d?.status === "cancelled") setDraft(null); }} />
            </>}
          </>}
          {sent && <Alert severity="success">Вопрос отправлен бадди.</Alert>}
        </>}
        {p.stage === "real_task" && <>
          <Typography>Практика завершена или пропущена. Реальную задачу выполняешь отдельно — шаг плана автоматически не отмечается.</Typography>
          <Box component="ul" sx={{ pl: 3 }}>{p.checklist?.map((c) => <li key={c}>{c}</li>)}</Box>
        </>}
        {p.stage !== "real_task" && <Button disabled={busy} onClick={() => advance("skip_all")}>Сразу к реальной задаче</Button>}
      </>}
      {p.sources.length > 0 && (buddy || p.stage !== "offered") && <Box>
        <Typography variant="body2" fontWeight={700}>Источники команды</Typography>
        <Stack direction="row" flexWrap="wrap" gap={1}>{p.sources.map((s) => <Button key={s.page_id} size="small" variant="text" onClick={() => setSource(s)}>{s.title}</Button>)}</Stack>
      </Box>}
    </Stack>
    <Dialog open={Boolean(source)} onClose={() => setSource(null)} fullWidth maxWidth="md">
      <DialogTitle>{source?.title}</DialogTitle><DialogContent><Typography color="text.secondary" mb={2}>{source?.author} · {source?.updated_at}</Typography><Markdown text={source?.body ?? ""} /></DialogContent>
      <DialogActions><Button onClick={() => setSource(null)}>Закрыть источник</Button></DialogActions>
    </Dialog>
  </Block>;
};

export const PracticePanel = ({ login, buddy, revision }: { login: string; buddy: boolean; revision: number }) => {
  const [items, setItems] = useState<Practice[]>([]);
  const [preparing, setPreparing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(false);
  const controllers = useRef(new Set<AbortController>());
  const sequence = useRef(0);
  const refresh = async (signal: AbortSignal) => {
    const seq = ++sequence.current;
    const data = unwrap(await api<{ ok: boolean; data: Practice[] }>(login, "/api/practices", { signal }));
    if (active.current && !signal.aborted && sequence.current === seq) setItems(data);
  };
  useEffect(() => {
    active.current = true;
    const c = new AbortController(); controllers.current.add(c);
    void (async () => {
      setPreparing(true);
      try { await refresh(c.signal); await api(login, "/api/practices/sync", { method: "POST", body: "{}", signal: c.signal }); await refresh(c.signal); }
      catch (e) { if (active.current && !c.signal.aborted) setError(e instanceof Error ? e.message : "Не удалось подготовить практику"); }
      finally { controllers.current.delete(c); if (active.current && !c.signal.aborted) setPreparing(false); }
    })();
    return () => { active.current = false; controllers.current.forEach((x) => x.abort()); controllers.current.clear(); };
    // Панель привязана к учётке через key: старые ответы при переключении отменяются.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [login]);
  useEffect(() => {
    const c = new AbortController(); controllers.current.add(c);
    void refresh(c.signal).catch(() => undefined).finally(() => controllers.current.delete(c));
    return () => c.abort();
  }, [revision]);
  const run: Run = async (path, body, key) => {
    if (busy || !active.current) return;
    const c = new AbortController(); controllers.current.add(c); setBusy(true); setError("");
    try {
      const response = await api<{ ok?: boolean; data?: unknown }>(login, path, { method: "POST", body: JSON.stringify(body), signal: c.signal, headers: key ? { "Idempotency-Key": key } : {} });
      if (!active.current || c.signal.aborted) return;
      await refresh(c.signal);
      return response.data ?? response;
    } catch (e) { if (active.current && !c.signal.aborted) setError(e instanceof Error ? e.message : "Не удалось выполнить действие"); }
    finally { controllers.current.delete(c); if (active.current && !c.signal.aborted) setBusy(false); }
  };
  if (!items.length && !preparing && !error) return null;
  return <Stack spacing={3} mt={4}>
    {preparing && <><Typography color="text.secondary">Напарник готовит учебную практику…</Typography><LinearProgress aria-label="Подготовка практики" /></>}
    {error && <Alert severity="error" action={<Button onClick={() => void run("/api/practices/sync", {})}>Повторить</Button>}>{error}</Alert>}
    {items.map((p) => <PracticeCard key={`${p.intern_id}/${p.id}`} item={p} buddy={buddy} busy={busy} run={run} login={login} />)}
  </Stack>;
};
