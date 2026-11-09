"""Провайдеры Claude и DeepSeek на подставных клиентах: проверяем разбор ответа и формат истории без сети и ключа."""
import json
from types import SimpleNamespace as NS

import anthropic
import httpx
import httpx2  # anthropic SDK 1.x+ работает на httpx2
import openai

from agent import cli
from agent.config import Settings
from agent.loop import Agent
from agent.providers import make_provider
from agent.providers.base import ToolCall, ToolSpec
from agent.providers.claude import ClaudeProvider, ClaudeSession
from agent.providers.deepseek import DeepSeekProvider, DeepSeekSession
from agent.store import Store

SPEC = [ToolSpec("get_route", "Маршрут", {"type": "object", "properties": {}})]


def settings(provider: str) -> Settings:
    return Settings(provider=provider, model="m", max_tokens=1000, timeout_seconds=5, max_tool_rounds=5)


# ---------- Claude ----------
class FakeMessages:
    def __init__(self, responses):
        self.responses, self.calls = list(responses), []

    def create(self, **kwargs):
        self.calls.append(kwargs)
        r = self.responses.pop(0)
        if isinstance(r, Exception):
            raise r
        return r


def claude_resp(*blocks, stop="end_turn"):
    return NS(content=list(blocks), stop_reason=stop)


def test_claude_tool_call_then_text(monkeypatch):
    monkeypatch.setenv("LLM_FALLBACKS", "1")
    fake = FakeMessages([
        claude_resp(NS(type="tool_use", id="t1", name="get_route", input={}), stop="tool_use"),
        claude_resp(NS(type="text", text="Прогресс 40%")),
    ])
    client = NS(beta=NS(messages=fake), messages=fake)
    s = ClaudeSession(client, settings("claude"), "sys", "ctx", SPEC)
    step = s.send_user("где я?")
    assert step.tool_calls[0].name == "get_route"
    step = s.send_tool_results([(step.tool_calls[0], '{"ok": true}')])
    assert step.text == "Прогресс 40%"
    first = fake.calls[0]
    assert first["system"][0]["cache_control"] == {"type": "ephemeral"} and first["fallbacks"] == "default"
    # результаты инструментов — одним сообщением пользователя
    assert s.messages[2]["content"][0]["tool_use_id"] == "t1"


def test_claude_without_fallbacks_and_refusal(monkeypatch):
    monkeypatch.setenv("LLM_FALLBACKS", "0")
    fake = FakeMessages([claude_resp(stop="refusal")])
    s = ClaudeSession(NS(messages=fake), settings("claude"), "sys", "ctx", SPEC)
    assert s.send_user("?").stop_reason == "refusal"
    assert "fallbacks" not in fake.calls[0]


def test_claude_errors_become_messages(monkeypatch):
    monkeypatch.setenv("LLM_FALLBACKS", "0")
    req = httpx2.Request("POST", "https://api.anthropic.com/v1/messages")
    errors = [
        anthropic.AuthenticationError("bad", response=httpx2.Response(401, request=req), body=None),
        anthropic.RateLimitError("slow", response=httpx2.Response(429, request=req), body=None),
        anthropic.APIConnectionError(request=req),
        anthropic.InternalServerError("boom", response=httpx2.Response(500, request=req), body=None),
    ]
    for err in errors:
        s = ClaudeSession(NS(messages=FakeMessages([err])), settings("claude"), "sys", "ctx", SPEC)
        assert s.send_user("?").stop_reason == "error"


def test_claude_provider_builds(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "test")
    p = ClaudeProvider(settings("claude"))
    assert isinstance(p.start("s", "c", SPEC), ClaudeSession)
    assert isinstance(make_provider(settings("claude"), Store()), ClaudeProvider)


# ---------- DeepSeek ----------
class FakeCompletions(FakeMessages):
    pass


def ds_resp(content=None, tool_calls=None):
    msg = NS(content=content, tool_calls=tool_calls,
             model_dump=lambda exclude_none=True: {"role": "assistant", "content": content})
    return NS(choices=[NS(message=msg)])


def test_deepseek_tool_call_then_text():
    tc = NS(id="c1", function=NS(name="get_route", arguments=json.dumps({"intern_id": "artem"})))
    bad = NS(id="c2", function=NS(name="get_route", arguments="{broken"))
    fake = FakeCompletions([ds_resp(tool_calls=[tc, bad]), ds_resp(content="Готово")])
    s = DeepSeekSession(NS(chat=NS(completions=fake)), settings("deepseek"), "sys", "ctx", SPEC)
    step = s.send_user("где я?")
    assert [c.input for c in step.tool_calls] == [{"intern_id": "artem"}, {}]
    step = s.send_tool_results([(ToolCall("c1", "get_route", {}), "{}")])
    assert step.text == "Готово" and s.messages[-2]["role"] == "tool"
    assert fake.calls[0]["tools"][0]["function"]["name"] == "get_route"


def test_deepseek_errors_become_messages():
    req = httpx.Request("POST", "https://api.deepseek.com/chat/completions")  # openai SDK работает на httpx
    errors = [
        openai.AuthenticationError("bad", response=httpx.Response(401, request=req), body=None),
        openai.APIConnectionError(request=req),
        openai.InternalServerError("boom", response=httpx.Response(500, request=req), body=None),
    ]
    for err in errors:
        s = DeepSeekSession(NS(chat=NS(completions=FakeCompletions([err]))), settings("deepseek"), "s", "c", SPEC)
        assert s.send_user("?").stop_reason == "error"


def test_deepseek_provider_builds(monkeypatch):
    monkeypatch.setenv("DEEPSEEK_API_KEY", "test")
    assert isinstance(DeepSeekProvider(settings("deepseek")).start("s", "c", SPEC), DeepSeekSession)
    assert isinstance(make_provider(settings("deepseek"), Store()), DeepSeekProvider)


# ---------- CLI ----------
def test_cli_single_question(monkeypatch, capsys):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    monkeypatch.setattr("sys.argv", ["cli", "--user", "artem", "мне нужен отгул в пятницу"])
    monkeypatch.setattr("builtins.input", lambda *_: "y")
    cli.main()
    out = capsys.readouterr().out
    assert "Черновик" in out and "pending" in out


def test_cli_dialog_and_sources(monkeypatch, capsys):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    monkeypatch.setattr("sys.argv", ["cli", "--user", "artem"])
    answers = iter(["что такое Шаталити?", ""])
    monkeypatch.setattr("builtins.input", lambda *_: next(answers))
    cli.main()
    assert "📎" in capsys.readouterr().out


def test_agent_context_for_mentor():
    agent = Agent(settings=settings("mock"), store=Store())
    text = agent.ask("oleg", "что такое LSR?").text
    assert "Live Site Review" in text
