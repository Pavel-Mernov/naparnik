import { useCallback, useEffect, useRef, useState, type ReactElement, type ReactNode } from "react";
import {
  Alert,
  AppBar,
  Avatar,
  Badge,
  Box,
  Button,
  Card,
  CardActionArea,
  CardContent,
  Checkbox,
  Chip,
  Container,
  CssBaseline,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  IconButton,
  InputLabel,
  LinearProgress,
  List,
  ListItem,
  ListItemAvatar,
  ListItemButton,
  ListItemText,
  MenuItem,
  Paper,
  Select,
  Snackbar,
  Stack,
  Tab,
  Tabs,
  TextField,
  ThemeProvider,
  Toolbar,
  Tooltip,
  Typography,
  createTheme,
} from "@mui/material";
import {
  AutoAwesomeRounded,
  CalendarMonthRounded,
  CheckCircleRounded,
  DescriptionRounded,
  EventAvailableRounded,
  FolderRounded,
  ForumRounded,
  HomeRounded,
  MenuBookRounded,
  NotificationsNoneRounded,
  OpenInNewRounded,
  TaskAltRounded,
} from "@mui/icons-material";
import avitoLogo from "../avito-logo.svg";
import mascotCat from "../mascot-cat.png";
import { ChatDrawer, Markdown, type ChatMessage, type Lookup } from "./chat";
import {
  api,
  ApiError,
  type TeamOverview,
  type Services,
  unwrap,
  type ChatReply,
  type KbFolder,
  type KbPage,
  type Me,
  type PlanData,
  type PlanStep,
  type Question,
  type RequestItem,
  type TodayData,
  type User,
} from "./api";

const theme = createTheme({
  palette: {
    mode: "light",
    primary: { main: "#0af", contrastText: "#fff" },
    secondary: { main: "#965eeb" },
    success: { main: "#00b956" },
    error: { main: "#ff4053" },
    background: { default: "#f7f7f5", paper: "#fff" },
    text: { primary: "#161616", secondary: "#6f6f6f" },
  },
  shape: { borderRadius: 0 },
  typography: {
    fontFamily: 'Arial, "Helvetica Neue", sans-serif',
    h1: { fontWeight: 800, letterSpacing: "-0.045em" },
    h2: { fontWeight: 800, letterSpacing: "-0.035em" },
    h3: { fontWeight: 750, letterSpacing: "-0.025em" },
    button: { textTransform: "none", fontWeight: 700 },
  },
  components: {
    MuiButton: { styleOverrides: { root: { borderRadius: 0, boxShadow: "none", minHeight: 44 } } },
    MuiCard: { styleOverrides: { root: { borderRadius: 0, border: "1px solid #e5e5e5", boxShadow: "none" } } },
    MuiPaper: { styleOverrides: { root: { backgroundImage: "none" } } },
  },
});

type View = "day" | "plan" | "questions" | "knowledge" | "requests";
type DataState = {
  me: Me;
  users: User[];
  plan?: PlanData;
  today?: TodayData;
  questions: Question[];
  requests: RequestItem[];
  pages: KbPage[];
  folders: KbFolder[];
  overview?: TeamOverview;
  managedPlans: Array<{ id: string; name: string; plan?: PlanData }>;
  services?: Services;
};

const viewItems: Array<{ value: View; label: string; icon: ReactElement }> = [
  { value: "day", label: "Мой день", icon: <HomeRounded /> },
  { value: "plan", label: "Мой план", icon: <TaskAltRounded /> },
  { value: "questions", label: "Вопросы", icon: <ForumRounded /> },
  { value: "knowledge", label: "База знаний", icon: <MenuBookRounded /> },
  { value: "requests", label: "Заявки", icon: <DescriptionRounded /> },
];

type Role = "lead" | "buddy" | "intern" | "expert";
const primaryRole = (roles: string[]): Role => roles.includes("lead") ? "lead" : roles.includes("buddy") ? "buddy" : roles.includes("intern") ? "intern" : "expert";
const absenceLabel = (type?: string) => ({ day_off: "Отгул", vacation: "Отпуск", sick_leave: "Больничный" } as Record<string, string>)[type ?? ""] ?? "Отсутствие";
const roleLabel = (role: Role, view: View) => {
  if (view === "day") return ({ lead: "Обзор команды", buddy: "Мои стажёры", intern: "Мой день", expert: "Входящие" })[role];
  if (view === "plan") return role === "intern" ? "Мой план" : "Планы стажёров";
  if (view === "requests") return role === "lead" ? "Согласование" : "Мои заявки";
  if (view === "questions") return role === "intern" ? "Мои вопросы" : role === "buddy" ? "Вопросы стажёров" : "Входящие вопросы";
  return "База знаний";
};

