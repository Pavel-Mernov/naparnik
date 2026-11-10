// Живая версия прототипа design1.html: данные и чат — из Go-бэкенда (docs/spec/05-tools-api.md).
// Явный API-клиент (FE-03): все запросы идут через api(). Учётка — заголовок X-User (демо-переключатель ролей).
'use strict';

const $ = (id) => document.getElementById(id);
const state = { login: 'intern.sokolov', me: null, users: [], lastQuestion: '' };

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

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text; // только textContent — никаких HTML-вставок из данных
  return e;
}

function toast(text) {
  const t = $('toast');
  t.textContent = text;
  t.classList.add('show');
  setTimeout(() => t.classList.remove('show'), 2200);
}

const ruDate = (iso) => new Date(iso + 'T12:00:00').toLocaleDateString('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' });
const statusBadge = {
  Overdue: ['red', 'Просрочен'], Today: ['blue', 'На сегодня'], Done: ['green', 'Готово'], Planned: ['', 'Запланирован'],
  pending: ['amber', 'На согласовании'], approved: ['green', 'Согласовано'], rejected: ['red', 'Отклонено'],
  granted: ['green', 'Выдан'], Sent: ['blue', 'Отправлен'], Answered: ['green', 'Ответ готов'],
};
function badge(status) {
  const [color, text] = statusBadge[status] || ['', status];
  return el('span', 'badge ' + color, text);
}

// ---------- загрузка экрана ----------

async function loadUsers() {
  const { data } = await api('GET', '/api/users');
  state.users = data.users || [];
  const who = $('who');
  who.replaceChildren();
  for (const u of state.users) {
    const o = el('option', '', `${u.name} · ${u.position}`);
    o.value = u.login;
    if (u.login === state.login) o.selected = true;
    who.appendChild(o);
  }
}

const nameById = (id) => (state.users.find((u) => u.id === id) || {}).name || id;
const isIntern = () => state.me && state.me.roles.includes('intern');

async function loadAll() {
  const me = await api('GET', '/api/me');
  if (me.status !== 200) { $('hello').textContent = 'Не удалось войти — проверь, что сервер запущен'; return; }
  state.me = me.data;
  $('date').textContent = ruDate(state.me.today);
  $('hello').textContent = `Доброе утро, ${state.me.name.split(' ')[0]}`;
  $('weekPill').replaceChildren(el('b', '', isIntern() ? `Неделя ${state.me.week}` : state.me.position), document.createTextNode(` · ${state.me.department}`));
  await Promise.all([renderToday(), renderPlan(), renderQuestions(), renderRequests()]);
}

async function renderToday() {
  const list = $('todayList');
  list.replaceChildren();
  $('todayCard').hidden = !isIntern();
  $('events').replaceChildren(el('div', 'empty', 'Событий на сегодня нет'));
  if (!isIntern()) { $('subtitle').textContent = 'Здесь видно прогресс стажёров и вопросы, адресованные тебе.'; return; }
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
    meta.append(el('span', '', `до ${s.due_date}`), badge(s.status));
    body.appendChild(meta);
    const action = el('button', 'task-action', s.title.toLowerCase().includes('доступ') ? 'Оформить' : 'Нужна помощь');
    action.onclick = () => openChat(s.title.toLowerCase().includes('доступ') ? `Нужен доступ: ${s.title}` : `Помоги с шагом: ${s.title}`);
    row.append(check, body, action);
    list.appendChild(row);
  }
  const box = $('events');
  box.replaceChildren();
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
  await Promise.all([renderToday(), renderPlan()]);
}

async function renderPlan() {
  const internId = isIntern() ? state.me.id : (state.me.interns[0] || {}).id;
  const progress = $('progress');
  $('stages').replaceChildren();
  $('stageLabels').replaceChildren();
  if (!internId) { $('plan').hidden = true; return; }
  $('plan').hidden = false;
  $('planTitle').textContent = isIntern() ? 'Путь адаптации' : `Путь адаптации: ${nameById(internId)}`;
  const { status, data } = await api('GET', `/api/plans/${internId}`);
  if (status !== 200) {
    progress.textContent = '—';
    $('progressMeta').textContent = (data.error && data.error.message) || 'План недоступен';
    $('nextStep').textContent = '';
    $('planBadge').textContent = '';
    return;
  }
  const p = data.data;
  const mine = p.steps.filter((s) => s.owner === 'intern');
  progress.textContent = `${p.progress_pct}%`;
  $('progressMeta').textContent = `${mine.filter((s) => s.status === 'Done').length} из ${mine.length} шагов готовы`;
  $('nextStep').textContent = p.next_step ? `Следующий шаг: ${p.next_step.title}` : 'Все шаги пройдены';
  const overdue = mine.some((s) => s.status === 'Overdue');
  $('planBadge').className = 'badge ' + (overdue ? 'amber' : 'green');
  $('planBadge').textContent = overdue ? 'Есть просрочка' : 'В темпе';
  $('stages').style.gridTemplateColumns = `repeat(${p.stages.length}, 1fr)`;
  for (const st of p.stages) {
    const steps = mine.filter((s) => s.stage === st.id);
    const done = steps.filter((s) => s.status === 'Done').length;
    const bar = el('div', 'stage' + (steps.length && done === steps.length ? ' complete' : ''));
    const fill = el('span');
    fill.style.width = steps.length ? `${Math.round((100 * done) / steps.length)}%` : '0%';
    bar.appendChild(fill);
    $('stages').appendChild(bar);
    $('stageLabels').appendChild(el('span', '', st.title.split(':')[0]));
  }
}

