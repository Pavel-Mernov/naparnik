"""Инструменты агента (docs/spec/05-tools-api.md).

Правила, которые проверяет КОД, а не промпт (docs/spec/01-roles-and-access.md, 04-agent-behavior.md):
- модели передаются только инструменты, разрешённые роли пользователя;
- пользователь берётся из сессии (ToolContext), аргументам модели про «кто я» не доверяем;
- связи «стажёр — наставник — руководитель» проверяются в каждом обработчике;
- инструменты с `confirm=True` не выполняются по вызову модели: агент возвращает черновик,
  выполнение — только после подтверждения человеком (лестница доверия).

Заготовка: календарь, встречи, выжимки, save_answer и напоминания — следующий шаг (см. TODO в конце).
"""
from __future__ import annotations

import re
from collections.abc import Callable
from dataclasses import dataclass
from datetime import date

from agent.store import Store, User

INTERN, MENTOR, MANAGER, EMPLOYEE = "intern", "mentor", "manager", "employee"
ALL = (INTERN, MENTOR, MANAGER, EMPLOYEE)
REASON_CODES = {"DATE_CONFLICT", "TOO_LATE", "NOT_NEEDED_YET", "NEED_INFO", "OTHER"}


class ToolError(Exception):
    def __init__(self, code: str, message: str, who_can: str | None = None):
        super().__init__(message)
        self.code, self.message, self.who_can = code, message, who_can

    def as_result(self) -> dict:
        err = {"code": self.code, "message": self.message}
        if self.who_can:
            err["who_can"] = self.who_can
        return {"ok": False, "error": err}


@dataclass
class ToolContext:
    store: Store
    user: User


@dataclass(frozen=True)
class Tool:
    name: str
    description: str
    schema: dict
    roles: tuple[str, ...]
    confirm: bool
    handler: Callable[[ToolContext, dict], dict]


def _schema(props: dict, required: list[str] | None = None) -> dict:
    return {"type": "object", "properties": props, "required": required or [], "additionalProperties": False}


def _target_intern(ctx: ToolContext, intern_id: str | None) -> User:
    """Чей маршрут смотрим: свой (стажёр) или подчинённого/подопечного (наставник, руководитель)."""
    me = ctx.user
    if me.role == INTERN:
        if intern_id and intern_id != me.id:
            raise ToolError("forbidden", "Можно смотреть только свой маршрут.")
        return me
    if not intern_id:
        raise ToolError("validation", "Укажи intern_id стажёра.")
    intern = ctx.store.user(intern_id)
    if me.id not in (intern.mentor, intern.manager):
        raise ToolError("forbidden", "Это не твой стажёр.")
    return intern


# ---------- чек-лист новичка ----------
def get_route(ctx: ToolContext, args: dict) -> dict:
    intern = _target_intern(ctx, args.get("intern_id"))
    steps = [s for s in ctx.store.steps_for(intern) if s["owner"] == INTERN or ctx.user.role != INTERN]
    mine = [s for s in steps if s["owner"] == INTERN]
    done = sum(s["status"] == "done" for s in mine)
    overdue = [s for s in mine if s["status"] != "done" and s["period"] < (intern.week or 1)]
    nxt = next((s for s in mine if s["status"] != "done"), None)
    return {"ok": True, "data": {
        "intern": intern.name, "week": intern.week, "progress_pct": round(100 * done / len(mine)) if mine else 0,
        "steps": steps, "overdue": overdue, "next_step": nxt,
    }}


def get_today_steps(ctx: ToolContext, args: dict) -> dict:
    me = ctx.user
    todo = [s for s in ctx.store.steps_for(me)
            if s["owner"] == INTERN and s["status"] != "done" and s["period"] <= (me.week or 1)]
    return {"ok": True, "data": {"week": me.week, "steps": todo[:3]}}


def get_step_details(ctx: ToolContext, args: dict) -> dict:
    for steps in ctx.store.checklist.values():
        for s in steps:
            if s["id"] == args["step_id"]:
                return {"ok": True, "data": s}
    raise ToolError("not_found", "Такого шага нет.")


