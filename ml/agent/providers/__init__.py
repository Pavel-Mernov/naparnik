from __future__ import annotations

from agent.config import Settings
from agent.providers.base import Provider
from agent.store import Store


def make_provider(settings: Settings, store: Store) -> Provider:
    """Провайдер выбирается переменной LLM_PROVIDER в .env."""
    if settings.provider == "claude":
        from agent.providers.claude import ClaudeProvider

        return ClaudeProvider(settings)
    if settings.provider == "deepseek":
        from agent.providers.deepseek import DeepSeekProvider

        return DeepSeekProvider(settings)
    from agent.providers.mock import MockProvider

    # Для правила «чужой маршрут»: основа имени в родительном падеже («Нины») → id
    people = {u.name.split()[0].lower()[:3]: u.id for u in store.users.values() if u.role == "intern"}
    return MockProvider(store.today, people)
