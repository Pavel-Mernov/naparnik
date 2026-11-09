"""Claude через официальный Anthropic SDK. Ключ — ANTHROPIC_API_KEY в .env.

- Системный промпт и инструменты стабильны → кэшируются (дешевле повторные запросы).
- Контекст пользователя — отдельный блок после кэшируемой части.
- Ответ модели (включая блоки размышлений) возвращаем в историю без изменений.
- Серверный fallback при отказе модели включён по умолчанию (LLM_FALLBACKS=0 — выключить).
"""
from __future__ import annotations

import os

import anthropic

from agent.config import Settings
from agent.providers.base import Step, ToolCall, ToolSpec

FALLBACK_BETA = "server-side-fallback-2026-07-01"


class ClaudeSession:
    def __init__(self, client: anthropic.Anthropic, settings: Settings, system: str, context: str,
                 tools: list[ToolSpec]):
        self.client, self.settings = client, settings
        self.system = [
            {"type": "text", "text": system, "cache_control": {"type": "ephemeral"}},
            {"type": "text", "text": context},
        ]
        self.tools = [{"name": t.name, "description": t.description, "input_schema": t.schema} for t in tools]
        self.messages: list[dict] = []

    def _call(self) -> Step:
        kwargs = {
            "model": self.settings.model,
            "max_tokens": self.settings.max_tokens,
            "system": self.system,
            "tools": self.tools,
            "messages": self.messages,
            "output_config": {"effort": os.getenv("LLM_EFFORT", "medium")},
        }
        try:
            if os.getenv("LLM_FALLBACKS", "1") == "1":
                resp = self.client.beta.messages.create(betas=[FALLBACK_BETA], fallbacks="default", **kwargs)
            else:
                resp = self.client.messages.create(**kwargs)
        except anthropic.AuthenticationError:
            return Step(text="Неверный ключ ANTHROPIC_API_KEY.", stop_reason="error")
        except anthropic.RateLimitError:
            return Step(text="Слишком много запросов к модели, попробуй через минуту.", stop_reason="error")
        except (anthropic.APIConnectionError, anthropic.APITimeoutError):
            return Step(text="Модель недоступна. Шаги и заявки работают — открой их на сайте.", stop_reason="error")
        except anthropic.APIStatusError as e:
            return Step(text=f"Ошибка модели ({e.status_code}).", stop_reason="error")

        self.messages.append({"role": "assistant", "content": resp.content})
        if resp.stop_reason == "refusal":
            return Step(text="С этим запросом помочь не могу.", stop_reason="refusal")
        text = "".join(b.text for b in resp.content if b.type == "text")
        calls = [ToolCall(b.id, b.name, dict(b.input)) for b in resp.content if b.type == "tool_use"]
        return Step(text=text, tool_calls=calls, stop_reason=resp.stop_reason or "end_turn")

    def send_user(self, text: str) -> Step:
        self.messages.append({"role": "user", "content": text})
        return self._call()

    def send_tool_results(self, results: list[tuple[ToolCall, str]]) -> Step:
        # Все результаты — одним сообщением, иначе модель перестанет вызывать инструменты параллельно
        self.messages.append({"role": "user", "content": [
            {"type": "tool_result", "tool_use_id": call.id, "content": content} for call, content in results
        ]})
        return self._call()


class ClaudeProvider:
    def __init__(self, settings: Settings):
        self.settings = settings
        self.client = anthropic.Anthropic(timeout=settings.timeout_seconds)  # ключ из ANTHROPIC_API_KEY

    def start(self, system: str, context: str, tools: list[ToolSpec]) -> ClaudeSession:
        return ClaudeSession(self.client, self.settings, system, context, tools)
