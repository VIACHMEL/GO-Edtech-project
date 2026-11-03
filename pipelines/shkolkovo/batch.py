"""Последовательный сбор разделов ЕГЭ в локальные HTML и JSON."""

import argparse
import json
import re
import time
from pathlib import Path
from urllib.parse import urljoin

from playwright.sync_api import sync_playwright, Error
from capture_and_parse import capture_page, check_url, QUESTION


def save_json(path, value):
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    temporary.replace(path)


def next_section(page):
    links = page.get_by_text("Следующий раздел", exact=True)
    if not links.count():
        return None
    link = links.first
    href = link.get_attribute("href")
    if not href:
        href = link.locator("xpath=ancestor::a[1]").get_attribute("href")
    if not href:
        return None
    url = urljoin(page.url, href)
    check_url(url)
    return url


def open_section(page, url, attempts):
    check_url(url)
    for attempt in range(attempts):
        try:
            page.goto(url, wait_until="domcontentloaded", timeout=60000)
            page.locator(QUESTION).first.wait_for(state="visible", timeout=30000)
            check_url(page.url)
            return
        except Error:
            if attempt + 1 == attempts:
                raise
            time.sleep(15 * (attempt + 1))


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("url", help="первый раздел профильной математики ЕГЭ")
    cli.add_argument("--output-dir", required=True, type=Path)
    cli.add_argument("--target", type=int, default=1000, help="число записей в этой выгрузке")
    cli.add_argument("--max-sections", type=int, default=50)
    cli.add_argument("--attempts", type=int, choices=(1, 2, 3), default=2)
    cli.add_argument("--delay", type=float, default=1.0)
    cli.add_argument("--browser", choices=("msedge", "chrome"), default="msedge")
    args = cli.parse_args()
    if args.target < 1 or args.max_sections < 1 or args.delay < 1:
        cli.error("target и max-sections должны быть положительными, delay — не менее 1")
    check_url(args.url)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    checkpoint = args.output_dir / "progress.json"
    state = json.loads(checkpoint.read_text(encoding="utf-8")) if checkpoint.exists() else {
        "start_url": args.url, "next_url": args.url, "sections": []}
    if state["start_url"] != args.url:
        cli.error("папка содержит выгрузку с другого начального адреса")
    records = {}
    for section in state["sections"]:
        for record in json.loads((args.output_dir / section["file"]).read_text(encoding="utf-8")):
            records[(record["source_url"], record["content_text"])] = record
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(channel=args.browser, headless=False)
        try:
            page = browser.new_page()
            while len(records) < args.target and state["next_url"] and len(state["sections"]) < args.max_sections:
                url = state["next_url"]
                print(f"Открываю {url}; собрано {len(records)}", flush=True)
                open_section(page, url, args.attempts)
                heading = page.locator('[class*="contentWithAside__exercise_title__"]').first.inner_text()
                match = re.search(r"№\s*(\d+)", heading)
                if not match or not 1 <= int(match.group(1)) <= 12:
                    raise ValueError("Не определён поддерживаемый номер краткого ответа: " + heading)
                stem = f"section-{len(state['sections']) + 1:03d}"
                html_path = args.output_dir / (stem + ".html")
                json_path = args.output_dir / (stem + ".json")
                # Дочитываем сохранённую страницу после сбоя перед записью checkpoint.
                if html_path.exists() and json_path.exists():
                    report = {"recovered": True}
                else:
                    if html_path.exists() or json_path.exists():
                        raise ValueError("Обнаружен неполный файл раздела; проверьте его перед продолжением")
                    report = capture_page(page, url, html_path, json_path, "ege", int(match.group(1)),
                                          False, round(args.delay * 1000), args.target-len(records), 5,
                                          already_loaded=True)
                for record in json.loads(json_path.read_text(encoding="utf-8")):
                    records[(record["source_url"], record["content_text"])] = record
                following = next_section(page)
                if following == url or following in {s["url"] for s in state["sections"]}:
                    raise ValueError("Обнаружен цикл ссылок разделов")
                state["sections"].append({"url": url, "file": json_path.name, "report": report})
                state["next_url"] = following
                save_json(checkpoint, state)
                save_json(args.output_dir / "tasks.json", list(records.values()))
                print(f"Сохранено записей: {len(records)}", flush=True)
                page.wait_for_timeout(round(args.delay * 1000))
        finally:
            browser.close()


if __name__ == "__main__":
    main()