async function renderQuestions() {
  const { data } = await api('GET', '/api/questions');
  const list = $('questionList');
  list.replaceChildren();
  const qs = data.questions || [];
  if (data.team_questions_count) list.appendChild(el('div', 'empty', `Вопросов стажёров людям: ${data.team_questions_count}. Тексты руководителю не показываются.`));
  if (!qs.length && !data.team_questions_count) list.appendChild(el('div', 'empty', 'Вопросов пока нет'));
  for (const q of qs) {
    const row = el('div', 'question');
    const body = el('div');
    body.append(el('p', '', q.text_hidden ? 'Текст скрыт настройками стажёра' : q.text),
      el('small', '', `${nameById(q.asked_by)} → ${q.addressee}${q.urgent ? ' · срочно' : ''}`));
    row.append(body, badge(q.status));
    list.appendChild(row);
  }
}

const requestTitle = (r) => r.kind === 'absence' ? `Отгул ${r.date}` : `Доступ: ${r.system}`;

async function renderRequests() {
  const { data } = await api('GET', '/api/requests');
  const list = $('requestList');
  list.replaceChildren();
  const rs = data.requests || [];
  if (!rs.length) list.appendChild(el('div', 'empty', 'Заявок пока нет'));
  for (const r of rs) {
    const row = el('div', 'req');
    row.append(el('span', '', `${r.id} · ${requestTitle(r)} · ${nameById(r.employee_id)}`), badge(r.status));
    list.appendChild(row);
  }
}

// ---------- чат ----------

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
    if (d.args.reason) box.appendChild(el('div', 'model-pill', `Почему: ${d.args.reason}`));
  }
  const row = el('div', 'row');
  const ok = el('button', 'btn primary', 'Подтвердить');
  const cancel = el('button', 'btn ghost', 'Отмена');
  ok.onclick = async () => {
    ok.disabled = true;
    const { status, data } = await api('POST', `/api/drafts/${d.id}/confirm`);
    if (status === 200) {
      const r = data.receipt || {};
      box.replaceWith(el('div', 'receipt', `✅ Готово: ${r.id || ''} · ${r.status === 'pending' ? 'на согласовании' : r.status || ''}${r.approver ? ' · ' + r.approver : ''}${r.to ? ' · ' + r.to + ', ' + r.delivery : ''}`));
      await Promise.all([renderRequests(), renderQuestions(), renderPlan()]);
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

async function ask(text) {
  state.lastQuestion = text;
  addToChat(el('div', 'bubble user', text));
  const thinking = addToChat(el('div', 'bubble bot thinking', 'Думаю…')); // BR-70: интерфейс не блокируется
  let res;
  try {
    res = await api('POST', '/api/chat/messages', { text });
  } catch {
    res = { status: 0, data: {} };
  }
  thinking.remove();
  if (res.status !== 200) {
    const err = addToChat(el('div', 'error', 'Сервер не ответил. Вопрос сохранён.'));
    const retry = el('button', 'btn ghost', 'Повторить');
    retry.onclick = () => ask(text);
    err.appendChild(el('div', 'retry-row')).appendChild(retry);
    return;
  }
  const r = res.data;
  addToChat(el('div', 'bubble bot', r.text));
  (r.sources || []).slice(0, 2).forEach((s) => addToChat(renderSource(s)));
  (r.drafts || []).forEach((d) => addToChat(renderDraft(d)));
  if (r.retry) {
    const retry = el('button', 'btn ghost', 'Повторить');
    retry.onclick = () => ask(state.lastQuestion);
    addToChat(el('div', 'retry-row')).appendChild(retry);
  }
}

// ---------- события интерфейса ----------

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
$('theme').onclick = () => {
  const dark = document.documentElement.dataset.theme !== 'dark';
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  $('theme').textContent = dark ? '☾' : '☼';
};
document.querySelectorAll('[data-go]').forEach((b) => {
  b.onclick = () => { const t = $(b.dataset.go); if (t) t.scrollIntoView({ behavior: 'smooth' }); };
});
$('who').onchange = async (e) => {
  state.login = e.target.value;
  try { localStorage.setItem('buddy.login', state.login); } catch { /* не критично */ }
  document.querySelectorAll('#messages > :not(:first-child):not(#quick)').forEach((n) => n.remove());
  await loadAll();
  toast(`Вы вошли как ${state.me ? state.me.name : state.login}`);
};

(async () => { await loadUsers(); await loadAll(); })();
