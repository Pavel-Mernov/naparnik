// Оболочка Напарника: данные, роли, навигация. Экраны — в src/pages, блоки — в src/components.
// Дизайн — по логике Avito Start и дизайн-системы Авито: плоско, линии 2 px, чёрная главная кнопка,
// цвета логотипа как акценты, фиолетовый — цвет ИИ. Главное на каждом экране — в первой строке.
import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import {
  Alert, Avatar, Badge, Box, BottomNavigation, BottomNavigationAction, Button, Container, CssBaseline, IconButton, LinearProgress,
  ListItemText, Menu, MenuItem, Popover, Snackbar, Stack, TextField, ThemeProvider, Tooltip, Typography, useMediaQuery,
} from "@mui/material";
import {
  DarkModeOutlined, DescriptionOutlined, ForumOutlined, HomeOutlined, LightModeOutlined, MenuBookOutlined,
  NotificationsNoneOutlined, TaskAltOutlined,
} from "@mui/icons-material";
import avitoLogo from "../avito-logo.svg";
import { ChatDrawer, type ChatMessage, type Lookup } from "./chat";
import {
  api, ApiError, unwrap, type ChatReply, type KbDraft, type KbFolder, type KbPage, type Me, type PlanData, type PlanStep,
  type Question, type RequestItem, type Services, type TeamOverview, type TodayData, type User,
} from "./api";
import { makeTheme, tokens, type Mode } from "./theme";
import { buildNotes, initials, requestTitle, statusOf } from "./helpers";
import { ROLE_RU, primaryRole, viewLabel, viewsFor, type DataState, type View } from "./state";
import { Block, Empty, Row, StatusTag } from "./components/ui";
import { MascotBadge } from "./components/Mascot";
import { DayPage, PlanPage } from "./pages/InternPages";
import { QuestionsPage } from "./pages/QuestionsPage";
import { KnowledgePage } from "./pages/KnowledgePage";
import { RequestsPage } from "./pages/RequestsPage";
import { BuddyPage, TeamPage } from "./pages/TeamPages";
import { PracticePanel } from "./pages/PracticePanel";

const ICONS: Record<View, ReactElement> = {
  day: <HomeOutlined />, plan: <TaskAltOutlined />, questions: <ForumOutlined />, knowledge: <MenuBookOutlined />, requests: <DescriptionOutlined />,
};
const GREETING: ChatMessage = { role: "assistant", text: "Привет! Спроси про команду, процессы или задачи. Могу оформить отгул или доступ — без твоего «Подтвердить» ничего не отправлю." };

const initialMode = (): Mode => {
  try { const saved = localStorage.getItem("naparnik.theme"); if (saved === "light" || saved === "dark") return saved; } catch { /* приватный режим */ }
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
};