const statusInfo: Record<string, { label: string; color: "default" | "primary" | "success" | "warning" | "error" }> = {
  Done: { label: "Готово", color: "success" },
  Today: { label: "На сегодня", color: "primary" },
  Planned: { label: "Запланировано", color: "default" },
  Overdue: { label: "Просрочено", color: "error" },
  pending: { label: "На согласовании", color: "warning" },
  approved: { label: "Согласовано", color: "success" },
  granted: { label: "Выдан", color: "success" },
  rejected: { label: "Отклонено", color: "error" },
  Sent: { label: "Ждёт ответа", color: "primary" },
  Answered: { label: "Ответ готов", color: "success" },
};

const StatusChip = ({ status }: { status: string }) => {
  const item = statusInfo[status] ?? { label: status, color: "default" as const };
  return <Chip label={item.label} color={item.color} size="small" sx={{ fontWeight: 700 }} />;
};

const SectionTitle = ({ eyebrow, title, text }: { eyebrow: string; title: string; text: string }) => (
  <Box sx={{ mb: { xs: 3, md: 5 } }}>
    <Typography variant="overline" color="text.secondary" fontWeight={800} letterSpacing="0.12em">
      {eyebrow}
    </Typography>
    <Typography variant="h1" sx={{ fontSize: { xs: 42, md: 68 }, maxWidth: 900, lineHeight: 0.98, mt: 0.5 }}>
      {title}
    </Typography>
    <Typography color="text.secondary" sx={{ fontSize: { xs: 17, md: 20 }, mt: 2, maxWidth: 700 }}>
      {text}
    </Typography>
  </Box>
);

const EmptyState = ({ children }: { children: ReactNode }) => (
  <Stack alignItems="center" justifyContent="center" spacing={1} sx={{ py: 7, color: "text.secondary" }}>
    <AutoAwesomeRounded color="secondary" />
    <Typography>{children}</Typography>
  </Stack>
);