def mark_step_done(ctx: ToolContext, args: dict) -> dict:
    intern = _target_intern(ctx, args.get("intern_id"))
    step_ids = {s["id"] for s in ctx.store.steps_for(intern)}
    if args["step_id"] not in step_ids:
        raise ToolError("not_found", "Такого шага нет в маршруте.")
    ctx.store.progress[intern.id][args["step_id"]] = "done"
    return {"ok": True, "data": {"step_id": args["step_id"], "status": "done"},
            "receipt": {"id": f"STEP-{args['step_id']}", "status": "done"}}


# ---------- отсутствия ----------
def get_absence_rules(ctx: ToolContext, args: dict) -> dict:
    rule = ctx.store.absence_rules.get(args["type"])
    if not rule:
        raise ToolError("not_found", "Таких правил нет. Есть: " + ", ".join(ctx.store.absence_rules))
    return {"ok": True, "data": {"type": args["type"], **rule}}


def _parse_date(value: str) -> date:
    try:
        return date.fromisoformat(value)
    except ValueError:
        raise ToolError("validation", "Дата в формате ГГГГ-ММ-ДД.")


def precheck_absence(ctx: ToolContext, args: dict) -> dict:
    d, store = _parse_date(args["date"]), ctx.store
    rule = store.absence_rules.get(args["type"], {})
    warnings = []
    if d <= store.today:
        warnings.append("Дата уже прошла или сегодня.")
    elif (d - store.today).days < rule.get("min_days_ahead", 0):
        warnings.append(f"По регламенту {args['type']} подаётся минимум за {rule['min_days_ahead']} дн.")
    if d in store.release_days:
        warnings.append("В этот день релиз юнита — согласование может быть сложнее.")
    for ev in store.calendar.get(ctx.user.id, []):
        if ev["date"] == args["date"]:
            warnings.append(f"В этот день встреча: {ev['title']}.")
    return {"ok": True, "data": {"date": args["date"], "warnings": warnings}}


def _manager_name(ctx: ToolContext) -> str:
    return ctx.store.user(ctx.user.manager).name if ctx.user.manager else "руководитель"


def create_absence_request(ctx: ToolContext, args: dict) -> dict:
    _parse_date(args["date"])
    rid = ctx.store.next_id("ABS")
    req = {"id": rid, "kind": "absence", "type": args["type"], "date": args["date"], "comment": args.get("comment", ""),
           "intern": ctx.user.id, "approver": ctx.user.manager, "status": "pending"}
    ctx.store.requests[rid] = req
    return {"ok": True, "data": req, "receipt": {"id": rid, "status": "pending", "approver": _manager_name(ctx)}}


def list_my_requests(ctx: ToolContext, args: dict) -> dict:
    return {"ok": True, "data": [r for r in ctx.store.requests.values() if r["intern"] == ctx.user.id]}


def decide_request(ctx: ToolContext, args: dict) -> dict:
    req = ctx.store.requests.get(args["request_id"])
    if req is None:
        raise ToolError("not_found", "Заявка не найдена.")
    if req["approver"] != ctx.user.id:
        raise ToolError("forbidden", "Согласовать может только руководитель этого стажёра.")
    if args["decision"] != "approve" and args.get("reason_code") not in REASON_CODES:
        raise ToolError("validation", "Отказ или другая дата — только с кодом причины: " + ", ".join(sorted(REASON_CODES)))
    req["status"] = {"approve": "approved", "reject": "rejected", "other_date": "other_date"}[args["decision"]]
    req["reason_code"], req["decision_comment"] = args.get("reason_code"), args.get("comment", "")
    return {"ok": True, "data": req, "receipt": {"id": req["id"], "status": req["status"]}}


# ---------- доступы ----------
def get_required_accesses(ctx: ToolContext, args: dict) -> dict:
    intern = _target_intern(ctx, args.get("intern_id"))
    plan = ctx.store.accesses.get(intern.department, {})
    systems = [s for week, items in plan.items() if int(week) <= (intern.week or 1) for s in items]
    requested = {r["system"]: r["status"] for r in ctx.store.requests.values()
                 if r["kind"] == "access" and r["intern"] == intern.id}
    return {"ok": True, "data": [{"system": s, "status": requested.get(s, "not_requested")} for s in systems]}


