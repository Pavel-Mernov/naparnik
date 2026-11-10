"""Проверка целостности мок-данных AI-buddy.

Запуск из корня репозитория:  python3 docs/04-mock-data/check_mocks.py
Проверяет: JSON валиден; все ссылки на людей, страницы, шаги, встречи существуют;
каждый сценарий SC-01…SC-29 из 06-TEST-PERSONAS.md обеспечен данными.
"""
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REQ = HERE.parent / "03-requirements" / "06-TEST-PERSONAS.md"
errors = []


def load(name):
    try:
        return json.loads((HERE / name).read_text(encoding="utf-8"))
    except Exception as exc:  # noqa: BLE001
        errors.append(f"{name}: невалидный JSON ({exc})")
        return {}


files = sorted(p.name for p in HERE.glob("*.json"))
data = {f: load(f) for f in files}

team = data["team.json"]
users = {u["id"] for u in team.get("users", [])}
tpl_steps = {s["id"] for s in data["adaptation-plan-template.json"].get("steps", [])}
kb = data["kb-pages.json"]
pages = {p["id"] for p in kb.get("pages", [])}
future_pages = {p["id"] for p in kb.get("expected_after_scenarios", [])}
meetings = {m["id"]: m for m in data["meetings.json"].get("meetings", [])}
questions = {q["id"]: q for q in data["questions.json"].get("questions", [])}

USER_KEYS = {"author_id", "addressee", "approver_id", "organizer_id", "by", "to", "from",
             "buddy_id", "lead_id", "kb_owner_id", "asked_by", "suggested_addressee",
             "expected_assignee", "redirect_by", "approved_by", "done_by", "intern_id",
             "employee_id", "author_shown"}
USER_LIST_KEYS = {"participants", "buddy_for", "expect_offer"}
PAGE_KEYS = {"sources", "related_pages", "pages", "expect_sources"}


def walk(obj, where):
    if isinstance(obj, dict):
        for k, v in obj.items():
            if k in USER_KEYS and isinstance(v, str) and v.startswith("u-") and v not in users:
                errors.append(f"{where}: неизвестный пользователь {v} в поле {k}")
            if k in USER_LIST_KEYS and isinstance(v, list):
                for u in v:
                    if isinstance(u, str) and u.startswith("u-") and u not in users:
                        errors.append(f"{where}: неизвестный пользователь {u} в {k}")
            if k in PAGE_KEYS and isinstance(v, list):
                for p in v:
                    if isinstance(p, str) and p.startswith("kb-") and p not in pages | future_pages:
                        errors.append(f"{where}: неизвестная страница {p}")
            if k in {"step_id", "related_step", "links_step"} and isinstance(v, str) and v not in tpl_steps:
                errors.append(f"{where}: неизвестный шаг {v}")
            if k in {"meeting_id"} and isinstance(v, str) and v not in meetings:
                errors.append(f"{where}: неизвестная встреча {v}")
            if k in {"question_id", "source_question"} and isinstance(v, str) and v not in questions and v != "q-seed-open":
                errors.append(f"{where}: неизвестный вопрос {v}")
            walk(v, where)
    elif isinstance(obj, list):
        for item in obj:
            walk(item, where)


for name, content in data.items():
    walk(content, name)

# транскрипт
for mid, m in meetings.items():
    tf = m.get("transcript_file")
    if tf:
        path = HERE / tf
        if not path.exists():
            errors.append(f"meetings.json: нет файла транскрипта {tf}")
        else:
            stamps = re.findall(r"\*\*\[(\d\d):(\d\d)\]", path.read_text(encoding="utf-8"))
            last = max(int(a) * 60 + int(b) for a, b in stamps)
            if last > m["recording"]["duration_sec"]:
                errors.append(f"{mid}: таймкод {last}s длиннее записи")

# покрытие сценариев
text = REQ.read_text(encoding="utf-8")
scenarios = sorted(set(re.findall(r"\| (SC-\d\d) \|", text)))
blob = json.dumps(data, ensure_ascii=False)
uncovered = [sc for sc in scenarios if sc not in blob]
for sc in uncovered:
    errors.append(f"Сценарий {sc} не упомянут ни в одном файле мок-данных")

print(f"Файлов: {len(files)}, людей: {len(users)}, страниц: {len(pages)}, шагов: {len(tpl_steps)}, "
      f"вопросов: {len(questions)}, встреч: {len(meetings)}, сценариев: {len(scenarios)}, "
      f"eval-вопросов: {len(data['evals.json'].get('items', []))}")
if errors:
    print("ОШИБКИ:")
    for e in errors:
        print(" -", e)
    sys.exit(1)
print("OK: все ссылки целы, все сценарии обеспечены данными")