function App() {
  const [login, setLogin] = useState(() => localStorage.getItem("naparnik.login") ?? "intern.sokolov");
  const account = useRef(login);
  const generation = useRef(0);
  const loadSequence = useRef(0);
  const loadController = useRef<AbortController | null>(null);
  const actions = useRef(new Set<AbortController>());
  const [users, setUsers] = useState<User[]>([]);
  const [view, setView] = useState<View>("day");
  const [data, setData] = useState<DataState | null>(null);
  const [selectedIntern, setSelectedIntern] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [chatOpen, setChatOpen] = useState(false);
  const [chatText, setChatText] = useState("");
  const [chatLoading, setChatLoading] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: "assistant", text: "Привет! Я помогу с планом, вопросами команды и организационными действиями." },
  ]);
  const [selectedPage, setSelectedPage] = useState<KbPage | null>(null);
  const [rejection, setRejection] = useState<RequestItem | null>(null);
  const [reason, setReason] = useState("");
  const [comment, setComment] = useState("");
  const [deciding, setDeciding] = useState(false);
  const [snack, setSnack] = useState("");
  const role = data ? primaryRole(data.me.roles) : null;
  const visibleItems = viewItems.filter((item) => role && (item.value !== "plan" || role === "intern" || role === "buddy")
    && (item.value !== "requests" || role === "intern" || role === "lead"))
    .map((item) => ({ ...item, label: roleLabel(role!, item.value) }));
  const isCurrent = (owner: string, epoch: number) => account.current === owner && generation.current === epoch;

  const load = useCallback(async () => {
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    const seq = ++loadSequence.current;
    const epoch = generation.current;
    const current = () => account.current === login && generation.current === epoch
      && loadSequence.current === seq && !controller.signal.aborted;
    const options = { signal: controller.signal };
    const optionalPlan = async (id: string) => {
      try { return unwrap(await api<{ ok: boolean; data: PlanData }>(login, `/api/plans/${id}`, options)); }
      catch (e) { if (e instanceof ApiError && e.status === 404) return undefined; throw e; }
    };
    setLoading(true);
    setError("");
    try {
      const [me, usersPayload] = await Promise.all([
        api<Me>(login, "/api/me", options), api<{ users: User[] }>(login, "/api/users", options),
      ]);
      if (!current()) return;
      setUsers(usersPayload.users);
      const nextRole = primaryRole(me.roles);
      const [today, plan, questions, kb, requests, overview, managedPlans, services] = await Promise.all([
        nextRole === "intern" ? api<{ ok: boolean; data: TodayData }>(login, "/api/today", options).then(unwrap) : undefined,
        nextRole === "intern" ? optionalPlan(me.id) : undefined,
        api<{ questions: Question[] }>(login, "/api/questions", options),
        api<{ folders: KbFolder[]; pages: KbPage[] }>(login, "/api/kb/pages", options),
        nextRole === "intern" || nextRole === "lead" ? api<{ requests: RequestItem[] }>(login, "/api/requests", options) : undefined,
        nextRole === "lead" ? api<{ ok: boolean; data: TeamOverview }>(login, "/api/dashboard/team", options).then(unwrap) : undefined,
        nextRole === "buddy" ? Promise.all(me.interns.map(async (intern) => ({ ...intern, plan: await optionalPlan(intern.id) }))) : [],
        nextRole === "lead" || nextRole === "intern" ? api<Services>(login, "/api/services", options) : undefined,
      ]);
      if (!current()) return;
      setData({ me, users: usersPayload.users, today, plan, questions: questions.questions,
        folders: kb.folders, pages: kb.pages, requests: requests?.requests ?? [], overview, managedPlans, services });
      setSelectedIntern((previous) => me.interns.some((i) => i.id === previous) ? previous : me.interns[0]?.id ?? "");
    } catch (e) {
      if (current()) setError(e instanceof Error ? e.message : "Не удалось загрузить данные");
    } finally {
      if (current()) setLoading(false);
    }
  }, [login]);

  useEffect(() => { void load(); return () => { loadController.current?.abort(); }; }, [load]);
  useEffect(() => () => { actions.current.forEach((controller) => controller.abort()); }, []);
  // Чат на экране начинается с чистого листа — и разговор с агентом на сервере тоже.
  // Иначе после перезагрузки или смены учётки модель продолжает невидимый старый диалог.
  useEffect(() => { void api(login, "/api/chat/reset", { method: "POST", body: "{}" }).catch(() => undefined); }, [login]);

  const switchUser = (nextLogin: string) => {
    if (nextLogin === account.current) return;
    // Инвалидируем ответы немедленно, до следующего рендера/useEffect.
    account.current = nextLogin;
    generation.current++;
    loadSequence.current++;
    loadController.current?.abort();
    actions.current.forEach((controller) => controller.abort());
    actions.current.clear();
    localStorage.setItem("naparnik.login", nextLogin);
    setData(null); setView("day"); setSelectedIntern(""); setSelectedPage(null);
    setRejection(null); setReason(""); setComment(""); setDeciding(false);
    setError(""); setSnack(""); setChatText(""); setChatOpen(false); setChatLoading(false);
    setMessages([{ role: "assistant", text: "Учётка переключена. Чем помочь?" }]);
    setLoading(true); setLogin(nextLogin);
  };

  const sendChat = async (retry?: string) => {
    const text = (retry ?? chatText).trim();
    if (!text || chatLoading || account.current !== login) return;
    const owner = login, epoch = generation.current;
    const controller = new AbortController(); actions.current.add(controller);
    if (!retry) setChatText("");
    setMessages((current) => [...current, { role: "user", text }]); setChatLoading(true);
    try {
      const reply = await api<ChatReply>(owner, "/api/chat/messages", { method: "POST", body: JSON.stringify({ text }), signal: controller.signal });
      if (isCurrent(owner, epoch)) setMessages((current) => [...current, { role: "assistant", text: reply.text, reply, retryText: reply.retry ? text : undefined }]);
    } catch (e) {
      if (isCurrent(owner, epoch)) setMessages((current) => [...current, { role: "assistant", text: e instanceof Error ? e.message : "Не получилось ответить", retryText: text }]);
    } finally {
      actions.current.delete(controller);
      if (isCurrent(owner, epoch)) setChatLoading(false);
    }
  };

  const perform = async (path: string, method: string, body: unknown, message: string) => {
    const owner = login, epoch = generation.current;
    if (account.current !== owner) return;
    const controller = new AbortController(); actions.current.add(controller);
    try {
      await api(owner, path, { method, body: JSON.stringify(body), signal: controller.signal });
      if (isCurrent(owner, epoch)) { setSnack(message); setRejection(null); await load(); }
    } catch (e) {
      if (isCurrent(owner, epoch)) setSnack(e instanceof Error ? e.message : "Не получилось выполнить действие");
    } finally { actions.current.delete(controller); }
  };

  const markDone = async (step: PlanStep) => {
    if (!data || (role !== "intern" && role !== "buddy")) return;
    const internId = role === "intern" ? data.me.id : selectedIntern;
    if (!internId) return;
    await perform(`/api/plan-steps/${step.id}`, "PATCH", { intern_id: internId }, "Шаг отмечен выполненным");
  };
  const look: Lookup = {
    userName: (id) => (data?.users ?? users).find((u) => u.id === id)?.name ?? id,
    systemTitle: (id) => data?.services?.access.systems?.find((x) => x.system === id)?.title ?? id,
    stepTitle: (id) => data?.plan?.steps.find((x) => x.id === id)?.title ?? id,
  };
  const decideRequest = async (request: RequestItem, decision: "approve" | "reject") => {
    if (role !== "lead" || deciding) return;
    const owner = login, epoch = generation.current;
    setDeciding(true);
    await perform(`/api/requests/${request.id}/decision`, "POST", {
      decision, ...(decision === "reject" ? { reason, comment: comment.trim() } : {}),
    }, "Решение сохранено");
    if (isCurrent(owner, epoch)) setDeciding(false);
  };

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Box sx={{ minHeight: "100vh", bgcolor: "#fff" }}>
        <Box sx={{ height: 68, bgcolor: "#b9daf7", display: "flex", alignItems: "center", overflow: "hidden", whiteSpace: "nowrap" }}>
          <Typography sx={{ fontSize: { xs: 18, md: 21 }, wordSpacing: 5, animation: "ticker 28s linear infinite", "@keyframes ticker": { from: { transform: "translateX(0)" }, to: { transform: "translateX(-50%)" } } }}>
            Подпишись на новости Напарника　·　Подпишись на новости Напарника　·　Подпишись на новости Напарника　·　Подпишись на новости Напарника　·　Подпишись на новости Напарника　·　Подпишись на новости Напарника　·　
          </Typography>
        </Box>
        <AppBar position="sticky" elevation={0} color="transparent" sx={{ bgcolor: "#fff", borderBottom: "1px solid #dedede" }}>
          <Container maxWidth="xl">
            <Toolbar disableGutters sx={{ gap: { xs: 1, lg: 3 }, minHeight: 76, flexWrap: { xs: "wrap", md: "nowrap" }, py: { xs: 1, md: 0 } }}>
              <Box component="img" src={avitoLogo} alt="Avito" sx={{ width: { xs: 82, md: 104 }, height: 42, objectFit: "contain" }} />
              <Divider orientation="vertical" flexItem sx={{ my: 2.2 }} />
              <Typography fontWeight={800} sx={{ display: { xs: "none", sm: "block" }, whiteSpace: "nowrap", fontSize: 22 }}>Start · Напарник</Typography>
              <Tabs value={data ? view : false} onChange={(_, value: View) => setView(value)} variant="scrollable" scrollButtons="auto" allowScrollButtonsMobile sx={{ flex: { xs: "1 0 100%", md: 1 }, order: { xs: 1, md: 0 }, minWidth: 0, "& .MuiTabs-indicator": { bgcolor: "#000", height: 3 }, "& .MuiTab-root": { minWidth: 76, px: { xs: 1, md: 2 }, color: "#161616" }, "& .Mui-selected": { color: "#000 !important", fontWeight: 800 } }}>
                {visibleItems.map((item) => <Tab key={item.value} value={item.value} label={item.label} icon={item.icon} iconPosition="start" />)}
              </Tabs>
              {role && role !== "intern" && <Tooltip title="Спросить Напарника"><IconButton aria-label="Спросить Напарника" onClick={() => setChatOpen(true)}><ForumRounded /></IconButton></Tooltip>}
              <Tooltip title="Уведомления"><IconButton><Badge color="error" variant="dot"><NotificationsNoneRounded /></Badge></IconButton></Tooltip>
              <FormControl size="small" sx={{ minWidth: { xs: 92, md: 210 } }}>
                <InputLabel id="account-label">Учётка</InputLabel>
                <Select labelId="account-label" label="Учётка" value={login} onChange={(event) => switchUser(event.target.value)}>
                  {(users.length ? users : [{ login, name: login }]).map((user) => <MenuItem key={user.login} value={user.login}>{user.name}</MenuItem>)}
                </Select>
              </FormControl>
            </Toolbar>
          </Container>
        </AppBar>

        {error && <Container maxWidth="xl" sx={{ mt: 3 }}><Alert severity="error" action={<Button onClick={() => void load()}>Повторить</Button>}>{error}</Alert></Container>}
        {loading && <LinearProgress aria-label="Загрузка данных" />}
        {data && data.me.login === login && <Container maxWidth="xl" sx={{ pt: { xs: 7, md: 12 }, pb: 14 }}>
          {view === "day" && role === "intern" && <DayView data={data} onOpenChat={(text) => { setChatText(text); setChatOpen(true); }} onMarkDone={markDone} />}
          {view === "day" && role === "lead" && <TeamDashboardView overview={data.overview!} onRequests={() => setView("requests")} />}
          {view === "day" && role === "buddy" && <BuddyDashboardView data={data} onPlan={(id) => { setSelectedIntern(id); setView("plan"); }} />}
          {view === "day" && role === "expert" && <QuestionsView data={data} onAsk={() => setChatOpen(true)} />}
          {view === "plan" && role === "intern" && <PlanView plan={data.plan} week={data.me.week} title="Мой план" onMarkDone={markDone} />}
          {view === "plan" && role === "buddy" && <>
            <FormControl sx={{ mb: 3, minWidth: 240 }}><InputLabel id="intern-label">Стажёр</InputLabel><Select labelId="intern-label" label="Стажёр" value={selectedIntern} onChange={(e) => setSelectedIntern(e.target.value)}>{data.managedPlans.map((i) => <MenuItem key={i.id} value={i.id}>{i.name}</MenuItem>)}</Select></FormControl>
            <PlanView plan={data.managedPlans.find((i) => i.id === selectedIntern)?.plan} title={`План: ${data.managedPlans.find((i) => i.id === selectedIntern)?.name ?? "выбери стажёра"}`} onMarkDone={markDone} />
          </>}
          {view === "questions" && <QuestionsView data={data} onAsk={() => setChatOpen(true)} />}
          {view === "knowledge" && <KnowledgeView data={data} onOpen={setSelectedPage} />}
          {view === "requests" && (role === "lead" || role === "intern") && <RequestsView data={data} deciding={deciding} onApprove={(r) => void decideRequest(r, "approve")} onReject={(r) => { setReason(""); setComment(""); setRejection(r); }} onCreate={() => { setChatText("Мне нужен отгул "); setChatOpen(true); }} />}
        </Container>}

        {role === "intern" && <Tooltip title="Спросить Напарника" placement="left">
          <IconButton onClick={() => setChatOpen(true)} sx={{ position: "fixed", right: { xs: 18, md: 30 }, bottom: { xs: 18, md: 30 }, width: { xs: 82, md: 104 }, height: { xs: 82, md: 104 }, zIndex: 20, bgcolor: "#eaf5ff", boxShadow: "0 18px 45px rgba(0,100,220,.22)", transition: "transform .2s", "&:hover": { bgcolor: "#eaf5ff", transform: "scale(1.08) rotate(-2deg)" } }}>
            <Box component="img" src={mascotCat} alt="Маскот Напарника" sx={{ width: "124%", maxWidth: "none" }} />
          </IconButton>
        </Tooltip>}

        <ChatDrawer open={chatOpen} onClose={() => setChatOpen(false)} text={chatText} setText={setChatText} messages={messages} loading={chatLoading}
          onSend={(retry) => void sendChat(retry)} login={login} look={look} onAction={() => void load()} />
        <Dialog open={Boolean(selectedPage)} onClose={() => setSelectedPage(null)} maxWidth="sm" fullWidth>
          <DialogTitle>{selectedPage?.title}</DialogTitle>
          <DialogContent><Typography color="text.secondary" mb={2}>{selectedPage?.author} · {selectedPage?.updated_at}</Typography>{selectedPage && <Markdown text={selectedPage.body || selectedPage.excerpt} />}{selectedPage?.test_instruction && <Chip label="Тестовая инструкция" color="warning" size="small" sx={{ mt: 3 }} />}</DialogContent>
          <DialogActions><Button onClick={() => setSelectedPage(null)}>Закрыть</Button></DialogActions>
        </Dialog>
        <Dialog open={Boolean(rejection) && role === "lead"} onClose={() => !deciding && setRejection(null)} fullWidth maxWidth="sm">
          <DialogTitle>Отклонить заявку {rejection?.id}</DialogTitle>
          <DialogContent><Stack spacing={2} sx={{ pt: 1 }}>
            <FormControl fullWidth><InputLabel id="reason-label">Причина отказа</InputLabel><Select labelId="reason-label" label="Причина отказа" value={reason} onChange={(e) => setReason(e.target.value)}>{(rejection?.kind === "access" ? data?.services?.access.reason_codes : data?.services?.absence.reason_codes)?.map((r) => <MenuItem key={r.code} value={r.code}>{r.text}</MenuItem>)}</Select></FormControl>
            <TextField label="Комментарий" value={comment} onChange={(e) => setComment(e.target.value)} required={reason === "OTHER"} multiline />
          </Stack></DialogContent>
          <DialogActions><Button disabled={deciding} onClick={() => setRejection(null)}>Отмена</Button><Button disabled={deciding || !reason || (reason === "OTHER" && !comment.trim())} onClick={() => rejection && void decideRequest(rejection, "reject")}>Отклонить</Button></DialogActions>
        </Dialog>
        <Snackbar open={Boolean(snack)} autoHideDuration={2500} onClose={() => setSnack("")} message={snack} />
      </Box>
    </ThemeProvider>
  );
}

