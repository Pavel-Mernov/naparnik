"""Mock-провайдер без внешнего API: простые правила вместо модели.

Зачем: тесты и CI без ключа, демо без интернета, проверка экспертами без трат.
Проходит тот же путь, что настоящая модель: вызывает инструменты, получает результаты, пишет ответ.
"""
from __future__ import annotations

import itertools
import json
import re
from datetime import date, timedelta

from agent.providers.base import Step, ToolCall, ToolSpec

WEEKDAYS = {"понедельник": 0, "вторник": 1, "сред": 2, "четверг": 3, "пятниц": 4}


def parse_day(text: str, today: date) -> str | None:
    m = re.search(r"\d{4}-\d{2}-\d{2}", text)
    if m:
        return m.group(0)
    low = text.lower()
    if "завтра" in low:
        return (today + timedelta(days=1)).isoformat()
    for stem, wd in WEEKDAYS.items():
        if stem in low:
            delta = (wd - today.weekday()) % 7 or 7
            return (today + timedelta(days=delta)).isoformat()
    return None


class MockSession:
    def __init__(self, tools: list[ToolSpec], today: date, people: dict[str, str]):
        self.allowed = {t.name for t in tools}
        self.today, self.people = today, people
        self._ids = itertools.count(1)

    def _call(self, name: str, **args) -> ToolCall:
        return ToolCall(f"mock_{next(self._ids)}", name, args)

    def send_user(self, text: str) -> Step:
        low = text.lower()
        calls: list[ToolCall] = []
        if re.search(r"штат|оценк|зарплат", low):
            return Step(text="Это решают люди, а не я. Могу помочь сформулировать вопрос наставнику или руководителю.")
        other = next((uid for stem, uid in self.people.items() if re.search(rf"\b{stem}", low)), None)
        if other:
            calls.append(self._call("get_route", intern_id=other))
        elif m := re.match(r"(что такое|что значит|что означает)\s+(.+?)\??$", low):
            calls += [self._call("get_glossary_term", term=m.group(2)), self._call("search_knowledge", query=text)]
        elif "отгул" in low:
            day = parse_day(text, self.today)
            if not day:
                return Step(text="На какую дату нужен отгул?")
            calls += [self._call("get_absence_rules", type="отгул"), self._call("precheck_absence", type="отгул", date=day),
                      self._call("create_absence_request", type="отгул", date=day)]
        elif "доступ" in low and (m := re.search(r"доступ\w*\s+(?:к|в)\s+(.+?)\??$", text, re.IGNORECASE)):
            calls.append(self._call("request_access", system=m.group(1).strip(), reason="Шаг маршрута адаптации"))
        elif re.search(r"сегодня|что делать|чем заняться", low):
            calls.append(self._call("get_today_steps"))
        elif re.search(r"маршрут|прогресс|где я|успеваю", low):
            calls.append(self._call("get_route"))
        elif low.startswith("кто"):
            calls.append(self._call("find_person", topic=re.sub(r"^кто\s+(знает|отвечает за)?\s*", "", low).strip(" ?")))
        else:
            calls.append(self._call("search_knowledge", query=text))
        calls = [c for c in calls if c.name in self.allowed] or calls[:0]
        if not calls:
            return Step(text="Это действие тебе недоступно.")
        return Step(tool_calls=calls, stop_reason="tool_use")

    def send_tool_results(self, results: list[tuple[ToolCall, str]]) -> Step:
        parts: list[str] = []
        glossary_hit = False
        for call, raw in results:
            res = json.loads(raw)
            if not res.get("ok"):
                err = res["error"]
                parts.append(err["message"] + (f" Обратись: {err['who_can']}." if err.get("who_can") else ""))
                continue
            data = res.get("data")
            if res.get("status") == "needs_confirmation":
                parts.append("Подготовил черновик — проверь и подтверди, тогда отправлю.")
            elif call.name == "get_glossary_term" and data:
                glossary_hit = True
                parts.append(f"{data['term']} — {data['definition']} (Источник: {data['source']})")
            elif call.name == "search_knowledge" and not glossary_hit:
                if data["hits"]:
                    h = data["hits"][0]
                    demo = ", демо-регламент прототипа" if h["demo"] and "демо" not in h["source"].lower() else ""
                    parts.append(f"{h['text']} (Источник: {h['source']}{demo})")
                else:
                    parts.append("В базе знаний не нашёл. Могу сформулировать вопрос наставнику.")
            elif call.name == "get_today_steps":
                steps = "; ".join(s["title"] for s in data["steps"]) or "всё сделано"
                parts.append(f"Неделя {data['week']}. На сегодня: {steps}.")
            elif call.name == "get_route":
                nxt = data["next_step"]["title"] if data["next_step"] else "всё пройдено"
                parts.append(f"Прогресс {data['progress_pct']}%. Следующий шаг: {nxt}.")
            elif call.name == "precheck_absence" and data["warnings"]:
                parts.append("Обрати внимание: " + " ".join(data["warnings"]))
            elif call.name == "get_absence_rules":
                parts.append(f"{data['text']} (Источник: {data['source']})")
            elif call.name == "find_person":
                parts.append("По этой теме: " + ", ".join(f"{p['name']} ({p['title']})" for p in data))
        return Step(text="\n".join(parts) or "Готово.")


class MockProvider:
    def __init__(self, today: date, people: dict[str, str]):
        self.today, self.people = today, people

    def start(self, system: str, context: str, tools: list[ToolSpec]) -> MockSession:
        return MockSession(tools, self.today, self.people)