def request_access(ctx: ToolContext, args: dict) -> dict:
    rid = ctx.store.next_id("ACC")
    req = {"id": rid, "kind": "access", "system": args["system"], "reason": args["reason"], "intern": ctx.user.id,
           "approver": ctx.user.manager, "status": "pending"}
    ctx.store.requests[rid] = req
    return {"ok": True, "data": req, "receipt": {"id": rid, "status": "pending", "approver": _manager_name(ctx)}}


# ---------- база знаний ----------
STOP_WORDS = {"как", "что", "где", "это", "мне", "для", "через", "такое", "нужно", "можно", "почему", "когда",
              "какой", "какие", "кто", "мой", "моя", "мои", "или", "если", "есть"}
MIN_COVERAGE = 0.5  # доля слов вопроса, найденных в статье; ниже — «не нашёл», а не слабый ответ


def _stems(text: str) -> set[str]:
    words = re.findall(r"[a-zа-яё0-9]+", text.lower().replace("ё", "е"))
    return {w[:5] for w in words if len(w) > 2 and w not in STOP_WORDS}


def search_knowledge(ctx: ToolContext, args: dict) -> dict:
    """Ищет только в пространстве отдела пользователя (права по отделу — в коде)."""
    q = _stems(args["query"])
    scored = []
    for doc in ctx.store.knowledge.get(ctx.user.department, []):
        score = len(q & _stems(doc["title"] + " " + doc["text"]))
        if q and score / len(q) >= MIN_COVERAGE:
            scored.append((score, doc))
    hits = [d for _, d in sorted(scored, key=lambda x: -x[0])[:3]]
    return {"ok": True, "data": {"hits": hits, "found": bool(hits)}}


def get_glossary_term(ctx: ToolContext, args: dict) -> dict:
    term = args["term"].strip().lower()
    for g in ctx.store.glossary:
        if g["term"].lower() == term:
            return {"ok": True, "data": g}
    return {"ok": True, "data": None}


# ---------- люди и вопросы ----------
def find_person(ctx: ToolContext, args: dict) -> dict:
    topic = args["topic"].lower()
    people = [u for u in ctx.store.users.values()
              if u.department == ctx.user.department and u.role != INTERN and any(t in topic or topic in t for t in u.topics)]
    if not people and ctx.user.mentor:
        people = [ctx.store.user(ctx.user.mentor)]
    return {"ok": True, "data": [{"id": u.id, "name": u.name, "title": u.title, "hours": u.hours} for u in people]}


def escalate_question(ctx: ToolContext, args: dict) -> dict:
    to = ctx.store.user(args["addressee_id"])
    if to.department != ctx.user.department:
        raise ToolError("forbidden", "Вопрос можно отправить только коллеге своего отдела.")
    qid = ctx.store.next_id("Q")
    ctx.store.questions[qid] = {
        "id": qid, "to": to.id, "text": args["text"], "context": args.get("context", ""),
        "author": None if args.get("anonymous") else ctx.user.id, "status": "sent",
    }
    return {"ok": True, "data": {"id": qid, "to": to.name}, "receipt": {"id": qid, "status": "sent", "to": to.name}}