const TeamDashboardView = ({ overview, onRequests }: { overview: TeamOverview; onRequests: () => void }) => <>
  <SectionTitle eyebrow={`Руководитель · ${overview.department}`} title="Обзор команды" text="Прогресс стажёров, просрочки и заявки на согласование." />
  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 2, mb: 4 }}>
    {[['Стажёры', overview.totals.interns], ['Просроченные шаги', overview.totals.overdue_steps], ['Открытые вопросы', overview.totals.open_questions], ['На согласовании', overview.totals.pending_requests]].map(([label, value]) => <Card key={label}><CardContent><Typography color="text.secondary">{label}</Typography><Typography variant="h3">{value}</Typography></CardContent></Card>)}
  </Box>
  <Typography variant="h4" mb={2}>Стажёры команды</Typography>
  <Stack spacing={2} mb={4}>{overview.interns.map((intern) => <Card key={intern.id}><CardContent>
    <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" gap={1}><Box><Typography variant="h5">{intern.name}</Typography><Typography color="text.secondary">Бадди: {intern.buddy}</Typography></Box><Chip label={intern.plan_status === "approved" ? "План утверждён" : intern.plan_status === "draft" ? "План готовится" : "План не утверждён"} /></Stack>
    <Typography mt={2}>{intern.stage || (intern.plan_status === "approved" ? "Этапы завершены" : "План готовится")}</Typography>
    <LinearProgress aria-label={`Прогресс ${intern.name}`} variant="determinate" value={intern.progress_pct} sx={{ my: 1, height: 8 }} />
    <Typography color="text.secondary">Прогресс: {intern.progress_pct}% · Просрочки: {intern.overdue_steps} · Открытые вопросы: {intern.open_questions}</Typography>
  </CardContent></Card>)}{!overview.interns.length && <EmptyState>Подчинённых стажёров пока нет</EmptyState>}</Stack>
  <Stack direction="row" justifyContent="space-between" alignItems="center" gap={2}><Typography variant="h4">Заявки на согласовании</Typography><Button onClick={onRequests}>Открыть согласование</Button></Stack>
  <Stack spacing={1} mt={2}>{overview.requests.filter((r) => r.status === "pending").map((r) => <Paper variant="outlined" key={r.id} sx={{ p: 2 }}><Typography fontWeight={700}>{r.employee_name} · {r.kind === "access" ? r.title || "Доступ" : `${absenceLabel(r.type)} · ${r.date ?? ""}`}</Typography><Typography color="text.secondary">{r.id}</Typography></Paper>)}{!overview.totals.pending_requests && <EmptyState>Нет заявок, ожидающих решения</EmptyState>}</Stack>
