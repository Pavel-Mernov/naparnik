#!/usr/bin/env python3
"""SC-30/31: отдельный backend с заглушкой и свежими данными, Vite или Docker frontend.

APP_URL=http://localhost:5173 (по умолчанию), CHROMIUM_PATH — необязательный путь Chromium.
Тест создаёт учебные результаты. Не запускайте на общей демо-БД: нужна отдельная тестовая среда.
"""
import os
from playwright.sync_api import sync_playwright, expect

ANSWER = """WITH daily AS (
 SELECT event_date, sum(contacts) AS contacts, sum(views) AS views
 FROM sandbox.training_ads_mart
 WHERE event_date BETWEEN '2026-11-16' AND '2026-11-22' AND is_active = 1
 GROUP BY event_date
) SELECT event_date, 1.0 * contacts / nullIf(views, 0) AS conversion FROM daily;
Таблица sandbox.sokolov_ads_conv_20261124
График «Конверсия карточки в контакт по дням», папка «Стажёры / Черновики»"""


def main():
    with sync_playwright() as p:
        options = {"headless": True}
        if os.environ.get("CHROMIUM_PATH"):
            options["executable_path"] = os.environ["CHROMIUM_PATH"]
        browser = p.chromium.launch(**options)
        page = browser.new_page(viewport={"width": 1440, "height": 1100})
        errors = []
        page.on("pageerror", lambda e: errors.append(str(e)))

        def switch(name):
            page.get_by_role("button", name="Тестовая учётка — сменить роль", exact=True).click()
            page.get_by_role("menuitem").filter(has_text=name).click()

        try:
            page.goto(os.environ.get("APP_URL", "http://127.0.0.1:5173"))
            expect(page.get_by_role("button", name="Начать практику", exact=True)).to_be_visible()
            switch("Екатерина Белова")
            draft = page.get_by_role("textbox", name="Черновик учебного задания", exact=True)
            expect(draft).to_be_visible()
            draft.fill(draft.input_value() + " Правка бадди: проверь подпись графика.")
            page.get_by_role("button", name="Одобрить задание", exact=True).click()
            expect(page.get_by_text("Задание одобрено", exact=True)).to_be_visible()
            switch("Даниил Соколов")
            page.get_by_role("button", name="Начать практику", exact=True).click()
            expect(page.get_by_role("heading", name="Общие термины", exact=True)).to_be_visible()
            page.get_by_role("button", name="Дальше", exact=True).click()
            expect(page.get_by_role("heading", name="Термины и правила команды", exact=True)).to_be_visible()
            page.get_by_role("button", name="Дальше", exact=True).click()
            answer = page.get_by_role("textbox", name="Твоя попытка", exact=True)
            bad = ANSWER.replace("1.0 * ", "").replace("event_date BETWEEN '2026-11-16' AND '2026-11-22' AND ", "") + "\nPRIVATE-FIRST"
            answer.fill(bad)
            page.get_by_role("button", name="Проверить попытку", exact=True).click()
            expect(page.get_by_text("Есть что поправить", exact=False)).to_be_visible()
            answer.fill(ANSWER)
            page.get_by_role("button", name="Проверить попытку", exact=True).click()
            expect(page.get_by_text("Учебное задание пройдено", exact=False)).to_be_visible()
            switch("Екатерина Белова")
            expect(page.get_by_text("Лучшая попытка №2", exact=False)).to_be_visible()
            assert "PRIVATE-FIRST" not in page.locator("body").inner_text()
            switch("Даниил Соколов")
            page.get_by_role("button", name="К реальной задаче", exact=True).click()
            expect(page.get_by_role("heading", name="К реальной задаче", exact=True)).to_be_visible()
            page.reload()
            expect(page.get_by_role("heading", name="К реальной задаче", exact=True)).to_be_visible()
            page.set_viewport_size({"width": 375, "height": 812})
            assert page.evaluate("document.documentElement.scrollWidth <= innerWidth")
            assert not errors, errors
            print("PASS: одобрение → термины → ошибочная и успешная попытки → лучшая попытка → чек-лист; 375 px; без ошибок JS")
        finally:
            browser.close()


if __name__ == "__main__":
    main()
