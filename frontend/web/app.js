// Живая версия прототипа design1.html: 5 страниц, данные и чат — из Go-бэкенда (docs/spec/05-tools-api.md).
// Явный API-клиент (FE-03): все запросы идут через api(). Учётка — заголовок X-User (демо-переключатель ролей).
'use strict';

const $ = (id) => document.getElementById(id);
const state = { login: 'intern.sokolov', me: null, users: [], plan: null, questions: [], qFilter: '', kb: null, kbType: '', kbFolder: '', lastQuestion: '' };

try { state.login = localStorage.getItem('buddy.login') || state.login; } catch { /* приватный режим — не страшно */ }

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { 'X-User': state.login, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  let data = {};
  try { data = await res.json(); } catch { /* пустой ответ */ }
  return { status: res.status, data };
}

// Только textContent — никаких HTML-вставок из данных.
function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function toast(text) {
  const t = $('toast');
  t.textContent = text;
  t.classList.add('show');
  setTimeout(() => t.classList.remove('show'), 2200);
}

const ruDate = (iso, opts = { weekday: 'long', day: 'numeric', month: 'long' }) => new Date(iso + 'T12:00:00').toLocaleDateString('ru-RU', opts);
const STATUS = {
  Overdue: ['red', 'Просрочен'], Today: ['blue', 'На сегодня'], Done: ['green', 'Готово'], Planned: ['', 'Запланирован'],
  pending: ['amber', 'На согласовании'], approved: ['green', 'Согласовано'], rejected: ['red', 'Отклонено'],
  granted: ['green', 'Выдан'], Sent: ['blue', 'Ждёт ответа'], Answered: ['green', 'Ответ готов'],
};
function badge(status) {
  const [color, text] = STATUS[status] || ['', status];
  return el('span', 'badge ' + color, text);
}
const initials = (name) => name.split(' ').slice(0, 2).map((w) => w[0]).join('');
const nameById = (id) => (state.users.find((u) => u.id === id) || {}).name || id;
const isIntern = () => state.me && state.me.roles.includes('intern');
const planOwner = () => (isIntern() ? state.me.id : (state.me.interns[0] || {}).id);

// ---------- навигация ----------

