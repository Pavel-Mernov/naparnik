import { useCallback, useEffect, useState, type ReactElement, type ReactNode } from "react";
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
  CircularProgress,
  Container,
  CssBaseline,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Drawer,
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
  CloseRounded,
  DescriptionRounded,
  EventAvailableRounded,
  FolderRounded,
  ForumRounded,
  HomeRounded,
  MenuBookRounded,
  NotificationsNoneRounded,
  OpenInNewRounded,
  SendRounded,
  TaskAltRounded,
} from "@mui/icons-material";
import avitoLogo from "../avito-logo.svg";
import mascotCat from "../mascot-cat.png";
import {
  api,
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
};
type ChatMessage = { role: "user" | "assistant"; text: string; reply?: ChatReply };

const viewItems: Array<{ value: View; label: string; icon: ReactElement }> = [
  { value: "day", label: "Мой день", icon: <HomeRounded /> },
  { value: "plan", label: "Мой план", icon: <TaskAltRounded /> },
  { value: "questions", label: "Вопросы", icon: <ForumRounded /> },
  { value: "knowledge", label: "База знаний", icon: <MenuBookRounded /> },
  { value: "requests", label: "Заявки", icon: <DescriptionRounded /> },
];

const statusInfo: Record<string, { label: string; color: "default" | "primary" | "success" | "warning" | "error" }> = {
  Done: { label: "Готово", color: "success" },
  Today: { label: "На сегодня", color: "primary" },
  Planned: { label: "Запланировано", color: "default" },
  Overdue: { label: "Просрочено", color: "error" },
  pending: { label: "На согласовании", color: "warning" },
  approved: { label: "Согласовано", color: "success" },
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
  const [view, setView] = useState<View>("day");
  const [data, setData] = useState<DataState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [chatOpen, setChatOpen] = useState(false);
  const [chatText, setChatText] = useState("");
  const [chatLoading, setChatLoading] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: "assistant", text: "Привет! Я помогу с планом, вопросами команды и организационными действиями." },
  ]);
  const [selectedPage, setSelectedPage] = useState<KbPage | null>(null);
  const [snack, setSnack] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [me, usersPayload] = await Promise.all([
        api<Me>(login, "/api/me"),
        api<{ users: User[] }>(login, "/api/users"),
      ]);
      const internId = me.roles.includes("intern") ? me.id : me.interns[0]?.id;
      const [todayResult, planResult, questionResult, kbResult, requestResult] = await Promise.allSettled([
        me.roles.includes("intern") ? api<{ ok: boolean; data: TodayData }>(login, "/api/today") : Promise.resolve(undefined),
        internId ? api<{ ok: boolean; data: PlanData }>(login, `/api/plans/${internId}`) : Promise.resolve(undefined),
        api<{ questions: Question[] }>(login, "/api/questions"),
        api<{ folders: KbFolder[]; pages: KbPage[] }>(login, "/api/kb/pages"),
        api<{ requests: RequestItem[] }>(login, "/api/requests"),
      ]);
      const value = <T,>(result: PromiseSettledResult<T>): T | undefined => result.status === "fulfilled" ? result.value : undefined;
      const todayEnvelope = value(todayResult);
      const planEnvelope = value(planResult);
      setData({
        me,
        users: usersPayload.users ?? [],
        today: todayEnvelope ? unwrap(todayEnvelope) : undefined,
        plan: planEnvelope ? unwrap(planEnvelope) : undefined,
        questions: value(questionResult)?.questions ?? [],
        folders: value(kbResult)?.folders ?? [],
        pages: value(kbResult)?.pages ?? [],
        requests: value(requestResult)?.requests ?? [],
      });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить данные");
    } finally {
      setLoading(false);
    }
  }, [login]);

  useEffect(() => { void load(); }, [load]);

  const switchUser = (nextLogin: string) => {
    localStorage.setItem("naparnik.login", nextLogin);
    setLogin(nextLogin);
    setMessages([{ role: "assistant", text: "Учётка переключена. Чем помочь?" }]);
  };

  const sendChat = async () => {
    const text = chatText.trim();
    if (!text || chatLoading) return;
    setChatText("");
    setMessages((current) => [...current, { role: "user", text }]);
    setChatLoading(true);
    try {
      const reply = await api<ChatReply>(login, "/api/chat/messages", { method: "POST", body: JSON.stringify({ text }) });
      setMessages((current) => [...current, { role: "assistant", text: reply.text, reply }]);
    } catch (reason) {
      setMessages((current) => [...current, { role: "assistant", text: reason instanceof Error ? reason.message : "Не получилось ответить" }]);
    } finally {
      setChatLoading(false);
    }
  };

  const markDone = async (step: PlanStep) => {
    try {
      await api(login, `/api/plan-steps/${step.id}`, { method: "PATCH", body: "{}" });
      setSnack("Шаг отмечен выполненным");
      await load();
    } catch (reason) {
      setSnack(reason instanceof Error ? reason.message : "Не получилось обновить шаг");
    }
  };

  const confirmDraft = async (id: string) => {
    try {
      await api(login, `/api/drafts/${id}/confirm`, { method: "POST", body: "{}" });
      setSnack("Действие подтверждено");
      await load();
    } catch (reason) {
      setSnack(reason instanceof Error ? reason.message : "Не получилось подтвердить действие");
    }
  };

  if (loading && !data) {
    return <ThemeProvider theme={theme}><CssBaseline /><Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}><CircularProgress /></Box></ThemeProvider>;
  }

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
            <Toolbar disableGutters sx={{ gap: { xs: 1, lg: 3 }, minHeight: 76 }}>
              <Box component="img" src={avitoLogo} alt="Avito" sx={{ width: { xs: 82, md: 104 }, height: 42, objectFit: "contain" }} />
              <Divider orientation="vertical" flexItem sx={{ my: 2.2 }} />
              <Typography fontWeight={800} sx={{ display: { xs: "none", sm: "block" }, whiteSpace: "nowrap", fontSize: 22 }}>Start · Напарник</Typography>
              <Tabs value={view} onChange={(_, value: View) => setView(value)} variant="scrollable" scrollButtons={false} sx={{ flex: 1, minWidth: 0, "& .MuiTabs-indicator": { bgcolor: "#000", height: 3 }, "& .MuiTab-root": { minWidth: 76, px: { xs: 1, md: 2 }, color: "#161616" }, "& .Mui-selected": { color: "#000 !important", fontWeight: 800 } }}>
                {viewItems.map((item) => <Tab key={item.value} value={item.value} label={item.label} icon={item.icon} iconPosition="start" />)}
              </Tabs>
              <Button variant="contained" onClick={() => setChatOpen(true)} sx={{ display: { xs: "none", xl: "inline-flex" }, bgcolor: "#000", color: "#fff", px: 3, "&:hover": { bgcolor: "#222" } }}>Следить за анонсами</Button>
              <Tooltip title="Уведомления"><IconButton><Badge color="error" variant="dot"><NotificationsNoneRounded /></Badge></IconButton></Tooltip>
              <FormControl size="small" sx={{ minWidth: { xs: 92, md: 210 } }}>
                <InputLabel>Учётка</InputLabel>
                <Select label="Учётка" value={login} onChange={(event) => switchUser(event.target.value)}>
                  {(data?.users ?? []).map((user) => <MenuItem key={user.login} value={user.login}>{user.name}</MenuItem>)}
                </Select>
              </FormControl>
            </Toolbar>
          </Container>
        </AppBar>

        {error && <Container maxWidth="xl" sx={{ mt: 3 }}><Alert severity="error" action={<Button onClick={() => void load()}>Повторить</Button>}>{error}</Alert></Container>}
        {data && <Container maxWidth="xl" sx={{ pt: { xs: 7, md: 12 }, pb: 14 }}>
          {view === "day" && <DayView data={data} onOpenChat={(text) => { setChatText(text); setChatOpen(true); }} onMarkDone={markDone} />}
          {view === "plan" && <PlanView data={data} onMarkDone={markDone} />}
          {view === "questions" && <QuestionsView data={data} onAsk={() => setChatOpen(true)} />}
          {view === "knowledge" && <KnowledgeView data={data} onOpen={setSelectedPage} />}
          {view === "requests" && <RequestsView data={data} onCreate={() => { setChatText("Мне нужен отгул "); setChatOpen(true); }} />}
        </Container>}

        <Tooltip title="Спросить Напарника" placement="left">
          <IconButton onClick={() => setChatOpen(true)} sx={{ position: "fixed", right: { xs: 18, md: 30 }, bottom: { xs: 18, md: 30 }, width: { xs: 82, md: 104 }, height: { xs: 82, md: 104 }, zIndex: 20, bgcolor: "#eaf5ff", boxShadow: "0 18px 45px rgba(0,100,220,.22)", transition: "transform .2s", "&:hover": { bgcolor: "#eaf5ff", transform: "scale(1.08) rotate(-2deg)" } }}>
            <Box component="img" src={mascotCat} alt="Маскот Напарника" sx={{ width: "124%", maxWidth: "none" }} />
          </IconButton>
        </Tooltip>

        <ChatDrawer open={chatOpen} onClose={() => setChatOpen(false)} text={chatText} setText={setChatText} messages={messages} loading={chatLoading} onSend={sendChat} onConfirm={confirmDraft} />
        <Dialog open={Boolean(selectedPage)} onClose={() => setSelectedPage(null)} maxWidth="sm" fullWidth>
          <DialogTitle>{selectedPage?.title}</DialogTitle>
          <DialogContent><Typography color="text.secondary" mb={2}>{selectedPage?.author} · {selectedPage?.updated_at}</Typography><Typography>{selectedPage?.excerpt}</Typography>{selectedPage?.test_instruction && <Chip label="Тестовая инструкция" color="warning" size="small" sx={{ mt: 3 }} />}</DialogContent>
          <DialogActions><Button onClick={() => setSelectedPage(null)}>Закрыть</Button></DialogActions>
        </Dialog>
        <Snackbar open={Boolean(snack)} autoHideDuration={2500} onClose={() => setSnack("")} message={snack} />
      </Box>
    </ThemeProvider>
  );
}

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
        <Card><CardContent sx={{ p: { xs: 2.5, md: 4 } }}><Stack direction="row" justifyContent="space-between" alignItems="center" mb={2}><Typography variant="h4" fontWeight={800}>Сегодня</Typography><Chip label={`${steps.length} шага`} /></Stack>{steps.length ? steps.map((step) => <Stack key={step.id} direction="row" spacing={2} alignItems="center" sx={{ py: 2, borderTop: "1px solid #eee" }}><Checkbox checked={step.status === "Done"} onChange={() => void onMarkDone(step)} /><Box flex={1}><Typography fontWeight={700}>{step.title}</Typography><Typography variant="body2" color="text.secondary">До {step.due_date}</Typography></Box><StatusChip status={step.status} /><Button onClick={() => onOpenChat(`Помоги с шагом: ${step.title}`)}>Помоги</Button></Stack>) : <EmptyState>На сегодня всё готово</EmptyState>}</CardContent></Card>
        <ProgressCard plan={data.plan} />
      </Stack>
      <Stack spacing={3}>
        <Card sx={{ bgcolor: "#eaf2ff" }}><CardContent sx={{ p: 3 }}><Typography variant="h5" fontWeight={800} mb={2}>Ближайшее</Typography>{events.length ? events.map((event) => <Stack key={`${event.start}-${event.title}`} direction="row" spacing={2} py={1.5}><Typography variant="h5" color="primary" fontWeight={800}>{event.start}</Typography><Typography fontWeight={700}>{event.title}</Typography></Stack>) : <Typography color="text.secondary">Событий нет</Typography>}</CardContent></Card>
        <Card><CardContent sx={{ p: 3 }}><Typography variant="h5" fontWeight={800} mb={1}>Вопросы</Typography>{data.questions.slice(0, 3).map((question) => <Stack key={question.id} direction="row" justifyContent="space-between" gap={1} py={1.5} borderTop="1px solid #eee"><Box><Typography fontWeight={700}>{question.text || "Текст скрыт"}</Typography><Typography variant="caption" color="text.secondary">{question.addressee}</Typography></Box><StatusChip status={question.status} /></Stack>)}{!data.questions.length && <Typography color="text.secondary">Пока нет вопросов людям</Typography>}</CardContent></Card>
      </Stack>
    </Box>
  </>;
};

