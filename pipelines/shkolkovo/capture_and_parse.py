"""Открыть заданные страницы Школково, сохранить HTML и извлечь задания."""

import argparse
import json
import re
import sys
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import Error, TimeoutError as PlaywrightTimeout, sync_playwright
from parse import TasksParser


QUESTION = '[class*="ExerciseStyles_exercise__text__"]'
ANSWER = '[class*="AnswerOldBlock_shortAnswer__"]'
SOLUTION = '.exercise__solution-text'
SUPPORT = '[class*="ExerciseSupportItem_support__"]'
CARD = 'xpath=ancestor::div[./div[contains(@class,"ExerciseStyles_exercise__main__")]][1]'


def load_more(page, delay_ms=1000, max_loads=5, limit=0, timeout_ms=15000):
    loads = 0
    load_error = ""
    while loads < max_loads:
        count = page.locator(QUESTION).count()
        if limit and count >= limit:
            break
        button = page.get_by_role("button", name=re.compile(r"^Загрузить ещ[её]", re.I))
        if not button.count() or not button.first.is_visible():
            break
        button.first.click(timeout=timeout_ms)
        try:
            page.wait_for_function("(n) => document.querySelectorAll('[class*=\"ExerciseStyles_exercise__text__\"]').length > n",
                                   arg=count, timeout=timeout_ms)
        except PlaywrightTimeout:
            load_error = "после кнопки «Загрузить ещё» новые карточки не появились"
            break
        loads += 1
        print(f"Подгружено карточек: {page.locator(QUESTION).count()}", flush=True)
        if delay_ms:
            page.wait_for_timeout(delay_ms)
    button = page.get_by_role("button", name=re.compile(r"^Загрузить ещ[её]", re.I))
    remaining = bool(button.count() and button.first.is_visible())
    result = {"load_more_clicks": loads, "more_available": remaining}
    if load_error:
        result["load_error"] = load_error
    return result


def check_url(url):
    parsed = urlparse(url)
    if parsed.scheme != "https" or parsed.hostname != "3.shkolkovo.online":
        raise ValueError("допускаются только HTTPS-страницы 3.shkolkovo.online")


def reveal_answers(page, delay_ms=1000, limit=0, timeout_ms=8000):
    questions = page.locator(QUESTION)
    total = questions.count()
    if not total:
        raise ValueError("карточки не найдены: проверьте доступ и выбранный раздел")
    count = min(total, limit) if limit else total
    opened, unavailable = 0, []
    for index in range(count):
        card = questions.nth(index).locator(CARD)
        if card.count() != 1:
            unavailable.append({"card": index + 1, "reason": "не найдена карточка"})
            continue
        control = card.locator(SUPPORT).filter(has_text=re.compile(r"^\s*Ответ и решение\s*$"))
        if control.count() != 1:
            unavailable.append({"card": index + 1, "reason": "кнопка ответа не найдена"})
            continue
        try:
            active = "ExerciseSupportItem_support__active__" in (control.get_attribute("class") or "")
            if not active:
                control.click(timeout=timeout_ms)
            # Развёрнутые задачи могут содержать решение без краткого ответа.
            card.locator(f"{ANSWER}, {SOLUTION}").first.wait_for(state="attached", timeout=timeout_ms)
            opened += 1
        except PlaywrightTimeout:
            unavailable.append({"card": index + 1, "reason": "ответ или решение не появились"})
        if delay_ms:
            page.wait_for_timeout(delay_ms)
        print(f"Карточка {index + 1}/{count}: раскрыто {opened}", flush=True)
    return {"cards": total, "attempted": count, "opened": opened, "unavailable": unavailable}


def capture_page(page, url, html_path, json_path, exam, number, manual, delay_ms, limit, max_loads, already_loaded=False):
    check_url(url)
    for path in (html_path, json_path):
        if path.exists():
            raise ValueError(f"файл уже существует: {path}")
    response = None if already_loaded else page.goto(url, wait_until="domcontentloaded", timeout=60000)
    if manual:
        print("Если сайт просит проверку или вход, выполните их самостоятельно.")
        print("Откройте нужную подтему, затем нажмите Enter в терминале.")
        input()
    check_url(page.url)
    try:
        page.locator(QUESTION).first.wait_for(state="visible", timeout=20000)
    except PlaywrightTimeout as exc:
        status = f" (первый ответ HTTP {response.status})" if response is not None else ""
        raise ValueError(f"страница не показала задания{status}; проверьте доступ или используйте --manual") from exc
    report = load_more(page, delay_ms, max_loads, limit)
    report.update(reveal_answers(page, delay_ms, limit))
    check_url(page.url)
    html = page.content()
    parser = TasksParser(page.url, number, exam)
    parser.feed(html)
    records, skipped = parser.records()
    heading = page.locator('h1').first.inner_text() if page.locator('h1').count() else page.title()
    for record in records:
        record['metadata']['section_url'] = url
        record['metadata']['section_title'] = heading
    with html_path.open("x", encoding="utf-8") as output:
        output.write(html)
    with json_path.open("x", encoding="utf-8") as output:
        json.dump(records, output, ensure_ascii=False, indent=2)
        output.write("\n")
    report.update({"url": page.url, "html": str(html_path), "json": str(json_path),
                   "parsed_cards": len(parser.cards), "accepted": len(records), "skipped": skipped})
    return report


def main():
    cli = argparse.ArgumentParser(description="Открыть указанные страницы Школково и разобрать задания")
    cli.add_argument("urls", nargs="+", help="адреса страниц одного номера экзамена")
    cli.add_argument("--output-dir", required=True, type=Path, help="пустая папка для HTML и JSON")
    cli.add_argument("--number", required=True, type=int, choices=range(1, 20))
    cli.add_argument("--exam", choices=("ege", "oge"), required=True)
    cli.add_argument("--browser", choices=("msedge", "chrome"), default="msedge")
    cli.add_argument("--manual", action="store_true", help="дождаться подтверждения после открытия каждой страницы")
    cli.add_argument("--delay", type=float, default=1.0, help="пауза между карточками, в секундах (не менее 1)")
    cli.add_argument("--limit", type=int, default=0, help="для проверки: сколько карточек раскрывать на странице; 0 — все")
    cli.add_argument("--max-loads", type=int, default=5, help="максимум нажатий «Загрузить ещё» на странице")
    args = cli.parse_args()
    if args.delay < 1 or args.limit < 0 or args.max_loads < 0:
        cli.error("пауза должна быть не менее 1 секунды; limit и max-loads не могут быть отрицательными")
    for url in args.urls:
        try:
            check_url(url)
        except ValueError as exc:
            cli.error(str(exc))
    args.output_dir.mkdir(parents=True, exist_ok=True)
    paths = [(args.output_dir / f"page-{i:03d}.html", args.output_dir / f"page-{i:03d}.json")
             for i in range(1, len(args.urls) + 1)]
    if any(p.exists() for pair in paths for p in pair):
        cli.error("файлы page-NNN.html/json уже существуют в папке; выберите другую")
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel=args.browser, headless=False)
        try:
            page = browser.new_page()
            for url, (html_path, json_path) in zip(args.urls, paths):
                print(f"Открываю: {url}", flush=True)
                result = capture_page(page, url, html_path, json_path, args.exam, args.number,
                                      args.manual, round(args.delay * 1000), args.limit, args.max_loads)
                print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
        finally:
            browser.close()


if __name__ == "__main__":
    try:
        main()
    except (Error, OSError, ValueError) as exc:
        print(f"Ошибка: {exc}", file=sys.stderr)
        sys.exit(1)