const PAGES = { day: 'мой день', plan: 'мой план', questions: 'вопросы', knowledge: 'база знаний', requests: 'заявки' };
function openPage(name) {
  if (!PAGES[name]) name = 'day';
  document.querySelectorAll('.page-view').forEach((v) => v.classList.toggle('active', v.dataset.view === name));
  document.querySelectorAll('.nav button').forEach((b) => b.classList.toggle('active', b.dataset.page === name));
  document.title = 'Напарник — ' + PAGES[name];
  history.replaceState(null, '', '#' + name);
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

// ---------- загрузка ----------

async function loadUsers() {
  const { data } = await api('GET', '/api/users');
  state.users = data.users || [];
  const who = $('who');
  who.replaceChildren();
  for (const u of state.users) {
    const o = el('option', '', `${u.name} · ${u.position}`);
    o.value = u.login;
    o.selected = u.login === state.login;
    who.appendChild(o);
  }
}

async function loadAll() {
  const me = await api('GET', '/api/me');
  if (me.status !== 200) { $('hello').textContent = 'Не удалось войти — проверь, что сервер запущен'; return; }
  state.me = me.data;
  $('date').textContent = ruDate(state.me.today);
  $('hello').textContent = `Доброе утро, ${state.me.name.split(' ')[0]}`;
  $('weekPill').replaceChildren(el('b', '', isIntern() ? `Неделя ${state.me.week}` : state.me.position), document.createTextNode(` · ${state.me.department}`));
  $('kbEyebrow').textContent = state.me.department;
  await Promise.all([renderToday(), loadPlan(), loadQuestions(), loadKB(), renderRequests()]);
}

// ---------- Мой день ----------

async function renderToday() {
  const list = $('todayList');
  const box = $('events');
  list.replaceChildren();
  box.replaceChildren();
  $('todayCard').hidden = !isIntern();
  if (!isIntern()) {
    $('subtitle').textContent = 'Здесь видно прогресс стажёров и вопросы, адресованные тебе.';
    box.appendChild(el('div', 'empty', 'Событий на сегодня нет'));
    return;
  }
  const { data } = await api('GET', '/api/today');
  const steps = (data.data && data.data.steps) || [];
  const events = (data.data && data.data.events) || [];
  $('todayCount').textContent = steps.length;
  $('subtitle').textContent = `Сегодня шагов: ${steps.length}, встреч: ${events.length}. Начнём без спешки.`;
  if (!steps.length) list.appendChild(el('div', 'empty', 'На сегодня всё сделано'));
  for (const s of steps) {
    const row = el('div', 'task');
    const check = el('button', 'check', '✓');
    check.setAttribute('aria-label', 'Отметить шаг выполненным');
    check.onclick = () => markDone(s.id);
    const body = el('div');
    body.appendChild(el('div', 'task-title', s.title));
    const meta = el('div', 'meta');
    meta.append(el('span', '', `до ${ruDate(s.due_date, { day: 'numeric', month: 'long' })}`), badge(s.status));
    body.appendChild(meta);
    const access = s.title.toLowerCase().includes('доступ');
    const action = el('button', 'task-action', access ? 'Оформить' : 'Нужна помощь');
    action.onclick = () => openChat(access ? `Нужен доступ: ${s.title}` : `Помоги с шагом: ${s.title}`);
    row.append(check, body, action);
    list.appendChild(row);
  }
  if (!events.length) box.appendChild(el('div', 'empty', 'Событий на сегодня нет'));
  for (const e of events) {
    const ev = el('div', 'event');
    const right = el('div');
    right.append(el('div', 'event-title', e.title), el('div', 'event-sub', 'Сегодня'));
    ev.append(el('div', 'event-time', e.start), right);
    box.appendChild(ev);
  }
}

async function markDone(stepId) {
  const { status, data } = await api('PATCH', `/api/plan-steps/${stepId}`, {});
  toast(status === 200 ? 'Шаг отмечен выполненным' : (data.error && data.error.message) || 'Не получилось');
  await Promise.all([renderToday(), loadPlan()]);
}

// ---------- План ----------

async function loadPlan() {
  const id = planOwner();
  state.plan = null;
  if (id) {
    const { status, data } = await api('GET', `/api/plans/${id}`);
    state.plan = status === 200 ? data.data : { error: (data.error && data.error.message) || 'План недоступен' };
  }
  renderProgressCard();
  renderPlanPage();
}

function stageStats(p) {
  const mine = p.steps.filter((s) => s.owner === 'intern');
  return p.stages.map((st) => {
    const steps = mine.filter((s) => s.stage === st.id);
    return { st, steps, done: steps.filter((s) => s.status === 'Done').length };
  });
}

function renderProgressCard() {
  const p = state.plan;
  $('progressCard').hidden = !p;
  $('stages').replaceChildren();
  $('stageLabels').replaceChildren();
  if (!p) return;
  $('progressTitle').textContent = isIntern() ? 'Путь адаптации' : `Путь адаптации: ${nameById(planOwner())}`;
  if (p.error) {
    $('progress').textContent = '—';
    $('progressMeta').textContent = p.error;
    $('nextStep').textContent = '';
    $('planBadge').textContent = '';
    return;
  }
  const mine = p.steps.filter((s) => s.owner === 'intern');
  const overdue = mine.some((s) => s.status === 'Overdue');
  $('progress').textContent = `${p.progress_pct}%`;
  $('progressMeta').textContent = `${mine.filter((s) => s.status === 'Done').length} из ${mine.length} шагов готовы`;
  $('nextStep').textContent = p.next_step ? `Следующий шаг: ${p.next_step.title}` : 'Все шаги пройдены';
  $('planBadge').className = 'badge ' + (overdue ? 'amber' : 'green');
  $('planBadge').textContent = overdue ? 'Есть просрочка' : 'В темпе';
  $('stages').style.gridTemplateColumns = `repeat(${p.stages.length}, 1fr)`;
  for (const { st, steps, done } of stageStats(p)) {
    const bar = el('div', 'stage' + (steps.length && done === steps.length ? ' complete' : ''));
    const fill = el('span');
    fill.style.width = steps.length ? `${Math.round((100 * done) / steps.length)}%` : '0%';
    bar.appendChild(fill);
    $('stages').appendChild(bar);
    $('stageLabels').appendChild(el('span', '', st.title.split(':')[0]));
  }
}

function renderPlanPage() {
  const p = state.plan;
  const box = $('planStages');
  box.replaceChildren();
  $('planH1').textContent = isIntern() ? 'Мой план' : `План: ${nameById(planOwner()) || '—'}`;
  $('planEyebrow').textContent = isIntern() ? `Неделя ${state.me.week}` : 'План стажёра';
  if (!p || p.error) {
    box.appendChild(el('div', 'empty', p ? p.error : 'Стажёров с планом нет'));
    $('planStatus').textContent = '';
    return;
  }
  $('planStatus').textContent = 'Утверждён';
  let currentFound = false;
  stageStats(p).forEach(({ st, steps, done }, i) => {
    const complete = steps.length && done === steps.length;
    const current = !complete && !currentFound;
    if (current) currentFound = true;
    const stage = el('div', 'plan-stage' + (complete ? ' complete' : current ? ' current' : ''));
    stage.append(el('span', 'plan-node', complete ? '✓' : String(i + 1)), el('h3', '', st.title),
      el('p', '', `${done} из ${steps.length} шагов`));
    const list = el('div', 'plan-steps');
    for (const s of steps) {
      const row = el('div', 'plan-step' + (s.status === 'Done' ? ' done-step' : ''));
      row.append(document.createTextNode(`${s.status === 'Done' ? '✓' : '○'} ${s.title}${s.source === 'demo-analytics' ? ' (демо)' : ''}`), badge(s.status));
      list.appendChild(row);
    }
    stage.appendChild(list);
    box.appendChild(stage);
  });
  const mine = p.steps.filter((s) => s.owner === 'intern');
  const done = mine.filter((s) => s.status === 'Done').length;
  $('planProgress').textContent = `${p.progress_pct}%`;
  $('statDone').textContent = done;
  $('statLeft').textContent = mine.length - done;
  $('planNext').textContent = p.next_step ? p.next_step.title : 'Все шаги пройдены';
}

// ---------- Вопросы ----------

async function loadQuestions() {
  const { data } = await api('GET', '/api/questions');
  state.questions = data.questions || [];
  state.teamCount = data.team_questions_count || 0;
  renderQuestions();
}

function questionText(q) { return q.text_hidden ? 'Текст скрыт настройками стажёра' : q.text; }

function renderQuestions() {
  const qs = state.questions;
  const waiting = qs.filter((q) => q.status === 'Sent').length;
  $('qEyebrow').textContent = state.teamCount ? `Вопросов стажёров людям: ${state.teamCount}` : `${qs.length} всего · ${waiting} ждут ответа`;
  // карточка на «Мой день»
  const day = $('dayQuestions');
  day.replaceChildren();
  if (state.teamCount) day.appendChild(el('div', 'empty', `Вопросов стажёров людям: ${state.teamCount}. Тексты руководителю не показываются.`));
  else if (!qs.length) day.appendChild(el('div', 'empty', 'Вопросов людям пока нет'));
  for (const q of qs.slice(0, 3)) {
    const row = el('div', 'question');
    const body = el('div');
    body.append(el('p', '', questionText(q)), el('small', '', `${q.addressee}${q.urgent ? ' · срочно' : ''}`));
    row.append(body, badge(q.status));
    day.appendChild(row);
  }
  // таблица на странице «Вопросы»
  const search = $('qSearch').value.trim().toLowerCase();
  const rows = qs.filter((q) => (!state.qFilter || q.status === state.qFilter) && (!search || questionText(q).toLowerCase().includes(search)));
  const tbody = $('qTable');
  tbody.replaceChildren();
  for (const q of rows) {
    const tr = el('tr');
    const c1 = el('td');
    c1.append(el('div', 'table-title', questionText(q)), el('div', 'meta', q.urgent ? 'Срочно' : 'В сводке'));
    const person = (name) => { const td = el('td'); const p = el('div', 'person'); p.append(el('span', 'avatar', initials(name)), document.createTextNode(name)); td.appendChild(p); return td; };
    const c4 = el('td');
    c4.appendChild(badge(q.status));
    tr.append(c1, person(nameById(q.asked_by)), person(q.addressee), c4);
    tbody.appendChild(tr);
  }
  $('qEmpty').replaceChildren(rows.length ? '' : el('div', 'empty', state.teamCount ? 'Тексты вопросов руководителю не показываются.' : 'Вопросов нет. Спроси Напарника — если ответа нет в базе, он поможет написать нужному человеку.'));
}

// ---------- База знаний ----------

async function loadKB() {
  const { data } = await api('GET', '/api/kb/pages');
  state.kb = data;
  renderFolders();
  renderKB();
}

function renderFolders() {
  const box = $('folders');
  box.replaceChildren();
  const pages = state.kb.pages || [];
  const add = (id, title) => {
    const f = el('div', 'folder' + (state.kbFolder === id ? ' active' : ''));
    f.append(el('span', '', title), el('span', '', String(pages.filter((p) => !id || p.folder === id).length)));
    f.onclick = () => { state.kbFolder = id; renderFolders(); renderKB(); };
    box.appendChild(f);
  };
  add('', 'Все материалы');
  (state.kb.folders || []).forEach((f) => add(f.id, f.title));
}

const KB_ICON = { article: 'Doc', qa: 'Q', glossary: 'ABC' };

function renderKB() {
  const grid = $('kbGrid');
  grid.replaceChildren();
  const search = $('kbSearch').value.trim().toLowerCase();
  const pages = (state.kb.pages || []).filter((p) => (!state.kbType || p.type === state.kbType) && (!state.kbFolder || p.folder === state.kbFolder)
    && (!search || (p.title + ' ' + p.excerpt).toLowerCase().includes(search)));
  if (!pages.length) grid.appendChild(el('div', 'empty', 'Ничего не нашлось. Спроси Напарника — он поищет и подскажет, к кому идти.'));
  for (const p of pages) {
    const card = el('article', 'kb-card');
    card.tabIndex = 0;
    card.append(el('div', 'kb-icon', KB_ICON[p.type] || 'Doc'), el('h3', '', p.title), el('p', 'kb-excerpt', p.excerpt));
    const meta = el('div', 'meta');
    meta.appendChild(el('span', '', `${p.author} · ${ruDate(p.updated_at, { day: 'numeric', month: 'long', year: 'numeric' })}`));
    if (p.test_instruction) meta.appendChild(el('span', 'badge amber', 'Тестовая инструкция'));
    card.appendChild(meta);
    card.onclick = () => openChat(`Расскажи подробнее: ${p.title}`);
    grid.appendChild(card);
  }
}

// ---------- Заявки ----------

const requestTitle = (r) => (r.kind === 'absence' ? `Отгул · ${ruDate(r.date, { day: 'numeric', month: 'long' })}` : `Доступ: ${r.system}`);

async function renderRequests() {
  const { data } = await api('GET', '/api/requests');
  const rs = data.requests || [];
  $('reqCount').textContent = rs.length;
  const list = $('reqList');
  list.replaceChildren();
  if (!rs.length) list.appendChild(el('div', 'empty', 'Заявок пока нет. Нажми «Новая заявка» — Напарник подготовит черновик.'));
  for (const r of rs) {
    const card = el('div', 'request-card');
    const body = el('div');
    body.append(el('div', 'table-title', requestTitle(r)), el('div', 'meta', `${r.id} · ${nameById(r.employee_id)} · согласует ${nameById(r.approver_id)}`));
    card.append(el('div', 'request-icon', r.kind === 'absence' ? '☂' : '⌘'), body, badge(r.status));
    list.appendChild(card);
  }
  const last = rs[rs.length - 1];
  const box = $('lastReceipt');
  box.replaceChildren(el('div', 'eyebrow', 'Последняя квитанция'));
  if (!last) { box.appendChild(el('p', 'meta', 'Квитанций пока нет')); return; }
  box.append(el('div', 'receipt-code', last.id), el('b', '', requestTitle(last)),
    el('p', 'meta', `Заявка сохранена и отправлена: ${nameById(last.approver_id)}. Статус обновится здесь.`), badge(last.status));
}

// ---------- Чат ----------

const chat = $('chat');
function setChat(open) {
  chat.classList.toggle('open', open);
  chat.setAttribute('aria-hidden', String(!open));
  $('mascot').setAttribute('aria-expanded', String(open));
  if (open) $('chatText').focus();
}
function openChat(prefill) {
  setChat(true);
  if (prefill) $('chatText').value = prefill;
}

function addToChat(node) {
  $('messages').appendChild(node);
  $('messages').scrollTop = $('messages').scrollHeight;
  return node;
}

const draftTitle = (d) => ({
  create_absence_request: `Заявка на отгул: ${d.args.date}`,
  request_access: `Заявка на доступ: ${d.args.system}`,
  send_question: `Вопрос для ${nameById(d.args.addressee_id)}`,
  mark_step_done: `Отметить шаг ${d.args.step_id}`,
}[d.tool] || d.tool);

function renderDraft(d) {
  const box = el('div', 'draft');
  box.appendChild(el('b', '', '📝 ' + draftTitle(d)));
  if (d.tool === 'send_question') {
    box.appendChild(el('div', '', d.args.text));
    if (d.args.reason) box.appendChild(el('div', 'meta', `Почему: ${d.args.reason}`));
  }
  const row = el('div', 'row-btns');
  const ok = el('button', 'btn primary-sm', 'Подтвердить');
  const cancel = el('button', 'btn ghost', 'Отмена');
  ok.onclick = async () => {
    ok.disabled = true;
    const { status, data } = await api('POST', `/api/drafts/${d.id}/confirm`);
    if (status === 200) {
      const r = data.receipt || {};
      const status = (STATUS[r.status] || [])[1] || r.status;
      const parts = [r.id, status && status.toLowerCase(), r.approver, r.to && `${r.to}, ${r.delivery}`].filter(Boolean);
      box.replaceWith(el('div', 'receipt', '✅ Готово: ' + parts.join(' · ')));
      await Promise.all([renderRequests(), loadQuestions(), loadPlan()]);
    } else {
      ok.disabled = false;
      ok.textContent = status === 503 ? 'Повторить' : 'Подтвердить';
      addToChat(el('div', 'error', (data.error && data.error.message) || 'Не получилось'));
    }
  };
  cancel.onclick = () => box.replaceWith(el('div', 'empty', 'Черновик отменён — ничего не отправлено.'));
  row.append(ok, cancel);
  box.appendChild(row);
  return box;
}

function renderSource(s) {
  const box = el('div', 'source');
  if (s.label) box.appendChild(el('span', 'badge amber', s.label));
  box.append(el('b', '', s.title), el('p', '', `${s.author} · обновлено ${s.updated_at}`));
  return box;
}

function retryButton(text) {
  const row = el('div', 'row-btns');
  const retry = el('button', 'btn ghost', 'Повторить');
  retry.onclick = () => { row.remove(); ask(text); };
  row.appendChild(retry);
  return row;
}

async function ask(text) {
  state.lastQuestion = text;
  addToChat(el('div', 'bubble user', text));
  const thinking = addToChat(el('div', 'bubble bot thinking', 'Думаю…')); // BR-70: интерфейс не блокируется
  let res;
  try { res = await api('POST', '/api/chat/messages', { text }); } catch { res = { status: 0, data: {} }; }
  thinking.remove();
  if (res.status !== 200) {
    addToChat(el('div', 'error', 'Сервер не ответил. Вопрос сохранён.'));
    addToChat(retryButton(text));
    return;
  }
  const r = res.data;
  addToChat(el('div', 'bubble bot', r.text));
  (r.sources || []).slice(0, 2).forEach((s) => addToChat(renderSource(s)));
  (r.drafts || []).forEach((d) => addToChat(renderDraft(d)));
  if (r.retry) addToChat(retryButton(text));
}

// ---------- события ----------

$('mascot').onclick = () => setChat(!chat.classList.contains('open'));
$('closeChat').onclick = () => setChat(false);
$('chatForm').onsubmit = (e) => {
  e.preventDefault();
  const text = $('chatText').value.trim();
  if (!text) return;
  $('chatText').value = '';
  ask(text);
};
document.querySelectorAll('#quick button').forEach((b) => { b.onclick = () => ask(b.textContent); });
document.querySelectorAll('.nav button').forEach((b) => { b.onclick = () => openPage(b.dataset.page); });
document.querySelectorAll('[data-open]').forEach((b) => { b.onclick = () => openPage(b.dataset.open); });
document.querySelectorAll('[data-ask]').forEach((b) => { b.onclick = () => openChat(b.dataset.ask); });
document.querySelectorAll('[data-qfilter]').forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll('[data-qfilter]').forEach((x) => x.classList.toggle('active', x === b));
    state.qFilter = b.dataset.qfilter;
    renderQuestions();
  };
});
document.querySelectorAll('[data-kbtype]').forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll('[data-kbtype]').forEach((x) => x.classList.toggle('active', x === b));
    state.kbType = b.dataset.kbtype;
    renderKB();
  };
});
$('qSearch').oninput = renderQuestions;
$('kbSearch').oninput = renderKB;
$('theme').onclick = () => {
  const dark = document.documentElement.dataset.theme !== 'dark';
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  $('theme').textContent = dark ? '☾' : '☼';
};
$('who').onchange = async (e) => {
  state.login = e.target.value;
  try { localStorage.setItem('buddy.login', state.login); } catch { /* не критично */ }
  document.querySelectorAll('#messages > :not(:first-child):not(#quick)').forEach((n) => n.remove());
  await loadAll();
  toast(`Вы вошли как ${state.me ? state.me.name : state.login}`);
};

openPage(location.hash.slice(1));
(async () => { await loadUsers(); await loadAll(); })();