const ProgressCard = ({ plan }: { plan?: PlanData }) => <Card sx={{ bgcolor: "#fff" }}><CardContent sx={{ p: { xs: 3, md: 4 } }}><Stack direction="row" justifyContent="space-between"><Box><Typography variant="overline" color="text.secondary">Путь адаптации</Typography><Typography variant="h2" sx={{ fontSize: 48 }}>{plan?.progress_pct ?? 0}%</Typography></Box><CheckCircleRounded color="success" sx={{ fontSize: 38 }} /></Stack><LinearProgress variant="determinate" value={plan?.progress_pct ?? 0} sx={{ height: 10, borderRadius: 5, my: 2 }} /><Typography color="text.secondary">{plan?.next_step ? `Дальше: ${plan.next_step.title}` : "Все шаги пройдены"}</Typography></CardContent></Card>;

const PlanView = ({ data, onMarkDone }: { data: DataState; onMarkDone: (step: PlanStep) => void }) => <>
  <SectionTitle eyebrow={`Неделя ${data.me.week}`} title="Мой план" text="Понятный маршрут от знакомства с командой до самостоятельных задач." />
  <Stack spacing={2}>{data.plan?.stages.map((stage, index) => { const steps = data.plan?.steps.filter((step) => step.stage === stage.id && step.owner === "intern") ?? []; return <Card key={stage.id}><CardContent sx={{ p: { xs: 3, md: 4 } }}><Stack direction={{ xs: "column", sm: "row" }} gap={2}><Avatar sx={{ bgcolor: index === 0 ? "success.main" : "primary.main", fontWeight: 800 }}>{index + 1}</Avatar><Box flex={1}><Typography variant="h5" fontWeight={800}>{stage.title}</Typography><Typography color="text.secondary" mb={2}>{steps.filter((step) => step.status === "Done").length} из {steps.length} шагов</Typography><Stack spacing={1}>{steps.map((step) => <Paper key={step.id} variant="outlined" sx={{ p: 2, display: "flex", alignItems: "center", gap: 2, borderRadius: 4 }}><Checkbox checked={step.status === "Done"} onChange={() => void onMarkDone(step)} /><Typography flex={1} fontWeight={650}>{step.title}</Typography><StatusChip status={step.status} /></Paper>)}</Stack></Box></Stack></CardContent></Card>; })}{!data.plan && <EmptyState>План пока не утверждён</EmptyState>}</Stack>
  </>;

