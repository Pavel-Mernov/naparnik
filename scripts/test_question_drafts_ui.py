#!/usr/bin/env python3
"""BR-14, SC-05: отдельный backend с заглушкой и свежей тестовой БД, frontend Vite.

APP_URL=http://127.0.0.1:5173, API_URL=http://127.0.0.1:8080, CHROMIUM_PATH — путь Chromium.
Скрипт создаёт тестовый вопрос и черновики. Не запускайте на общей демо-БД.
"""
import json
import os
import urllib.error
import urllib.request

from playwright.sync_api import sync_playwright, expect

QUESTION = "Как выбрать длительность A/B-теста для низкого трафика?"
EDITED = "Как исключить ботов из выборки эксперимента? Проверил фильтры."
LATEST = "Как исключить ботов из выборки эксперимента? Уточнение из первой вкладки."


def api(path, body=None, method=None, user="intern.sokolov"):
    req = urllib.request.Request(
        os.environ.get("API_URL", "http://127.0.0.1:8080") + path,
        data=None if body is None else json.dumps(body).encode(), method=method,
        headers={"X-User": user, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def main():
    assert not api("/api/drafts")[1]["drafts"], "Нужен отдельный backend без ожидающих черновиков"
    initial_questions = len(api("/api/questions")[1]["questions"])
    with sync_playwright() as p:
        opts = {"headless": True}
        if os.environ.get("CHROMIUM_PATH"):
            opts["executable_path"] = os.environ["CHROMIUM_PATH"]
        browser = p.chromium.launch(**opts)
        context = browser.new_context(viewport={"width": 1440, "height": 1100})
        context.add_init_script('localStorage.setItem("naparnik.login", "intern.sokolov")')
        errors = []

        def open_page():
            page = context.new_page()
            page.on("pageerror", lambda e: errors.append(str(e)))
            page.goto(os.environ.get("APP_URL", "http://127.0.0.1:5173"))
            page.get_by_role("button", name="Открыть чат с Напарником", exact=True).click()
            return page

        def ask(page):
            page.get_by_role("textbox", name="Сообщение Напарнику", exact=True).fill(QUESTION)
            page.get_by_role("button", name="Отправить", exact=True).click()
            expect(page.get_by_role("button", name="Изменить вопрос", exact=True)).to_be_enabled()

        def edit(page, text):
            page.get_by_role("button", name="Изменить вопрос", exact=True).click()
            expect(page.get_by_role("button", name="Подтвердить", exact=True)).to_be_disabled()
            page.get_by_role("textbox", name="Текст вопроса", exact=True).fill(text)
            page.get_by_role("checkbox", name="Срочно — без ответа не могу продолжить работу", exact=True).check()
            page.get_by_role("button", name="Сохранить изменения", exact=True).click()
            expect(page.get_by_role("button", name="Подтвердить", exact=True)).to_be_enabled()

        try:
            page = open_page()
            ask(page)
            draft_id = api("/api/drafts")[1]["drafts"][0]["id"]
            edit(page, EDITED)
            saved = api("/api/drafts/" + draft_id)[1]["draft"]
            assert saved["version"] == 2 and saved["args"]["urgent"]
            assert len(api("/api/questions")[1]["questions"]) == initial_questions
            page.close()
            page = open_page()
            expect(page.get_by_text(EDITED, exact=True)).to_be_visible()
            old_tab = open_page()
            expect(old_tab.get_by_role("button", name="Подтвердить", exact=True)).to_be_enabled()
            edit(page, LATEST)
            old_tab.get_by_role("button", name="Подтвердить", exact=True).click()
            expect(old_tab.get_by_role("alert")).to_contain_text("Черновик изменился")
            expect(old_tab.get_by_text(LATEST, exact=True)).to_be_visible()
            assert len(api("/api/questions")[1]["questions"]) == initial_questions
            old_tab.get_by_role("button", name="Подтвердить", exact=True).click()
            expect(old_tab.get_by_text("✓ Готово:", exact=False)).to_be_visible()
            assert len(api("/api/questions")[1]["questions"]) == initial_questions + 1
            old_tab.close()
            # Первая вкладка ещё считает черновик ожидающим: отмена не должна скрыть уже отправленное.
            page.get_by_role("button", name="Отмена", exact=True).click()
            expect(page.get_by_text("✓ Готово:", exact=False)).to_be_visible()
            ask(page)
            second = api("/api/drafts")[1]["drafts"][0]
            page.get_by_role("button", name="Отмена", exact=True).click()
            expect(page.get_by_text("Отменено — ничего не отправлено.", exact=True)).to_be_visible()
            assert api("/api/drafts/" + second["id"] + "/confirm", {"version": 1})[0] == 409
            page.close()
            page = open_page()
            expect(page.get_by_role("button", name="Подтвердить", exact=True)).to_have_count(0)
            assert not api("/api/drafts")[1]["drafts"]
            ask(page)
            page.set_viewport_size({"width": 375, "height": 812})
            page.get_by_role("button", name="Изменить вопрос", exact=True).click()
            page.wait_for_function("document.documentElement.scrollWidth <= innerWidth")
            assert not errors, errors
            print("PASS: правка и срочность → восстановление → устаревшая вкладка → подтверждение → отмена; 375 px; без ошибок JS")
        finally:
            browser.close()


if __name__ == "__main__":
    main()
