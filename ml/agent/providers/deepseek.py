"""DeepSeek через OpenAI-совместимый API. Ключ — DEEPSEEK_API_KEY в .env.

Адрес API — DEEPSEEK_BASE_URL (по умолчанию https://api.deepseek.com). Имя модели проверьте в кабинете DeepSeek.
"""
from __future__ import annotations

import json
import os

import openai

from agent.config import Settings
from agent.providers.base import Step, ToolCall, ToolSpec


class DeepSeekSession:
    def __init__(self, client: openai.OpenAI, settings: Settings, system: str, context: str, tools: list[ToolSpec]):
        self.client, self.settings = client, settings
        self.messages: list[dict] = [{"role": "system", "content": f"{system}\n\n{context}"}]
        self.tools = [{"type": "function", "function": {"name": t.name, "description": t.description,
                                                         "parameters": t.schema}} for t in tools]

    def _call(self) -> Step:
        try:
            resp = self.client.chat.completions.create(
                model=self.settings.model, messages=self.messages, tools=self.tools,
                max_tokens=min(self.settings.max_tokens, 8000),
            )
        except openai.AuthenticationError:
            return Step(text="Неверный ключ DEEPSEEK_API_KEY.", stop_reason="error")
        except (openai.APIConnectionError, openai.APITimeoutError):
            return Step(text="Модель недоступна. Шаги и заявки работают — открой их на сайте.", stop_reason="error")
        except openai.APIStatusError as e:
            return Step(text=f"Ошибка модели ({e.status_code}).", stop_reason="error")

        msg = resp.choices[0].message
        self.messages.append(msg.model_dump(exclude_none=True))
        calls = []
        for tc in msg.tool_calls or []:
            try:
                args = json.loads(tc.function.arguments or "{}")
            except json.JSONDecodeError:
                args = {}
            calls.append(ToolCall(tc.id, tc.function.name, args))
        return Step(text=msg.content or "", tool_calls=calls, stop_reason="tool_use" if calls else "end_turn")

    def send_user(self, text: str) -> Step:
        self.messages.append({"role": "user", "content": text})
        return self._call()

    def send_tool_results(self, results: list[tuple[ToolCall, str]]) -> Step:
        for call, content in results:
            self.messages.append({"role": "tool", "tool_call_id": call.id, "content": content})
        return self._call()


class DeepSeekProvider:
    def __init__(self, settings: Settings):
        self.settings = settings
        self.client = openai.OpenAI(
            api_key=os.getenv("DEEPSEEK_API_KEY"),
            base_url=os.getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
            timeout=settings.timeout_seconds,
        )

    def start(self, system: str, context: str, tools: list[ToolSpec]) -> DeepSeekSession:
        return DeepSeekSession(self.client, self.settings, system, context, tools)