const QuestionsView = ({ data, onAsk }: { data: DataState; onAsk: () => void }) => <>
  <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" alignItems={{ sm: "end" }} mb={4}><SectionTitle eyebrow={`${data.questions.length} вопросов`} title="Вопросы" text="Маршрут каждого вопроса прозрачен: база, эксперт или наставник." /><Button variant="contained" size="large" startIcon={<ForumRounded />} onClick={onAsk}>Задать вопрос</Button></Stack>
  <Stack spacing={2}>{data.questions.map((question) => <Card key={question.id}><CardContent sx={{ p: 3 }}><Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" gap={2}><Box><Stack direction="row" gap={1} alignItems="center"><Typography variant="h6" fontWeight={800}>{question.text || "Текст вопроса скрыт"}</Typography>{question.urgent && <Chip label="Срочно" color="error" size="small" />}</Stack><Typography color="text.secondary">Адресат: {question.addressee}</Typography></Box><StatusChip status={question.status} /></Stack></CardContent></Card>)}{!data.questions.length && <EmptyState>Спроси Напарника — он сначала проверит базу</EmptyState>}</Stack>
  </>;

const KnowledgeView = ({ data, onOpen }: { data: DataState; onOpen: (page: KbPage) => void }) => <>
  <SectionTitle eyebrow={data.me.department} title="База знаний" text="Материалы твоей команды с авторами, датами и честной маркировкой демо-инструкций." />
  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "230px 1fr" }, gap: 3 }}><Card><CardContent><Typography fontWeight={800} mb={2}>Папки</Typography><List disablePadding>{data.folders.map((folder) => <ListItem key={folder.id} disablePadding><ListItemButton><ListItemAvatar><Avatar sx={{ bgcolor: "#f0ebff", color: "secondary.main" }}><FolderRounded /></Avatar></ListItemAvatar><ListItemText primary={folder.title} /></ListItemButton></ListItem>)}</List></CardContent></Card><Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", xl: "repeat(3, 1fr)" }, gap: 2 }}>{data.pages.map((page) => <Card key={page.id} sx={{ minHeight: 240 }}><CardActionArea onClick={() => onOpen(page)} sx={{ height: "100%" }}><CardContent sx={{ p: 3, height: "100%", display: "flex", flexDirection: "column" }}><Avatar sx={{ bgcolor: "#e8f6ff", color: "primary.main", mb: 3 }}><MenuBookRounded /></Avatar><Typography variant="h6" fontWeight={800}>{page.title}</Typography><Typography color="text.secondary" my={1}>{page.excerpt}</Typography><Box mt="auto"><Typography variant="caption" color="text.secondary">{page.author} · {page.updated_at}</Typography>{page.test_instruction && <Chip label="Тестовая инструкция" color="warning" size="small" sx={{ mt: 1, display: "flex", width: "fit-content" }} />}</Box></CardContent></CardActionArea></Card>)}</Box></Box>
  </>;

