"""Общий интерфейс провайдера LLM. Агент не знает, какая модель внутри: Claude, DeepSeek или mock.

Сессия хранит историю сообщений в «родном» формате своего API (у Claude и OpenAI-совместимых он разный),
поэтому наружу отдаёт только текст и вызовы инструментов.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol


@dataclass
class ToolSpec:
    name: str
    description: str
    schema: dict


@dataclass
class ToolCall:
    id: str
    name: str
    input: dict


@dataclass
class Step:
    text: str = ""
    tool_calls: list[ToolCall] = field(default_factory=list)
    stop_reason: str = "end_turn"  # end_turn | tool_use | refusal | max_tokens | error


class Session(Protocol):
    def send_user(self, text: str) -> Step: ...

    def send_tool_results(self, results: list[tuple[ToolCall, str]]) -> Step: ...


class Provider(Protocol):
    def start(self, system: str, context: str, tools: list[ToolSpec]) -> Session: ...
