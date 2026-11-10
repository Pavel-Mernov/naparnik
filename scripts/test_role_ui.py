#!/usr/bin/env python3
"""SC-13/SC-21: роли React-интерфейса и ответы после смены учётки.

Нужны запущенные backend и Vite, Python Playwright и Chromium.
APP_URL — адрес React UI (по умолчанию http://127.0.0.1:5173).
CHROMIUM_PATH — путь к системному Chromium, если браузер Playwright не установлен.
Изменяющие запросы перехватываются: тест не меняет данные сервера.
"""

import asyncio
import json
import os

from playwright.async_api import async_playwright, expect


APP_URL = os.environ.get("APP_URL", "http://127.0.0.1:5173")
CHAT = "Спроси о команде или задачах…"


async def switch(page, name):
    await page.get_by_role("combobox", name="Учётка", exact=True).click()
    await page.get_by_role("option", name=name, exact=True).click()


async def heading(page, text):
    await expect(page.get_by_role("heading", name=text, exact=True)).to_be_visible()


async def new_page(browser):
    context = await browser.new_context(viewport={"width": 1600, "height": 1000})
    page = await context.new_page()
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    return context, page, errors


async def roles(browser):
    context, page, errors = await new_page(browser)
    reads = []
    page.on("request", lambda r: reads.append((r.headers.get("x-user"), r.url)))
    await page.goto(APP_URL)
    await heading(page, "Привет, Даниил")
    await page.get_by_role("tab", name="Мой план", exact=True).click()
    await expect(page.get_by_role("checkbox").first).to_be_visible()

    await switch(page, "Андрей Ковалёв")
    await heading(page, "Обзор команды")
    await expect(page.get_by_role("tab", name="Мой план", exact=True)).to_have_count(0)
    await expect(page.get_by_role("tab", name="Мой день", exact=True)).to_have_count(0)
    await expect(page.get_by_role("checkbox")).to_have_count(0)
    await heading(page, "Полина Егорова")

    await switch(page, "Екатерина Белова")
    await heading(page, "Мои стажёры")
    await page.get_by_role("tab", name="Планы стажёров", exact=True).click()
    await heading(page, "План: Даниил Соколов")
    await page.get_by_role("combobox", name="Стажёр", exact=True).click()
    await page.get_by_role("option", name="Полина Егорова", exact=True).click()
    await heading(page, "План: Полина Егорова")
    await expect(page.get_by_role("checkbox")).to_have_count(0)
    await expect(page.get_by_text("План пока не утверждён", exact=True)).to_be_visible()

    await switch(page, "Михаил Орлов")
    await heading(page, "Входящие вопросы")
    assert set(await page.get_by_role("tab").all_text_contents()) == {
        "Входящие", "Входящие вопросы", "База знаний"
    }
    await expect(page.get_by_role("checkbox")).to_have_count(0)
    for user, url in reads:
        if user in ("lead.kovalev", "expert.orlov"):
            assert "/api/today" not in url and "/api/plans/" not in url, (user, url)
    await switch(page, "Андрей Ковалёв")
    await heading(page, "Обзор команды")
    await page.set_viewport_size({"width": 375, "height": 812})
    await page.get_by_role("tab", name="Согласование", exact=True).click()
    await heading(page, "Согласование заявок")
    assert await page.evaluate("document.documentElement.scrollWidth <= innerWidth")
    assert not errors, errors
    await context.close()
    print("PASS: экраны и запросы четырёх ролей")