</>;

const BuddyDashboardView = ({ data, onPlan }: { data: DataState; onPlan: (id: string) => void }) => <>
  <SectionTitle eyebrow={`Бадди · ${data.me.department}`} title="Мои стажёры" text="Планы и прогресс закреплённых за тобой стажёров." />
  <Stack spacing={2}>{data.managedPlans.map((intern) => <Card key={intern.id}><CardContent><Typography variant="h5">{intern.name}</Typography><Typography color="text.secondary" my={1}>{intern.plan ? `Прогресс: ${intern.plan.progress_pct}%` : "План готовится"}</Typography>{intern.plan && <Typography mb={1}>Следующий шаг: {intern.plan.next_step?.title ?? "Все шаги пройдены"}</Typography>}<Button onClick={() => onPlan(intern.id)}>Открыть план</Button></CardContent></Card>)}{!data.managedPlans.length && <EmptyState>За тобой пока не закреплены стажёры</EmptyState>}</Stack>
</>;

const DayView = ({ data, onOpenChat, onMarkDone }: { data: DataState; onOpenChat: (text: string) => void; onMarkDone: (step: PlanStep) => void }) => {
  const firstName = data.me.name.split(" ")[0];
  const steps = data.today?.steps ?? [];
  const events = data.today?.events ?? [];
  return <>
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1.35fr .65fr" }, alignItems: "end", gap: 3, mb: 5 }}>
      <SectionTitle eyebrow={new Date(`${data.me.today}T12:00:00`).toLocaleDateString("ru-RU", { weekday: "long", day: "numeric", month: "long" })} title={`Привет, ${firstName}`} text="Собрали главное на сегодня — без лишнего шума и долгого поиска." />
      <Paper sx={{ p: 3, bgcolor: "#ffec68", justifySelf: { md: "end" }, width: "100%", maxWidth: 390 }}>
        <Typography variant="overline" fontWeight={800}>Твой контекст</Typography><Typography variant="h5" fontWeight={800}>Неделя {data.me.week} · {data.me.department}</Typography>
      </Paper>
    </Box>
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1.45fr .75fr" }, gap: 3 }}>
      <Stack spacing={3}>
        <Card><CardContent sx={{ p: { xs: 2.5, md: 4 } }}><Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}><Typography variant="h4" fontWeight={800}>Сегодня</Typography><Chip label={`${steps.length} шага`} /></Stack>{steps.length ? steps.map((step) => <Stack key={step.id} direction="row" spacing={2} alignItems="center" sx={{ py: 2, borderTop: "1px solid #eee" }}><Checkbox inputProps={{ "aria-label": step.title }} checked={step.status === "Done"} onChange={() => void onMarkDone(step)} /><Box flex={1}><Typography fontWeight={700}>{step.title}</Typography><Typography variant="body2" color="text.secondary">До {step.due_date}</Typography></Box><StatusChip status={step.status} /><Button onClick={() => onOpenChat(`Помоги с шагом: ${step.title}`)}>Помоги</Button></Stack>) : <EmptyState>На сегодня всё готово</EmptyState>}</CardContent></Card>
        <ProgressCard plan={data.plan} />
      </Stack>
      <Stack spacing={3}>
        <Card sx={{ bgcolor: "#eaf2ff" }}><CardContent sx={{ p: 3 }}><Typography variant="h5" fontWeight={800} mb={2}>Ближайшее</Typography>{events.length ? events.map((event) => <Stack key={`${event.start}-${event.title}`} direction="row" spacing={2} py={1.5}><Typography variant="h5" color="primary" fontWeight={800}>{event.start}</Typography><Typography fontWeight={700}>{event.title}</Typography></Stack>) : <Typography color="text.secondary">Событий нет</Typography>}</CardContent></Card>
        <Card><CardContent sx={{ p: 3 }}><Typography variant="h5" fontWeight={800} mb={1}>Вопросы</Typography>{data.questions.slice(0, 3).map((question) => <Stack key={question.id} direction="row" justifyContent="space-between" gap={1} py={1.5} borderTop="1px solid #eee"><Box><Typography fontWeight={700}>{question.text_hidden ? "Текст скрыт" : question.title || question.text}</Typography><Typography variant="caption" color="text.secondary">{question.addressee}</Typography></Box><StatusChip status={question.status} /></Stack>)}{!data.questions.length && <Typography color="text.secondary">Пока нет вопросов людям</Typography>}</CardContent></Card>
      </Stack>
    </Box>
  </>;
};