TOOLS: list[Tool] = [
    Tool("get_route", "Маршрут адаптации: этапы, шаги со статусами, прогресс, просроченное, следующий шаг.",
         _schema({"intern_id": {"type": "string", "description": "Только для наставника и руководителя"}}),
         ALL, False, get_route),
    Tool("get_today_steps", "1–3 шага на сегодня из текущего этапа чек-листа стажёра.",
         _schema({}), (INTERN,), False, get_today_steps),
    Tool("get_step_details", "Описание шага чек-листа.", _schema({"step_id": {"type": "string"}}, ["step_id"]),
         ALL, False, get_step_details),
    Tool("mark_step_done", "Отметить шаг чек-листа выполненным.",
         _schema({"step_id": {"type": "string"}, "intern_id": {"type": "string"}}, ["step_id"]),
         (INTERN, MENTOR), True, mark_step_done),
    Tool("get_absence_rules", "Порядок оформления отсутствия (отгул, отпуск).",
         _schema({"type": {"type": "string", "enum": ["отгул", "отпуск"]}}, ["type"]), ALL, False, get_absence_rules),
    Tool("precheck_absence", "Проверить дату отсутствия: сроки подачи, встречи, релизы.",
         _schema({"type": {"type": "string", "enum": ["отгул", "отпуск"]},
                  "date": {"type": "string", "description": "ГГГГ-ММ-ДД"}}, ["type", "date"]),
         (INTERN,), False, precheck_absence),
    Tool("create_absence_request", "Создать заявку на отсутствие в сервисе отсутствий (после подтверждения).",
         _schema({"type": {"type": "string", "enum": ["отгул", "отпуск"]},
                  "date": {"type": "string", "description": "ГГГГ-ММ-ДД"}, "comment": {"type": "string"}},
                 ["type", "date"]), (INTERN,), True, create_absence_request),
    Tool("list_my_requests", "Мои заявки и их статусы.", _schema({}), (INTERN,), False, list_my_requests),
    Tool("decide_request", "Согласовать или отклонить заявку подчинённого. Отказ — только с кодом причины.",
         _schema({"request_id": {"type": "string"},
                  "decision": {"type": "string", "enum": ["approve", "reject", "other_date"]},
                  "reason_code": {"type": "string", "enum": sorted(REASON_CODES)}, "comment": {"type": "string"}},
                 ["request_id", "decision"]), (MANAGER,), True, decide_request),
    Tool("get_required_accesses", "Какие доступы нужны на текущем этапе и их статусы.",
         _schema({"intern_id": {"type": "string"}}), (INTERN, MENTOR, MANAGER), False, get_required_accesses),
    Tool("request_access", "Создать заявку на доступ к системе (после подтверждения).",
         _schema({"system": {"type": "string"}, "reason": {"type": "string"}}, ["system", "reason"]),
         (INTERN,), True, request_access),
    Tool("search_knowledge", "Поиск в базе знаний отдела пользователя. Возвращает фрагменты с источником.",
         _schema({"query": {"type": "string"}}, ["query"]), ALL, False, search_knowledge),
    Tool("get_glossary_term", "Определение термина из жаргона команды.",
         _schema({"term": {"type": "string"}}, ["term"]), ALL, False, get_glossary_term),
    Tool("find_person", "Кто в отделе отвечает за тему.", _schema({"topic": {"type": "string"}}, ["topic"]),
         ALL, False, find_person),
    Tool("escalate_question", "Отправить вопрос коллеге (после подтверждения). anonymous — без имени автора.",
         _schema({"text": {"type": "string"}, "context": {"type": "string"}, "addressee_id": {"type": "string"},
                  "anonymous": {"type": "boolean"}}, ["text", "addressee_id"]), (INTERN,), True, escalate_question),
]
TOOLS_BY_NAME = {t.name: t for t in TOOLS}

# TODO (следующий шаг, docs/spec/05): suggest_slots, create_meeting, get_meeting_summary [заглушка],
# answer_question, save_answer, set_reminder; перенос инструментов в MCP-серверы-адаптеры.


def tools_for(user: User) -> list[Tool]:
    return [t for t in TOOLS if user.role in t.roles]


def run_tool(ctx: ToolContext, name: str, args: dict) -> dict:
    """Выполнить инструмент с проверкой прав. Ошибки — в формате {"ok": false, "error": {...}}."""
    tool = TOOLS_BY_NAME.get(name)
    if tool is None or ctx.user.role not in tool.roles:
        return ToolError("forbidden", "Это действие тебе недоступно.").as_result()
    try:
        return tool.handler(ctx, args)
    except ToolError as e:
        return e.as_result()
    except KeyError as e:
        return ToolError("validation", f"Не хватает или неверный параметр: {e}").as_result()
