"""Сохранить загруженную страницу каталога для локального разбора."""

import argparse
import sys
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import Error, sync_playwright


def main():
    parser = argparse.ArgumentParser(description="Сохранение одной страницы каталога Школково")
    parser.add_argument("url", help="Адрес раздела или подтемы")
    parser.add_argument("--output", required=True, help="Путь для сохранения HTML")
    parser.add_argument("--browser", choices=("msedge", "chrome"), default="msedge")
    args = parser.parse_args()
    address = urlparse(args.url)
    if address.scheme != "https" or address.hostname != "3.shkolkovo.online":
        parser.error("разрешён только HTTPS-адрес 3.shkolkovo.online")

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel=args.browser, headless=False)
        try:
            page = browser.new_page()
            page.goto(args.url, wait_until="domcontentloaded", timeout=60000)
            print("В открытом браузере дождитесь загрузки страницы.")
            print("При необходимости выполните проверку сайта самостоятельно и откройте нужную подтему.")
            print("Раскройте ответы вручную. Затем вернитесь в этот терминал и нажмите Enter.")
            input()
            actual = urlparse(page.url)
            if actual.scheme != "https" or actual.hostname != "3.shkolkovo.online":
                raise ValueError("открыта страница не на 3.shkolkovo.online")
            cards = page.locator('[class*="ExerciseStyles_exercise__text__"]').count()
            if cards == 0:
                raise ValueError("задания не найдены; проверьте, что страница загрузилась и открыта нужная подтема")
            output = Path(args.output)
            output.parent.mkdir(parents=True, exist_ok=True)
            if output.exists():
                raise ValueError(f"файл уже существует: {output}")
            output.write_text(page.content(), encoding="utf-8")
            print(f"Сохранено карточек: {cards}")
            print(f"Текущий адрес: {page.url}")
            print(f"Файл: {output}")
        finally:
            browser.close()


if __name__ == "__main__":
    try:
        main()
    except (Error, OSError, ValueError) as exc:
        print(f"Ошибка: {exc}", file=sys.stderr)
        sys.exit(1)