const RequestsView = ({ data, onCreate }: { data: DataState; onCreate: () => void }) => <>
  <Stack direction={{ xs: "column", sm: "row" }} justifyContent="space-between" alignItems={{ sm: "end" }} mb={4}><SectionTitle eyebrow="Корпоративные сервисы" title="Заявки" text="Сначала черновик и проверка, затем подтверждение и квитанция." /><Button variant="contained" size="large" startIcon={<EventAvailableRounded />} onClick={onCreate}>Новая заявка</Button></Stack>
  <Stack spacing={2}>{data.requests.map((request) => <Card key={request.id}><CardContent sx={{ p: 3 }}><Stack direction="row" alignItems="center" spacing={2}><Avatar sx={{ bgcolor: request.kind === "absence" ? "#fff3d6" : "#e8f6ff", color: "text.primary" }}>{request.kind === "absence" ? <CalendarMonthRounded /> : <OpenInNewRounded />}</Avatar><Box flex={1}><Typography fontWeight={800}>{request.kind === "absence" ? `${request.type || "Отгул"} · ${request.date}` : `Доступ: ${request.system}`}</Typography><Typography variant="body2" color="text.secondary">{request.id} · {request.created_at}</Typography></Box><StatusChip status={request.status} /></Stack></CardContent></Card>)}{!data.requests.length && <EmptyState>Заявок пока нет</EmptyState>}</Stack>
  </>;

