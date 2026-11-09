"""Настройки агента из окружения (.env). Ключи в коде не хранятся — только имена переменных.

PDF стр. 9: для внешнего API указываем сервис, модель, переменную окружения и способ подключить ключ.
"""
from __future__ import annotations

import os
from dataclasses import dataclass

from dotenv import load_dotenv

load_dotenv()  # читает .env из текущей папки или выше; в Docker переменные приходят из compose

DEFAULT_MODELS = {
    "claude": "claude-opus-5-5",  # дешевле: LLM_MODEL=claude-sonnet-5-5
    "deepseek": "deepseek-chat",  # проверьте актуальное имя модели в кабинете DeepSeek
    "mock": "mock",
}


@dataclass(frozen=True)
class Settings:
    provider: str  # claude | deepseek | mock
    model: str
    max_tokens: int
    timeout_seconds: float
    max_tool_rounds: int  # защита от зацикливания агента


def load_settings() -> Settings:
    provider = os.getenv("LLM_PROVIDER", "mock").strip().lower()
    if provider not in DEFAULT_MODELS:
        raise ValueError(f"LLM_PROVIDER={provider!r}: допустимо claude, deepseek или mock")
    return Settings(
        provider=provider,
        model=os.getenv("LLM_MODEL") or DEFAULT_MODELS[provider],
        max_tokens=int(os.getenv("LLM_MAX_TOKENS", "16000")),
        timeout_seconds=float(os.getenv("LLM_TIMEOUT_SECONDS", "60")),
        max_tool_rounds=int(os.getenv("AGENT_MAX_TOOL_ROUNDS", "5")),
    )