async def decisions(browser):
    context, page, errors = await new_page(browser)
    await context.add_init_script("localStorage.setItem('naparnik.login', 'lead.kovalev')")
    items = [{"id": "ABS-ui-test", "kind": "absence", "type": "day_off",
              "date": "2026-11-16", "employee_id": "u-sokolov",
              "approver_id": "u-kovalev", "status": "pending", "created_at": "2026-11-11"},
             {"id": "ACC-ui-test", "kind": "access", "title": "Redash",
              "employee_id": "u-sokolov", "approver_id": "u-kovalev",
              "status": "pending", "created_at": "2026-11-11"}]
    sent = []

    async def requests(route):
        await route.fulfill(json={"requests": items})

    async def decide(route):
        request = route.request
        body = request.post_data_json
        sent.append((request.headers["x-user"], body))
        item = next(i for i in items if i["id"] in request.url)
        item["status"] = "rejected" if body["decision"] == "reject" else "granted"
        await route.fulfill(json={"request": item})

    await page.route("**/api/requests", requests)
    await page.route("**/api/requests/*/decision", decide)
    await page.goto(APP_URL)
    await heading(page, "Обзор команды")
    await page.get_by_role("tab", name="Согласование", exact=True).click()
    await page.get_by_role("button", name="Выдать доступ", exact=True).click()
    await expect(page.get_by_text("Выдан", exact=True)).to_be_visible()
    await page.get_by_role("button", name="Отклонить", exact=True).click()
    dialog = page.get_by_role("dialog")
    await expect(dialog.get_by_role("button", name="Отклонить", exact=True)).to_be_disabled()
    await dialog.get_by_role("combobox", name="Причина отказа", exact=True).click()
    await page.get_by_role("option", name="Другое", exact=False).click()
    await expect(dialog.get_by_role("button", name="Отклонить", exact=True)).to_be_disabled()
    await dialog.get_by_role("textbox", name="Комментарий").fill(" Нужна другая дата ")
    await dialog.get_by_role("button", name="Отклонить", exact=True).click()
    await expect(page.get_by_text("Отклонено", exact=True)).to_be_visible()
    assert sent[0][0] == "lead.kovalev" and sent[0][1]["decision"] == "approve", sent
    assert sent[1] == ("lead.kovalev", {"decision": "reject", "reason": "OTHER", "comment": "Нужна другая дата"}), sent
    assert not errors, errors
    await context.close()
    print("PASS: согласование, отказ и обязательный комментарий")


async def stale_responses(browser, kind):
    context, page, errors = await new_page(browser)
    target = {"load": "/api/plans/u-sokolov", "chat": "/api/chat/messages",
              "mutation": "/api/plan-steps/"}[kind]
    # Имитируем транспорт, который не умеет отменять запросы: проверяем и защиту по поколению учётки.
    await context.add_init_script("""
        const nativeFetch = window.fetch.bind(window);
        window.fetch = (url, options = {}) => nativeFetch(url,
            String(url).includes(%s) ? {...options, signal: undefined} : options);
    """ % json.dumps(target))
    arrived, release, finished = asyncio.Event(), asyncio.Event(), asyncio.Event()

    async def delay(route):
        if arrived.is_set():
            await route.continue_()
            return
        payload = {"text": "STALE-PRIVATE-REPLY", "sources": [], "drafts": []}
        if kind == "load":
            response = await route.fetch()
            payload = await response.json()
        if kind == "mutation":
            assert route.request.headers["x-user"] == "intern.sokolov"
            assert route.request.post_data_json["intern_id"] == "u-sokolov"
            payload = {"ok": True}
        arrived.set()
        await release.wait()
        await route.fulfill(json=payload)
        finished.set()

    await page.route("**" + target + ("*" if kind == "mutation" else ""), delay)
    await page.goto(APP_URL)
    if kind != "load":
        await heading(page, "Привет, Даниил")
    if kind == "chat":
        await page.get_by_role("button", name="Спросить Напарника", exact=True).click()
        await page.get_by_placeholder(CHAT).fill("Личный вопрос старой учётки")
        await page.get_by_role("button", name="Отправить сообщение", exact=True).click()
        await page.get_by_role("button", name="Закрыть чат", exact=True).click()
    elif kind == "mutation":
        await page.get_by_role("checkbox").first.click()
    await asyncio.wait_for(arrived.wait(), timeout=10)
    await switch(page, "Андрей Ковалёв")
    await heading(page, "Обзор команды")
    if kind == "chat":
        # A → B → A: одного сравнения логина недостаточно.
        await switch(page, "Даниил Соколов")
        await heading(page, "Привет, Даниил")
        await page.get_by_role("button", name="Спросить Напарника", exact=True).click()
    release.set()
    await asyncio.wait_for(finished.wait(), timeout=10)
    await page.wait_for_timeout(250)
    if kind == "chat":
        await expect(page.get_by_text("STALE-PRIVATE-REPLY", exact=True)).to_have_count(0)
        await expect(page.get_by_text("Личный вопрос старой учётки", exact=True)).to_have_count(0)
    else:
        await heading(page, "Обзор команды")
        await expect(page.get_by_role("checkbox")).to_have_count(0)
        await expect(page.get_by_text("Шаг отмечен выполненным", exact=True)).to_have_count(0)
    assert not errors, errors
    await context.close()
    print("PASS: запоздавший ответ —", kind)


async def main():
    async with async_playwright() as p:
        options = {"headless": True}
        if os.environ.get("CHROMIUM_PATH"):
            options["executable_path"] = os.environ["CHROMIUM_PATH"]
        browser = await p.chromium.launch(**options)
        try:
            await roles(browser)
            await decisions(browser)
            for kind in ("load", "chat", "mutation"):
                await stale_responses(browser, kind)
        finally:
            await browser.close()


if __name__ == "__main__":
    asyncio.run(main())