const ProgressCard = ({ plan }: { plan?: PlanData }) => <Card sx={{ bgcolor: "#fff" }}><CardContent sx={{ p: { xs: 3, md: 4 } }}><Stack direction="row" justifyContent="space-between"><Box><Typography variant="overline" color="text.secondary">Путь адаптации</Typography><Typography variant="h2" sx={{ fontSize: 48 }}>{plan?.progress_pct ?? 0}%</Typography></Box><CheckCircleRounded color="success" sx={{ fontSize: 38 }} /></Stack><LinearProgress variant="determinate" value={plan?.progress_pct ?? 0} sx={{ height: 10, borderRadius: 5, my: 2 }} /><Typography color="text.secondary">{!plan ? "План готовится" : plan.next_step ? `Дальше: ${plan.next_step.title}` : "Все шаги пройдены"}</Typography></CardContent></Card>;

const PlanView = ({ plan, week, title, onMarkDone }: { plan?: PlanData; week?: number; title: string; onMarkDone: (step: PlanStep) => void }) => <>
  <SectionTitle eyebrow={week ? `Неделя ${week}` : "Адаптация стажёра"} title={title} text="Понятный маршрут от знакомства с командой до самостоятельных задач." />
  <Stack spacing={2}>{plan?.stages.map((stage, index) => { const steps = plan?.steps.filter((step) => step.stage === stage.id && step.owner === "intern") ?? []; return <Card key={stage.id}><CardContent sx={{ p: { xs: 3, md: 4 } }}><Stack direction={{ xs: "column", sm: "row" }} gap={2}><Avatar sx={{ bgcolor: index === 0 ? "success.main" : "primary.main", fontWeight: 800 }}>{index + 1}</Avatar><Box flex={1}><Typography variant="h5" fontWeight={800}>{stage.title}</Typography><Typography color="text.secondary" mb={2}>{steps.filter((step) => step.status === "Done").length} из {steps.length} шагов</Typography><Stack spacing={1}>{steps.map((step) => <Paper key={step.id} variant="outlined" sx={{ p: 2, display: "flex", alignItems: "center", gap: 2, borderRadius: 4 }}><Checkbox inputProps={{ "aria-label": step.title }} checked={step.status === "Done"} onChange={() => void onMarkDone(step)} /><Typography flex={1} fontWeight={650}>{step.title}</Typography><StatusChip status={step.status} /></Paper>)}</Stack></Box></Stack></CardContent></Card>; })}{!plan && <EmptyState>План пока не утверждён</EmptyState>}</Stack>
  </>;

