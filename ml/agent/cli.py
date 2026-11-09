"""Поговорить с агентом из терминала.

    uv run python -m agent.cli --user artem "что такое Шаталити?"
    uv run python -m agent.cli --user artem            # диалог, выход — пустая строка

Провайдер — из .env (LLM_PROVIDER=mock по умолчанию, работает без ключа).
Пользователи — ml/agent/data/fixtures.yaml: artem, nina, oleg, irina, liza, dima ...
"""
from __future__ import annotations

import argparse

from agent.loop import Agent


def show(agent: Agent, user: str, text: str) -> None:
    reply = agent.ask(user, text)
    print(f"\n🤖 {reply.text}")
    for s in reply.sources:
        print(f"   📎 {s['title']} — {s['source']}{' (демо)' if s['demo'] else ''}")
    for d in reply.drafts:
        print(f"   📝 Черновик {d.id}: {d.tool} {d.args}")
        if input("   Подтвердить? [y/N] ").strip().lower() == "y":
            res = agent.confirm(user, d.id)
            print(f"   ✅ {res.get('receipt') or res.get('error')}")


def main() -> None:
    p = argparse.ArgumentParser(description="AI-buddy в терминале")
    p.add_argument("--user", default="artem")
    p.add_argument("text", nargs="?")
    args = p.parse_args()
    agent = Agent()
    print(f"Провайдер: {agent.settings.provider}, модель: {agent.settings.model}, пользователь: {args.user}")
    if args.text:
        show(agent, args.user, args.text)
        return
    while text := input("\nТы: ").strip():
        show(agent, args.user, text)


if __name__ == "__main__":
    main()
