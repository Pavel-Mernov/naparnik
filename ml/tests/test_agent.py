"""Тесты агента на mock-провайдере (без ключа и интернета). Проверяют правила из docs/spec/01, 03, 04."""
import json

import pytest

from agent.config import Settings, load_settings
from agent.loop import Agent
from agent.providers.base import Step, ToolCall
from agent.store import Store
from agent.tools import ToolContext, run_tool, tools_for

MOCK = Settings(provider="mock", model="mock", max_tokens=1000, timeout_seconds=5, max_tool_rounds=5)


@pytest.fixture
def agent() -> Agent:
    return Agent(settings=MOCK, store=Store())


def ctx(agent: Agent, uid: str) -> ToolContext:
    return ToolContext(agent.store, agent.store.user(uid))


# --- С2: ответ из базы с источником ---
def test_glossary_answer_has_source(agent):
    reply = agent.ask("artem", "что такое Шаталити?")
    assert "Шаталити" in reply.text and reply.sources
    assert all(s["source"] for s in reply.sources)


def test_demo_source_is_marked(agent):
    reply = agent.ask("artem", "как настроить avito-cli")
    assert any(s["demo"] for s in reply.sources) and "демо" in reply.text.lower()


def test_knowledge_is_scoped_to_department(agent):
    hits = run_tool(ctx(agent, "liza"), "search_knowledge", {"query": "Шаталити планирование"})["data"]["hits"]
    assert hits == []  # статья Backend недоступна Frontend-стажёру


def test_not_found_offers_human(agent):
    reply = agent.ask("artem", "как задеплоить сервис на стейдж через кубер")
    assert "не нашёл" in reply.text.lower()


# --- С1: маршрут ---
def test_today_steps_only_intern_steps_max_three(agent):
    steps = run_tool(ctx(agent, "artem"), "get_today_steps", {})["data"]["steps"]
    assert 0 < len(steps) <= 3 and all(s["owner"] == "intern" and s["status"] != "done" for s in steps)


def test_route_progress_and_next_step(agent):
    data = run_tool(ctx(agent, "artem"), "get_route", {})["data"]
    assert 0 < data["progress_pct"] < 100 and data["next_step"]["status"] != "done"


# --- С4: отгул, лестница доверия, согласование ---
def test_absence_creates_draft_not_request(agent):
    reply = agent.ask("artem", "мне нужен отгул в пятницу")
    assert [d.tool for d in reply.drafts] == ["create_absence_request"]
    assert agent.store.requests == {}  # без подтверждения заявки нет


def test_precheck_warns_about_meeting(agent):
    reply = agent.ask("artem", "мне нужен отгул в пятницу")
    assert "1:1" in reply.text


def test_confirm_then_manager_approves(agent):
    draft = agent.ask("artem", "мне нужен отгул в пятницу").drafts[0]
    receipt = agent.confirm("artem", draft.id)["receipt"]
    assert receipt["status"] == "pending" and receipt["approver"] == "Ирина Соколова"
    res = run_tool(ctx(agent, "irina"), "decide_request", {"request_id": receipt["id"], "decision": "approve"})
    assert res["data"]["status"] == "approved"


def test_confirm_only_by_owner(agent):
    draft = agent.ask("artem", "мне нужен отгул в пятницу").drafts[0]
    assert agent.confirm("nina", draft.id)["error"]["code"] == "not_found"


def test_reject_requires_reason_code(agent):
    rid = run_tool(ctx(agent, "artem"), "create_absence_request", {"type": "отгул", "date": "2026-11-20"})["receipt"]["id"]
    res = run_tool(ctx(agent, "irina"), "decide_request", {"request_id": rid, "decision": "reject"})
    assert res["error"]["code"] == "validation"


def test_other_department_manager_cannot_decide(agent):
    rid = run_tool(ctx(agent, "artem"), "create_absence_request", {"type": "отгул", "date": "2026-11-20"})["receipt"]["id"]
    res = run_tool(ctx(agent, "maria"), "decide_request", {"request_id": rid, "decision": "approve"})
    assert res["error"]["code"] == "forbidden"


# --- С6: права ---
def test_intern_has_no_decide_tool(agent):
    names = {t.name for t in tools_for(agent.store.user("artem"))}
    assert "decide_request" not in names and "escalate_question" in names


def test_intern_cannot_decide_even_if_called(agent):
    res = run_tool(ctx(agent, "artem"), "decide_request", {"request_id": "ABS-1", "decision": "approve"})
    assert res["error"]["code"] == "forbidden"


def test_cannot_see_other_intern_route(agent):
    reply = agent.ask("artem", "покажи маршрут Нины")
    assert "только свой" in reply.text


def test_mentor_sees_own_intern_route(agent):
    assert run_tool(ctx(agent, "oleg"), "get_route", {"intern_id": "artem"})["ok"]
    assert run_tool(ctx(agent, "denis"), "get_route", {"intern_id": "artem"})["error"]["code"] == "forbidden"


def test_staffing_question_goes_to_human(agent):
    reply = agent.ask("artem", "меня возьмут в штат?")
    assert "люди" in reply.text and not reply.drafts


# --- С3: вопрос человеку ---
def test_anonymous_question_has_no_author(agent):
    res = run_tool(ctx(agent, "artem"), "escalate_question",
                   {"text": "как задеплоить на стейдж?", "addressee_id": "oleg", "anonymous": True})
    assert agent.store.questions[res["data"]["id"]]["author"] is None


def test_question_only_within_department(agent):
    res = run_tool(ctx(agent, "artem"), "escalate_question", {"text": "?", "addressee_id": "denis"})
    assert res["error"]["code"] == "forbidden"


# --- С5: доступы ---
def test_required_accesses_by_week(agent):
    systems = [a["system"] for a in run_tool(ctx(agent, "artem"), "get_required_accesses", {})["data"]]
    assert "Grafana" in systems and "Redash" not in systems  # неделя 2


# --- цикл агента ---
class LoopingSession:
    def send_user(self, text):
        return Step(tool_calls=[ToolCall("1", "get_route", {})])

    def send_tool_results(self, results):
        return Step(tool_calls=[ToolCall("2", "get_route", {})])


class LoopingProvider:
    def start(self, *a):
        return LoopingSession()


def test_loop_stops_after_max_rounds():
    agent = Agent(settings=MOCK, store=Store(), provider=LoopingProvider())
    assert "не получилось" in agent.ask("artem", "?").text.lower()


def test_unknown_tool_is_forbidden(agent):
    res = run_tool(ctx(agent, "artem"), "drop_database", {})
    assert res["error"]["code"] == "forbidden"
    assert json.dumps(res, ensure_ascii=False)


def test_invalid_provider_rejected(monkeypatch):
    monkeypatch.setenv("LLM_PROVIDER", "gpt")
    with pytest.raises(ValueError):
        load_settings()