const QuestionsView = ({ data, onAsk }: { data: DataState; onAsk: () => void }) => <>
  <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" alignItems={{ sm: "end" }} mb={4}><SectionTitle eyebrow={`${data.questions.length} вопросов`} title={roleLabel(primaryRole(data.me.roles), "questions")} text="Вопросы и ответы, доступные твоей роли." />{primaryRole(data.me.roles) === "intern" && <Button variant="contained" size="large" startIcon={<ForumRounded />} onClick={onAsk}>Задать вопрос</Button>}</Stack>
  <Stack spacing={2}>{data.questions.map((question) => <Card key={question.id}><CardContent sx={{ p: 3 }}><Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" gap={2}><Box><Stack direction="row" gap={1} alignItems="center"><Typography variant="h6" fontWeight={800}>{question.text_hidden ? "Текст вопроса скрыт настройками стажёра" : question.title || question.text}</Typography>{question.urgent && <Chip label="Срочно" color="error" size="small" />}</Stack><Typography color="text.secondary">Адресат: {question.addressee}</Typography>{question.answer && <Paper sx={{ p: 2, mt: 1.5, bgcolor: "#e4f6eb" }}><Typography fontWeight={800}>Ответ: {question.addressee}</Typography><Markdown text={question.answer} /></Paper>}</Box><StatusChip status={question.status} /></Stack></CardContent></Card>)}{!data.questions.length && <EmptyState>{primaryRole(data.me.roles) === "intern" ? "Спроси Напарника — он сначала проверит базу" : "Входящих вопросов пока нет"}</EmptyState>}</Stack>
  </>;

const KnowledgeView = ({ data, onOpen }: { data: DataState; onOpen: (page: KbPage) => void }) => <>
  <SectionTitle eyebrow={data.me.department} title="База знаний" text="Материалы твоей команды с авторами, датами и честной маркировкой демо-инструкций." />
  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "230px 1fr" }, gap: 3 }}><Card><CardContent><Typography fontWeight={800} mb={2}>Папки</Typography><List disablePadding>{data.folders.map((folder) => <ListItem key={folder.id} disablePadding><ListItemButton><ListItemAvatar><Avatar sx={{ bgcolor: "#f0ebff", color: "secondary.main" }}><FolderRounded /></Avatar></ListItemAvatar><ListItemText primary={folder.title} /></ListItemButton></ListItem>)}</List></CardContent></Card><Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", xl: "repeat(3, 1fr)" }, gap: 2 }}>{data.pages.map((page) => <Card key={page.id} sx={{ minHeight: 240 }}><CardActionArea onClick={() => onOpen(page)} sx={{ height: "100%" }}><CardContent sx={{ p: 3, height: "100%", display: "flex", flexDirection: "column" }}><Avatar sx={{ bgcolor: "#e8f6ff", color: "primary.main", mb: 3 }}><MenuBookRounded /></Avatar><Typography variant="h6" fontWeight={800}>{page.title}</Typography><Typography color="text.secondary" my={1}>{page.excerpt}</Typography><Box mt="auto"><Typography variant="caption" color="text.secondary">{page.author} · {page.updated_at}</Typography>{page.test_instruction && <Chip label="Тестовая инструкция" color="warning" size="small" sx={{ mt: 1, display: "flex", width: "fit-content" }} />}</Box></CardContent></CardActionArea></Card>)}</Box></Box>
  </>;

