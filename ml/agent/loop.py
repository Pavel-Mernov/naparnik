"""Цикл агента: вопрос → модель выбирает инструменты → код проверяет права и выполняет → модель отвечает.

Лестница доверия (docs/spec/04): инструменты с подтверждением превращаются в черновики. Выполняет их
только `Agent.confirm()` — по нажатию кнопки человеком, а не по решению модели.
"""
from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

from agent.config import Settings, load_settings
from agent.providers import make_provider
from agent.providers.base import Provider, ToolCall, ToolSpec
from agent.store import Store
from agent.tools import TOOLS_BY_NAME, ToolContext, ToolError, run_tool, tools_for

SYSTEM_PROMPT = (Path(__file__).parent / "prompts" / "system.md").read_text(encoding="utf-8")


@dataclass
class Draft:
    id: str
    user_id: str
    tool: str
    args: dict


@dataclass
class AgentReply:
    text: str
    drafts: list[Draft] = field(default_factory=list)  # что ждёт подтверждения (кнопки в UI)
    sources: list[dict] = field(default_factory=list)  # источники ответа — UI показывает их из данных, не из текста
    tool_log: list[dict] = field(default_factory=list)  # для журнала событий (clickstream) и отладки


class Agent:
    def __init__(self, settings: Settings | None = None, store: Store | None = None, provider: Provider | None = None):
        self.settings = settings or load_settings()
        self.store = store or Store()
        self.provider = provider or make_provider(self.settings, self.store)
        self.drafts: dict[str, Draft] = {}

    def _context(self, ctx: ToolContext) -> str:
        u, s = ctx.user, self.store
        lines = [
            "## Контекст",
            f"Сегодня: {s.today.isoformat()} ({['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'][s.today.weekday()]}).",
            f"Пользователь: {u.name}, роль: {u.role}, отдел: {s.departments[u.department]['name']}.",
        ]
        if u.week:
            lines.append(f"Неделя стажировки: {u.week}.")
        if u.mentor:
            lines.append(f"Наставник: {s.user(u.mentor).name} (id {u.mentor}).")
        if u.manager:
            lines.append(f"Руководитель: {s.user(u.manager).name} (id {u.manager}).")
        return "\n".join(lines)

    def ask(self, user_id: str, text: str) -> AgentReply:
        ctx = ToolContext(self.store, self.store.user(user_id))
        specs = [ToolSpec(t.name, t.description, t.schema) for t in tools_for(ctx.user)]
        session = self.provider.start(SYSTEM_PROMPT, self._context(ctx), specs)
        reply = AgentReply(text="")

        step = session.send_user(text)
        for _ in range(self.settings.max_tool_rounds):
            if not step.tool_calls:
                break
            results = [(call, json.dumps(self._handle(ctx, call, reply), ensure_ascii=False)) for call in step.tool_calls]
            step = session.send_tool_results(results)
        else:
            if step.tool_calls:
                step.text = "Не получилось довести запрос до конца. Попробуй сформулировать иначе."
        reply.text = step.text
        return reply

    def _handle(self, ctx: ToolContext, call: ToolCall, reply: AgentReply) -> dict:
        tool = TOOLS_BY_NAME.get(call.name)
        if tool and tool.confirm and ctx.user.role in tool.roles:
            draft = Draft(id=self.store.next_id("DRAFT"), user_id=ctx.user.id, tool=call.name, args=call.input)
            self.drafts[draft.id] = draft
            reply.drafts.append(draft)
            result = {"ok": True, "status": "needs_confirmation", "draft_id": draft.id, "draft": call.input}
        else:
            result = run_tool(ctx, call.name, call.input)
            data = result.get("data") if result.get("ok") else None
            if call.name == "search_knowledge" and data:
                reply.sources += [{"title": h["title"], "source": h["source"], "demo": h["demo"]} for h in data["hits"]]
            if call.name == "get_glossary_term" and data:
                reply.sources.append({"title": data["term"], "source": data["source"], "demo": data["demo"]})
        reply.tool_log.append({"tool": call.name, "args": call.input, "ok": result.get("ok")})
        return result

    def confirm(self, user_id: str, draft_id: str) -> dict:
        """Человек нажал «Подтвердить»: выполнить черновик и вернуть квитанцию."""
        draft = self.drafts.get(draft_id)
        if draft is None or draft.user_id != user_id:
            return ToolError("not_found", "Черновик не найден.").as_result()
        del self.drafts[draft_id]
        return run_tool(ToolContext(self.store, self.store.user(user_id)), draft.tool, draft.args)
