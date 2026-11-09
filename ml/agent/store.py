"""Имитации корпоративных сервисов на синтетических данных (docs/spec/05-tools-api.md, раздел «Имитации»).

В прототипе всё живёт в памяти и сбрасывается при перезапуске. Следующий шаг — перенести в PostgreSQL
за адаптерами MCP; интерфейс инструментов при этом не меняется.
"""
from __future__ import annotations

import copy
import itertools
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path

import yaml

FIXTURES = Path(__file__).parent / "data" / "fixtures.yaml"


@dataclass
class User:
    id: str
    name: str
    role: str  # intern | mentor | manager | employee
    department: str
    week: int | None = None
    mentor: str | None = None
    manager: str | None = None
    title: str = ""
    topics: list[str] = field(default_factory=list)
    hours: str = ""


class Store:
    def __init__(self, path: Path = FIXTURES):
        raw = yaml.safe_load(path.read_text(encoding="utf-8"))
        self.today = date.fromisoformat(raw["today"])
        self.departments = raw["departments"]
        self.users = {uid: User(id=uid, **u) for uid, u in raw["users"].items()}
        self.checklist = raw["checklist"]
        self.progress = copy.deepcopy(raw["progress"])
        self.knowledge = raw["knowledge"]
        self.glossary = raw["glossary"]
        self.absence_rules = raw["absence_rules"]
        self.calendar = raw["calendar"]
        self.release_days = {date.fromisoformat(d) for d in raw["release_days"]}
        self.accesses = raw["accesses"]
        self.requests: dict[str, dict] = {}  # отгулы и доступы: квитанции сервисов
        self.questions: dict[str, dict] = {}  # вопросы, эскалированные людям
        self._seq = itertools.count(1001)

    def next_id(self, prefix: str) -> str:
        return f"{prefix}-{next(self._seq)}"

    def user(self, uid: str) -> User:
        if uid not in self.users:
            raise KeyError(f"Нет пользователя {uid}")
        return self.users[uid]

    def steps_for(self, intern: User) -> list[dict]:
        done = self.progress.setdefault(intern.id, {})
        return [{**s, "status": done.get(s["id"], "todo")} for s in self.checklist.get(intern.department, [])]