const RequestsView = ({ data, onCreate, onApprove, onReject, deciding }: { data: DataState; onCreate: () => void; onApprove: (r: RequestItem) => void; onReject: (r: RequestItem) => void; deciding: boolean }) => {
  const isLead = primaryRole(data.me.roles) === "lead";
  return <>
    <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" alignItems={{ sm: "end" }} mb={4}><SectionTitle eyebrow="Корпоративные сервисы" title={isLead ? "Согласование заявок" : "Мои заявки"} text={isLead ? "Заявки стажёров, по которым ты принимаешь решение." : "Сначала черновик и проверка, затем подтверждение и квитанция."} />{!isLead && <Button variant="contained" size="large" startIcon={<EventAvailableRounded />} onClick={onCreate}>Новая заявка</Button>}</Stack>
    <Stack spacing={2}>{data.requests.map((request) => <Card key={request.id}><CardContent sx={{ p: 3 }}><Stack direction="row" alignItems="center" spacing={2}><Avatar sx={{ bgcolor: request.kind === "absence" ? "#fff3d6" : "#e8f6ff", color: "text.primary" }}>{request.kind === "absence" ? <CalendarMonthRounded /> : <OpenInNewRounded />}</Avatar><Box flex={1}><Typography fontWeight={800}>{request.kind === "absence" ? `${absenceLabel(request.type)} · ${request.date ? new Date(`${request.date}T12:00:00`).toLocaleDateString("ru-RU", { day: "numeric", month: "long" }) : ""}` : `Доступ: ${request.title || request.system}`}</Typography>{isLead && <Typography>{data.users.find((u) => u.id === request.employee_id)?.name ?? "Стажёр"}</Typography>}<Typography variant="body2" color="text.secondary">{request.id} · {data.users.find((u) => u.id === request.employee_id)?.name ?? request.employee_id} · {request.source === "self" ? "оформлена сам" : "через Напарника"} · {new Date(request.created_at).toLocaleString("ru-RU", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}</Typography></Box><StatusChip status={request.status} /></Stack>{isLead && request.approver_id === data.me.id && request.status === "pending" && <Stack direction="row" gap={1} mt={2}><Button disabled={deciding} variant="contained" onClick={() => onApprove(request)}>{request.kind === "access" ? "Выдать доступ" : "Согласовать"}</Button><Button disabled={deciding} color="error" onClick={() => onReject(request)}>Отклонить</Button></Stack>}</CardContent></Card>)}{!data.requests.length && <EmptyState>{isLead ? "Заявок на согласование пока нет" : "Заявок пока нет"}</EmptyState>}</Stack>
  </>;
};

export default App;