const ChatDrawer = ({ open, onClose, text, setText, messages, loading, onSend, onConfirm }: { open: boolean; onClose: () => void; text: string; setText: (text: string) => void; messages: ChatMessage[]; loading: boolean; onSend: () => void; onConfirm: (id: string) => void }) => <Drawer anchor="right" open={open} onClose={onClose} PaperProps={{ sx: { width: { xs: "100%", sm: 440 }, borderRadius: { sm: "28px 0 0 28px" } } }}><Stack height="100%"><Stack direction="row" alignItems="center" spacing={1.5} sx={{ p: 2.5, bgcolor: "#f0f6ff" }}><Avatar src={mascotCat} sx={{ width: 58, height: 58, bgcolor: "#dff1ff" }} /><Box flex={1}><Typography fontWeight={800}>Напарник</Typography><Typography variant="caption" color="success.main">● рядом и готов помочь</Typography></Box><IconButton onClick={onClose}><CloseRounded /></IconButton></Stack><Stack spacing={2} sx={{ p: 2.5, flex: 1, overflowY: "auto" }}>{messages.map((message, index) => <Box key={`${message.role}-${index}`} alignSelf={message.role === "user" ? "flex-end" : "flex-start"} maxWidth="88%"><Paper sx={{ p: 1.5, borderRadius: 4, bgcolor: message.role === "user" ? "primary.main" : "#f2f2f2", color: message.role === "user" ? "white" : "text.primary" }}><Typography>{message.text}</Typography></Paper>{message.reply?.sources.map((source) => <Paper variant="outlined" key={source.page_id} sx={{ p: 1.5, mt: 1, borderRadius: 3 }}><Typography fontWeight={800}>{source.title}</Typography><Typography variant="caption" color="text.secondary">{source.author} · {source.updated_at}</Typography></Paper>)}{message.reply?.drafts.map((draft) => <Paper key={draft.id} sx={{ p: 1.5, mt: 1, borderRadius: 3, bgcolor: "#fff6d8" }}><Typography fontWeight={800}>Черновик действия</Typography><Typography variant="caption">{draft.tool}</Typography><Button fullWidth variant="contained" sx={{ mt: 1 }} onClick={() => void onConfirm(draft.id)}>Подтвердить</Button></Paper>)}</Box>)}{loading && <CircularProgress size={24} />}</Stack><Stack direction="row" spacing={1} sx={{ p: 2, borderTop: "1px solid #eee" }}><TextField fullWidth multiline maxRows={4} value={text} onChange={(event) => setText(event.target.value)} placeholder="Спроси о команде или задачах…" onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); void onSend(); } }} /><IconButton color="primary" onClick={() => void onSend()} disabled={!text.trim() || loading}><SendRounded /></IconButton></Stack></Stack></Drawer>;

export default App;
