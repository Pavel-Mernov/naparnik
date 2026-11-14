#!/usr/bin/env python3
"""Приёмка отдельного Docker-стенда. Создаёт синтетические данные и перезапускает его сервисы.

Запуск и очистка описаны в docs/ACCEPTANCE.md. Рабочий Docker-проект не затрагивается.
"""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]
PROJECT = "naparnik-acceptance"
BASE = "http://127.0.0.1:18001"
ARTIFACTS = ROOT / ".artifacts" / "acceptance"
USER = "intern.sokolov"


def api(path, body=None, method=None, user=USER):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(), method=method,
                                 headers={"X-User": user, "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, json.load(r)
    except urllib.error.HTTPError as e:
        return e.code, json.load(e)


def compose(*args, mode="stub_ok", fail=""):
    env = os.environ.copy()
    for key in ("DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"):
        env.pop(key, None)
    env.update(ACCEPTANCE_LLM_MODE=mode, ACCEPTANCE_MOCK_FAIL=fail)
    cmd = ["docker", "--host=unix:///var/run/docker.sock", "compose", "-p", PROJECT,
           "-f", str(ROOT / "docker-compose.yml"), "-f", str(ROOT / "docker/compose.acceptance.yml"), *args]
    subprocess.run(cmd, cwd=ROOT, env=env, check=True, stdout=subprocess.DEVNULL)


def ready():
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            if api("/api/naparnik")[0] == 200:
                return
        except (OSError, ValueError):
            pass
        time.sleep(0.3)
    raise AssertionError("Стенд не восстановился за 60 секунд")


def plan(user="buddy.belova", intern="u-egorova"):
    status, data = api(f"/api/plans/{intern}/draft", user=user)
    assert status == 200, data
    return data


def browser_checks():
    errors = []
    with sync_playwright() as p:
        opts = {"headless": True}
        if os.environ.get("CHROMIUM_PATH"):
            opts["executable_path"] = os.environ["CHROMIUM_PATH"]
        browser = p.chromium.launch(**opts)

        def open_page(login, width=1440):
            c = browser.new_context(viewport={"width": width, "height": 1000})
            c.add_init_script(f"localStorage.setItem('naparnik.login', {json.dumps(login)})")
            page = c.new_page()
            page.on("pageerror", lambda e: errors.append(str(e)))
            page.goto(BASE)
            expect(page.get_by_role("button", name="Открыть чат с Напарником", exact=True)).to_be_visible()
            return page

        def open_plan(page):
            page.get_by_test_id("plan-control-u-egorova").click()
            expect(page.get_by_role("dialog")).to_be_visible()
            return page.get_by_role("dialog")

        try:
            buddy = open_page("buddy.belova")
            dialog = open_plan(buddy)
            title = plan()["steps"][0]["title"]
            dialog.get_by_label("Срок: " + title, exact=True).fill("2026-12-11")
            dialog.get_by_label("Комментарий руководителю", exact=True).fill("План учитывает проект стажёра")
            dialog.get_by_role("button", name="Сохранить правки", exact=True).click()
            expect(dialog.get_by_role("button", name="Отправить на согласование", exact=True)).to_be_enabled()
            dialog.get_by_role("button", name="Отправить на согласование", exact=True).click()
            expect(dialog.get_by_text("Действующего плана пока нет · Редакция 1: На согласовании", exact=True)).to_be_visible()
            lead = open_page("lead.kovalev")
            lead_dialog = open_plan(lead)
            lead_dialog.get_by_role("button", name="Вернуть бадди", exact=True).click()
            lead_dialog.get_by_label("Что нужно доработать", exact=True).fill("Перенеси знакомство на понедельник")
            lead_dialog.get_by_role("button", name="Вернуть с комментарием", exact=True).click()
            expect(lead_dialog.get_by_text("Действующего плана пока нет · Редакция 1: Возвращена бадди", exact=True)).to_be_visible()
            buddy.reload()
            dialog = open_plan(buddy)
            expect(dialog.get_by_text("Перенеси знакомство на понедельник", exact=True).first).to_be_visible()
            dialog.get_by_label("Срок: " + title, exact=True).fill("2026-12-14")
            dialog.get_by_label("Комментарий руководителю", exact=True).fill("Замечание учтено: знакомство в понедельник")
            dialog.get_by_role("button", name="Сохранить правки", exact=True).click()
            expect(dialog.get_by_role("button", name="Отправить на согласование", exact=True)).to_be_enabled()
            dialog.get_by_role("button", name="Отправить на согласование", exact=True).click()
            expect(dialog.get_by_text("Действующего плана пока нет · Редакция 1: На согласовании", exact=True)).to_be_visible()
            lead.reload()
            lead_dialog = open_plan(lead)
            lead_dialog.get_by_role("button", name="Утвердить план", exact=True).click()
            expect(lead_dialog.get_by_text("Действующая редакция: 1", exact=True)).to_be_visible()
            assert plan()["steps"][0]["due_date"] == "2026-12-14"

            # Новая редакция активного плана: прогресс не сбрасывается, несогласованные правки не видны стажёру.
            active_path = "/api/plans/u-egorova"
            first = api(active_path, user="intern.egorova")[1]["data"]
            step = first["steps"][0]
            assert api("/api/plan-steps/" + step["id"], {"intern_id": "u-egorova"}, "PATCH", "intern.egorova")[0] == 200
            buddy.reload()
            dialog = open_plan(buddy)
            dialog.get_by_role("button", name="Изменить действующий план", exact=True).click()
            expect(dialog.get_by_label("Комментарий руководителю", exact=True)).to_be_visible()
            expect(dialog.get_by_label("Срок: " + title, exact=True)).to_be_disabled()
            editable = next(s for s in plan()["steps"] if not s["done"] and not s.get("removed"))
            old_date = editable["due_date"]
            dialog.get_by_label("Срок: " + editable["title"], exact=True).fill("2026-12-21")
            dialog.get_by_label("Комментарий руководителю", exact=True).fill("Перенос с учётом рабочей нагрузки")
            dialog.get_by_role("button", name="Сохранить правки", exact=True).click()
            expect(dialog.get_by_role("button", name="Отправить на согласование", exact=True)).to_be_enabled()
            pending = plan()
            status, conflict = api("/api/plans/u-egorova/draft", {"version": pending["version"] - 1, "comment": "Устаревшая вкладка"}, "PATCH", "buddy.belova")
            assert status == 409, conflict
            live = api(active_path, user="intern.egorova")[1]["data"]
            assert next(s for s in live["steps"] if s["id"] == editable["id"])["due_date"] == old_date
            dialog.get_by_role("button", name="Отправить на согласование", exact=True).click()
            expect(dialog.get_by_text("Действующая редакция: 1 · Редакция 2: На согласовании", exact=True)).to_be_visible()
            compose("restart", "backend")
            ready()
            assert plan()["review_status"] == "review"
            lead.reload()
            lead_dialog = open_plan(lead)
            lead_dialog.get_by_role("button", name="Утвердить план", exact=True).click()
            expect(lead_dialog.get_by_text("Действующая редакция: 2", exact=True)).to_be_visible()
            live = api(active_path, user="intern.egorova")[1]["data"]
            assert live["steps"][0]["status"] == "Done"
            assert next(s for s in live["steps"] if s["id"] == editable["id"])["due_date"] == "2026-12-21"
            assert len({s["id"] for s in live["steps"]}) == len(live["steps"])
            intern = open_page("intern.egorova")
            intern.get_by_role("button", name="Мой план", exact=True).click()
            intern.get_by_role("button", name="История плана", exact=True).click()
            expect(intern.get_by_text("Редакция 2 · Утверждён", exact=True)).to_be_visible()
            intern.screenshot(path=str(ARTIFACTS / "intern-plan.png"), full_page=True)
            mobile = open_page("buddy.belova", 375)
            dialog = open_plan(mobile)
            expect(dialog.get_by_text("Действующая редакция: 2", exact=True)).to_be_visible()
            assert mobile.evaluate("document.documentElement.scrollWidth <= innerWidth"), "Горизонтальная прокрутка на 375 px"
            mobile.screenshot(path=str(ARTIFACTS / "plan-375.png"), full_page=False)

            # Основной цикл знаний: вопрос стажёра → эксперт → публикация → ответ следующему стажёру.
            question = "Почему конверсия в дашборде отличается от выгрузки из DWH?"
            asker = open_page("intern.sokolov")
            asker.get_by_role("button", name="Открыть чат с Напарником", exact=True).click()
            asker.get_by_placeholder("Спроси о команде или задачах").fill(question)
            asker.get_by_role("button", name="Отправить", exact=True).click()
            expect(asker.get_by_role("button", name="Подтвердить", exact=True)).to_be_enabled()
            asker.get_by_role("button", name="Подтвердить", exact=True).click()
            expect(asker.get_by_text("✓ Готово:", exact=False)).to_be_visible()
            sent = api("/api/questions")[1]["questions"]
            assert len(sent) == 1 and sent[0]["addressee_id"] == "u-orlov", sent
            expert = open_page("expert.orlov")
            expert.get_by_label("Твой ответ", exact=True).fill("Проверь одинаковые фильтры, дату обновления витрины DWH и часовой пояс дашборда. Разница конверсии возникает из-за разных срезов выгрузки.")
            expert.get_by_role("button", name="Ответить", exact=True).click()
            expert.get_by_role("button", name="Проверить", exact=True).click()
            expert.get_by_role("button", name="Ок, опубликовать", exact=True).click()
            expect(expert.get_by_text("Страница опубликована в базе знаний", exact=True)).to_be_visible()
            page_id = api("/api/questions")[1]["questions"][0]["page_id"]
            second = open_page("intern.egorova")
            second.get_by_role("button", name="Открыть чат с Напарником", exact=True).click()
            second.get_by_placeholder("Спроси о команде или задачах").fill(question)
            second.get_by_role("button", name="Отправить", exact=True).click()
            expect(second.get_by_role("button", name="Помогло", exact=True)).to_be_visible()
            answer = api("/api/chat/history", user="intern.egorova")[1]["turns"][-1]["reply"]
            assert not answer["drafts"] and any(s["page_id"] == page_id for s in answer["sources"]), answer
            assert not errors, errors
        except Exception:
            for i, c in enumerate(browser.contexts):
                for j, page in enumerate(c.pages):
                    page.screenshot(path=str(ARTIFACTS / f"failure-{i}-{j}.png"), full_page=True)
            raise
        finally:
            browser.close()


def resilience_checks():
    original = plan()
    # Данные сохраняются после перезапуска обоих хранилищ и backend.
    compose("restart", "postgres", "redis", "backend")
    ready()
    assert plan()["active_revision"] == original["active_revision"]
    assert plan()["history"] == original["history"]
    compose("up", "-d", "--no-deps", "--force-recreate", "backend", mode="stub_error", fail="absence")
    ready()
    code, reply = api("/api/chat/messages", {"text": "Мне нужен отгул 2026-11-20"})
    assert code == 200 and reply["degraded"] and reply["drafts"], reply
    draft = reply["drafts"][0]
    confirm = "/api/drafts/" + draft["id"] + "/confirm"
    code, failed = api(confirm, {"version": draft["version"]})
    assert code == 503, failed
    assert api("/api/drafts/" + draft["id"])[1]["draft"]["status"] == "pending"
    compose("up", "-d", "--no-deps", "--force-recreate", "backend")
    ready()
    code, receipt = api(confirm, {"version": draft["version"]})
    assert code == 200, receipt
    assert api(confirm, {"version": draft["version"]})[1] == receipt
    requests = api("/api/requests")[1]["requests"]
    assert len([r for r in requests if r["kind"] == "absence" and r["date"] == "2026-11-20"]) == 1
    assert plan()["history"] == original["history"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", default=PROJECT, help="отдельный проект Compose с конфигурацией приёмки")
    args = parser.parse_args()
    if args.project != PROJECT:
        raise SystemExit("Используй отдельный проект naparnik-acceptance, чтобы не перезапускать рабочий стенд")
    ARTIFACTS.mkdir(parents=True, exist_ok=True)
    ready()
    assert api("/api/naparnik")[1]["cache"]["kind"] == "redis"
    assert plan()["version"] == 1, "Нужен свежий отдельный стенд приёмки"
    assert api("/api/today", user="lead.kovalev")[0] == 403
    assert api("/api/plans/u-egorova/draft", user="expert.orlov")[0] == 403
    contracts = {"buddyDraft": plan(), "leadDraft": plan("lead.kovalev"),
                 "internHistory": api("/api/plans/u-sokolov/history")[1]}
    (ARTIFACTS / "contracts.json").write_text(json.dumps(contracts, ensure_ascii=False, indent=2) + "\n")
    browser_checks()
    print("PASS: четыре роли, цикл знаний, возврат, доработка, повторное согласование, история, прогресс, конфликт версии, 375 px", flush=True)
    resilience_checks()
    print("PASS: PostgreSQL/Redis/backend restart, сбой модели и сервиса, безопасный повтор, сохранность истории", flush=True)
    report = {"result": "passed", "project": PROJECT, "at": datetime.now(timezone.utc).isoformat(),
              "base_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
              "working_tree_changed": subprocess.run(["git", "diff", "--quiet"], cwd=ROOT).returncode != 0,
              "checks": ["roles", "knowledge_loop", "plan_lifecycle", "history", "progress", "version_conflict", "375px", "restart", "model_failure", "service_failure", "safe_retry"]}
    (ARTIFACTS / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")


if __name__ == "__main__":
    main()