function App() {
  const [mode, setMode] = useState<Mode>(initialMode);
  const theme = useMemo(() => makeTheme(mode), [mode]);
  const t = tokens[mode];
  const mobile = useMediaQuery(theme.breakpoints.down("md"));
  const wide = useMediaQuery(theme.breakpoints.up("lg"));

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
  const [practiceRevision, setPracticeRevision] = useState(0);
  const [messages, setMessages] = useState<ChatMessage[]>([GREETING]);
  const [busy, setBusy] = useState(false);
  const [snack, setSnack] = useState("");
  const [notesAnchor, setNotesAnchor] = useState<HTMLElement | null>(null);
  const [accountAnchor, setAccountAnchor] = useState<HTMLElement | null>(null);
  const role = data ? primaryRole(data.me.roles) : null;
  const views = role ? viewsFor(role) : [];
  const isCurrent = (owner: string, epoch: number) => account.current === owner && generation.current === epoch;

  useEffect(() => { try { localStorage.setItem("naparnik.theme", mode); } catch { /* не критично */ } }, [mode]);

  const load = useCallback(async () => {
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    const seq = ++loadSequence.current;
    const epoch = generation.current;
    const current = () => account.current === login && generation.current === epoch && loadSequence.current === seq && !controller.signal.aborted;
    const options = { signal: controller.signal };
    const optionalPlan = async (id: string) => {
      try { return unwrap(await api<{ ok: boolean; data: PlanData }>(login, `/api/plans/${id}`, options)); }
      catch (e) { if (e instanceof ApiError && e.status === 404) return undefined; throw e; }
    };
    setLoading(true);
    setError("");
    try {
      const [me, usersPayload] = await Promise.all([api<Me>(login, "/api/me", options), api<{ users: User[] }>(login, "/api/users", options)]);
      if (!current()) return;
      setUsers(usersPayload.users);
      const r = primaryRole(me.roles);
      const [today, plan, questions, kb, requests, overview, managedPlans, services, drafts] = await Promise.all([
        r === "intern" ? api<{ ok: boolean; data: TodayData }>(login, "/api/today", options).then(unwrap) : undefined,
        r === "intern" ? optionalPlan(me.id) : undefined,
        api<{ questions: Question[] }>(login, "/api/questions", options),
        api<{ folders: KbFolder[]; pages: KbPage[] }>(login, "/api/kb/pages", options),
        r === "intern" || r === "lead" ? api<{ requests: RequestItem[] }>(login, "/api/requests", options) : undefined,
        r === "lead" ? api<{ ok: boolean; data: TeamOverview }>(login, "/api/dashboard/team", options).then(unwrap) : undefined,
        r === "buddy" ? Promise.all(me.interns.map(async (i) => ({ ...i, plan: await optionalPlan(i.id) }))) : [],
        r === "lead" || r === "intern" ? api<Services>(login, "/api/services", options) : undefined,
        api<{ drafts: KbDraft[] }>(login, "/api/kb/drafts", options).then((x) => x.drafts).catch(() => []),
      ]);
      if (!current()) return;
      setData({ me, users: usersPayload.users, today, plan, questions: questions.questions, folders: kb.folders, pages: kb.pages,
        requests: requests?.requests ?? [], overview, managedPlans, services, drafts });
      setSelectedIntern((prev) => me.interns.some((i) => i.id === prev) ? prev : me.interns[0]?.id ?? "");
    } catch (e) {
      if (current()) setError(e instanceof Error ? e.message : "Не удалось загрузить данные");
    } finally {
      if (current()) setLoading(false);
    }
  }, [login]);

  useEffect(() => { void load(); return () => { loadController.current?.abort(); }; }, [load]);
  useEffect(() => () => { actions.current.forEach((c) => c.abort()); }, []);
  // Чат на экране начинается с чистого листа — и разговор с агентом на сервере тоже.
  useEffect(() => { void api(login, "/api/chat/reset", { method: "POST", body: "{}" }).catch(() => undefined); }, [login]);

  const switchUser = (next: string) => {
    setAccountAnchor(null);
    if (next === account.current) return;
    account.current = next;
    generation.current++;
    loadSequence.current++;
    loadController.current?.abort();
    actions.current.forEach((c) => c.abort());
    actions.current.clear();
    localStorage.setItem("naparnik.login", next);
    setData(null); setView("day"); setSelectedIntern(""); setBusy(false); setError(""); setSnack("");
    setChatText(""); setChatOpen(false); setChatLoading(false); setMessages([GREETING]);
    setLoading(true); setLogin(next);
  };

  const openChat = (prefill = "") => { setChatText(prefill); setChatOpen(true); };

  const sendChat = async (retry?: string) => {
    const text = (retry ?? chatText).trim();
    if (!text || chatLoading || account.current !== login) return;
    const owner = login, epoch = generation.current;
    const controller = new AbortController(); actions.current.add(controller);
    if (!retry) setChatText("");
    setMessages((m) => [...m, { role: "user", text }]); setChatLoading(true);
    try {
      const reply = await api<ChatReply>(owner, "/api/chat/messages", { method: "POST", body: JSON.stringify({ text }), signal: controller.signal });
      if (isCurrent(owner, epoch)) { setMessages((m) => [...m, { role: "assistant", text: reply.text, reply, retryText: reply.retry ? text : undefined }]); setPracticeRevision((n) => n + 1); }
    } catch (e) {
      if (isCurrent(owner, epoch)) setMessages((m) => [...m, { role: "assistant", text: e instanceof Error ? e.message : "Не получилось ответить", retryText: text }]);
    } finally {
      actions.current.delete(controller);
      if (isCurrent(owner, epoch)) setChatLoading(false);
    }
  };

  /** Действие человека в интерфейсе: запрос → сообщение → перезагрузка данных. Ответы чужой учётки отбрасываются. */
  const perform = async (path: string, method: string, body: unknown, message: string) => {
    const owner = login, epoch = generation.current;
    if (account.current !== owner) return false;
    const controller = new AbortController(); actions.current.add(controller);
    setBusy(true);
    try {
      await api(owner, path, { method, body: JSON.stringify(body), signal: controller.signal });
      if (isCurrent(owner, epoch)) { setSnack(message); await load(); }
      return true;
    } catch (e) {
      if (isCurrent(owner, epoch)) setSnack(e instanceof Error ? e.message : "Не получилось");
      return false;
    } finally { actions.current.delete(controller); if (isCurrent(owner, epoch)) setBusy(false); }
  };

  const markDone = async (step: PlanStep) => {
    if (!data || (role !== "intern" && role !== "buddy")) return;
    const internId = role === "intern" ? data.me.id : selectedIntern;
    if (internId) await perform(`/api/plan-steps/${step.id}`, "PATCH", { intern_id: internId }, "Шаг отмечен — так держать");
  };
  const look: Lookup = {
    userName: (id) => (data?.users ?? users).find((u) => u.id === id)?.name ?? id,
    systemTitle: (id) => data?.services?.access.systems?.find((x) => x.system === id)?.title ?? id,
    stepTitle: (id) => data?.plan?.steps.find((x) => x.id === id)?.title ?? id,
  };

  const notes = data ? buildNotes({
    meId: data.me.id, today: data.me.today, steps: data.today?.steps ?? data.plan?.steps ?? [], questions: data.questions,
    requests: data.requests, drafts: data.drafts, events: data.today?.events ?? [],
  }) : [];
  const urgent = notes.filter((n) => n.urgent).length;
  const waitingForMe = data ? notes.filter((n) => !n.urgent && n.go !== "day") : [];

  const nav = (v: View) => { setView(v); window.scrollTo({ top: 0 }); };

  const page = () => {
    if (!data || !role) return null;
    if (view === "day") {
      if (role === "intern") return <DayPage data={data} onMarkDone={(s) => void markDone(s)} onAsk={openChat} onOpenPlan={() => nav("plan")} onOpenQuestions={() => nav("questions")}
        waiting={<Block title="Новости">{waitingForMe.filter((n) => !n.id.startsWith("ans-")).length ? waitingForMe.filter((n) => !n.id.startsWith("ans-")).slice(0, 5).map((n) => <Row key={n.id} title={n.text} sub={n.meta} onClick={() => nav(n.go)} />)
          : <Empty text="Ответы людей и статусы заявок появятся здесь." variant={1} />}</Block>} />;
      if (role === "lead") return <TeamPage overview={data.overview} onRequests={() => nav("requests")} />;
      if (role === "buddy") return <BuddyPage data={data} onPlan={(id) => { setSelectedIntern(id); nav("plan"); }} onQuestions={() => nav("questions")} />;
    }
    if (view === "plan" && role === "intern") return <PlanPage plan={data.plan} title="Мой план" context={`${data.me.week}-я неделя · ${data.me.department}`} today={data.me.today} onMarkDone={(s) => void markDone(s)} />;
    if (view === "plan" && role === "buddy") {
      const chosen = data.managedPlans.find((i) => i.id === selectedIntern);
      return <PlanPage plan={chosen?.plan} title={chosen ? `План: ${chosen.name}` : "План стажёра"} context="Бадди может отмечать шаги стажёра" today={data.me.today}
        onMarkDone={(s) => void markDone(s)}
        picker={data.managedPlans.length > 1 && <TextField select size="small" label="Стажёр" value={selectedIntern} onChange={(e) => setSelectedIntern(e.target.value)} sx={{ minWidth: 220 }}>
          {data.managedPlans.map((i) => <MenuItem key={i.id} value={i.id}>{i.name}</MenuItem>)}</TextField>} />;
    }
    if (view === "questions" || (view === "day" && role === "expert")) return <QuestionsPage data={data} role={role} onAsk={() => openChat()}
      onAnswer={(q, text) => perform(`/api/questions/${q.id}/answer`, "POST", { text }, "Ответ ушёл стажёру. Проверь страницу для базы")}
      onApprove={async (d, title, body) => { await perform(`/api/kb/pages/${d.id}/approve`, "POST", { title, body }, "Страница опубликована в базе знаний"); }} />;
    if (view === "knowledge") return <KnowledgePage data={data} onAsk={openChat} />;
    if (view === "requests") return <RequestsPage data={data} login={login} isLead={role === "lead"} busy={busy} onAgent={() => openChat("Мне нужен отгул ")}
      onSent={(m) => { setSnack(m); void load(); }}
      onMarkStep={role === "intern" ? (id) => { const st = data.plan?.steps.find((x) => x.id === id); if (st) void markDone(st); } : undefined}
      onDecide={async (r, decision, reason, comment) => {
        await perform(`/api/requests/${r.id}/decision`, "POST", { decision, reason, comment },
          decision === "approve" ? `${requestTitle(r)}: ${statusOf(r.kind === "access" ? "granted" : "approved").label.toLowerCase()}` : "Заявка отклонена, стажёр увидит причину");
      }} />;
    return null;
  };

  const me = data?.me;
  const others = users.filter((u) => u.login !== login);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Box sx={{ minHeight: "100dvh", pb: { xs: 10, md: 0 } }}>
        <Box component="header" sx={{ position: "sticky", top: 0, zIndex: 10, bgcolor: t.bg, borderBottom: `2px solid ${t.line}` }}>
          <Container maxWidth="lg">
            <Stack direction="row" alignItems="center" gap={{ xs: 1, md: 3 }} sx={{ height: 64 }}>
              <Stack direction="row" alignItems="center" gap={1.25} component="a" href="#" onClick={(e) => { e.preventDefault(); nav("day"); }} sx={{ color: "inherit", textDecoration: "none", flex: "none" }}>
                <Box component="img" src={avitoLogo} alt="Avito" sx={{ height: { xs: 32, md: 38 }, width: "auto", filter: mode === "dark" ? "invert(1) hue-rotate(180deg)" : "none" }} />
                <Typography sx={{ display: { xs: "none", sm: "block" }, fontWeight: 700, fontSize: { xs: 19, md: 22 }, letterSpacing: "-0.025em" }}>Напарник</Typography>
              </Stack>
              {!mobile && role && <Stack component="nav" aria-label="Разделы" direction="row" gap={0} sx={{ flex: 1, ml: { md: 0, lg: 2 }, minWidth: 0, overflowX: "auto" }}>
                {views.map((v) => <Box key={v} component="button" onClick={() => nav(v)} aria-current={view === v ? "page" : undefined} sx={{
                  font: "inherit", fontSize: 15, border: "none", bgcolor: "transparent", color: view === v ? t.text : t.muted, px: { md: 1, lg: 1.5 }, height: 64, whiteSpace: "nowrap",
                  borderBottom: `2px solid ${view === v ? t.text : "transparent"}`, mb: "-2px", fontWeight: view === v ? 600 : 400, "&:hover": { color: t.text },
                }}>{viewLabel(role, v)}</Box>)}
              </Stack>}
              <Box flex={1} display={{ md: "none" }} />
              <Stack direction="row" alignItems="center" gap={0.5}>
                {role && role !== "intern" && !mobile && (wide
                  ? <Button variant="text" onClick={() => openChat()} startIcon={<ForumOutlined />}>Спросить Напарника</Button>
                  : <Tooltip title="Спросить Напарника"><IconButton aria-label="Спросить Напарника" onClick={() => openChat()}><ForumOutlined /></IconButton></Tooltip>)}
                <Tooltip title={mode === "light" ? "Тёмная тема" : "Светлая тема"}>
                  <IconButton aria-label="Сменить тему" onClick={() => setMode(mode === "light" ? "dark" : "light")}>{mode === "light" ? <DarkModeOutlined /> : <LightModeOutlined />}</IconButton>
                </Tooltip>
                <IconButton aria-label={`Уведомления: ${notes.length}`} onClick={(e) => setNotesAnchor(e.currentTarget)}>
                  <Badge badgeContent={urgent} sx={{ "& .MuiBadge-badge": { bgcolor: "error.main", color: "#000", fontWeight: 700, borderRadius: 0 } }}><NotificationsNoneOutlined /></Badge>
                </IconButton>
                <Box component="button" onClick={(e) => setAccountAnchor(e.currentTarget)} aria-label="Тестовая учётка — сменить роль" sx={{
                  display: "flex", alignItems: "center", gap: 1, font: "inherit", border: "none", bgcolor: "transparent", color: "inherit", px: 1, py: 0.5, "&:hover": { bgcolor: t.surface },
                }}>
                  <Avatar sx={{ width: 32, height: 32, fontSize: 13, bgcolor: t.todaySoft, color: t.text }}>{me ? initials(me.name) : "…"}</Avatar>
                  {wide && me && <Box textAlign="left"><Typography variant="body2" fontWeight={600} lineHeight={1.2}>{me.name}</Typography><Typography variant="caption" color="text.secondary">{role && ROLE_RU[role]} · демо</Typography></Box>}
                </Box>
              </Stack>
            </Stack>
          </Container>
        </Box>
        {loading && <LinearProgress aria-label="Загрузка" sx={{ position: "fixed", top: 64, left: 0, right: 0, zIndex: 11, height: 2 }} />}

        <Container maxWidth="lg" component="main" sx={{ pt: { xs: 3, md: 5 }, pb: { xs: 10, md: 12 } }}>
          {error && <Alert severity="error" sx={{ mb: 3, borderRadius: 0 }} action={<Button variant="text" onClick={() => void load()}>Повторить</Button>}>Не получилось загрузить данные: {error}</Alert>}
          {data && data.me.login === login ? page() : !error && <Stack gap={2} aria-hidden>{[70, 40, 90, 60].map((w, i) => <Box key={i} sx={{ height: i ? 18 : 40, width: `${w}%`, bgcolor: t.surface }} />)}</Stack>}
          {data && data.me.login === login && (role === "intern" || role === "buddy") && (view === "day" || view === "plan") && <PracticePanel key={login} login={login} buddy={role === "buddy"} revision={practiceRevision} />}
        </Container>

        {mobile && role && <BottomNavigation component="nav" aria-label="Разделы" showLabels value={view} onChange={(_, v: View) => nav(v)} sx={{
          position: "fixed", bottom: 0, left: 0, right: 0, zIndex: 10, height: 64, bgcolor: t.bg, borderTop: `2px solid ${t.line}`,
          "& .MuiBottomNavigationAction-root": { minWidth: 0, color: t.muted, px: 0.5 }, "& .Mui-selected": { color: `${t.text} !important` },
          "& .MuiBottomNavigationAction-label": { fontSize: "11px !important", lineHeight: 1.2, whiteSpace: "nowrap" },
        }}>
          {views.map((v) => <BottomNavigationAction key={v} value={v} label={viewLabel(role, v)} icon={ICONS[v]} />)}
        </BottomNavigation>}

        {/* Маскот — на всех экранах, 96 px (08-DESIGN): клик открывает чат, бейдж — сколько ждёт внимания,
            «Думаю…» — пока отвечает модель. Плавное «дыхание» отключается при prefers-reduced-motion. */}
        {role && !chatOpen && <Box component="button" onClick={() => openChat()} aria-label="Открыть чат с Напарником" sx={{
          position: "fixed", right: { xs: 8, md: 24 }, bottom: { xs: 70, md: 20 }, zIndex: 12, p: 0, border: "none", bgcolor: "transparent",
          transition: "transform .2s ease", "&:hover, &:focus-visible": { transform: "translateY(-3px)" },
          "&:hover .mascot-label, &:focus-visible .mascot-label": { opacity: 1, transform: "none" },
        }}>
          <MascotBadge size={mobile ? 72 : 96} />
          <Box component="span" className="mascot-label" sx={{
            position: "absolute", right: "100%", top: "30%", mr: 0.5, px: 1.25, py: 0.75, bgcolor: t.inverse, color: t.onInverse, fontSize: 13, fontWeight: 600,
            whiteSpace: "nowrap", opacity: chatLoading ? 1 : 0, transform: chatLoading ? "none" : "translateX(6px)", transition: "opacity .2s, transform .2s", pointerEvents: "none",
            display: { xs: "none", md: "block" },
          }}>{chatLoading ? "Думаю…" : urgent ? `Есть новое: ${urgent}` : "Спроси меня"}</Box>
          {urgent > 0 && <Box component="span" sx={{ position: "absolute", top: { xs: 6, md: 10 }, right: { xs: 6, md: 10 }, minWidth: 24, height: 24, px: 0.5, borderRadius: "12px", bgcolor: "error.main", color: "#000", fontSize: 12, fontWeight: 700, lineHeight: "20px", border: `2px solid ${t.bg}` }}>{urgent}</Box>}
        </Box>}

        <Popover open={Boolean(notesAnchor)} anchorEl={notesAnchor} onClose={() => setNotesAnchor(null)} anchorOrigin={{ vertical: "bottom", horizontal: "right" }} transformOrigin={{ vertical: "top", horizontal: "right" }}
          slotProps={{ paper: { sx: { width: 360, maxWidth: "calc(100vw - 24px)", maxHeight: 480 } } }}>
          <Box sx={{ p: 2 }}>
            <Typography variant="h3" component="h2" mb={1}>Уведомления</Typography>
            {notes.length ? notes.map((n) => <Row key={n.id} title={n.text} sub={n.meta} onClick={() => { setNotesAnchor(null); nav(n.go); }}
              end={n.urgent ? <StatusTag status="Overdue" label="нужно действие" /> : undefined} />) : <Empty text="Всё спокойно — новых событий нет." variant={2} />}
          </Box>
        </Popover>

        <Menu open={Boolean(accountAnchor)} anchorEl={accountAnchor} onClose={() => setAccountAnchor(null)} anchorOrigin={{ vertical: "bottom", horizontal: "right" }} transformOrigin={{ vertical: "top", horizontal: "right" }}>
          <Typography variant="caption" color="text.secondary" sx={{ px: 2, py: 1, display: "block" }}>Тестовые учётки · войти как</Typography>
          {others.map((u) => <MenuItem key={u.login} onClick={() => switchUser(u.login)} sx={{ gap: 1.5, minHeight: 48 }}>
            <Avatar sx={{ width: 28, height: 28, fontSize: 12, bgcolor: t.surface, color: t.text }}>{initials(u.name)}</Avatar>
            <ListItemText primary={u.name} secondary={`${ROLE_RU[primaryRole(u.roles)]} · ${u.position}`} />
          </MenuItem>)}
        </Menu>

        <ChatDrawer open={chatOpen} onClose={() => setChatOpen(false)} text={chatText} setText={setChatText} messages={messages} loading={chatLoading}
          onSend={(retry) => void sendChat(retry)} login={login} look={look} onAction={() => void load()} />
        <Snackbar open={Boolean(snack)} autoHideDuration={3000} onClose={() => setSnack("")} message={snack}
          anchorOrigin={{ vertical: "bottom", horizontal: "center" }} sx={{ bottom: { xs: 88, md: 24 } }} />
      </Box>
    </ThemeProvider>
  );
}

export default App;
