import unittest

from playwright.sync_api import sync_playwright
from capture_and_parse import load_more, reveal_answers
from parse import TasksParser
from batch import next_section


def card(number, active=False, control=True):
    return f'''<div class="ExerciseStyles_exercise__item">
      <a href="/catalog/123/{number}">#{number}</a>
      <div class="ExerciseStyles_exercise__main__item">
        <div class="ExerciseStyles_exercise__text__item">Условие {number}</div>
      </div>
      {('<div class="ExerciseSupportItem_support__item ' + ('ExerciseSupportItem_support__active__item' if active else '') + '" onclick="openAnswer(this)">Ответ и решение</div>') if control else ''}
      {('<span class="AnswerOldBlock_shortAnswer__item">42</span><div class="exercise__solution-text">Решение: проверка</div>') if active else ''}
    </div>'''


class BrowserTest(unittest.TestCase):
    def test_next_section_and_end_of_catalog(self):
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel="msedge", headless=True)
            try:
                page = browser.new_page()
                page.route("https://3.shkolkovo.online/**", lambda route: route.fulfill(
                    body='<a href="/catalog/456">Следующий раздел</a>', content_type="text/html; charset=utf-8"))
                page.goto("https://3.shkolkovo.online/catalog/123")
                self.assertEqual(next_section(page), "https://3.shkolkovo.online/catalog/456")
                page.set_content("<p>Конец</p>")
                self.assertIsNone(next_section(page))
            finally:
                browser.close()

    def test_load_more(self):
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel="msedge", headless=True)
            try:
                page = browser.new_page()
                page.set_content(card(1) + '''<button onclick="this.insertAdjacentHTML('beforebegin', '<div class=ExerciseStyles_exercise__text__item>New</div>'); this.remove()">Загрузить ещё</button>''')
                result = load_more(page, delay_ms=0, timeout_ms=1000)
                self.assertEqual(page.locator('[class*="ExerciseStyles_exercise__text__"]').count(), 2)
                self.assertEqual(result, {"load_more_clicks": 1, "more_available": False})
            finally:
                browser.close()

    def test_open_once_parse_and_report_missing_control(self):
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel="msedge", headless=True)
            try:
                page = browser.new_page()
                page.set_content('''<script>
                function openAnswer(el) {
                  if (el.className.includes('ExerciseSupportItem_support__active__')) throw Error('closed');
                  el.className += ' ExerciseSupportItem_support__active__item';
                  el.insertAdjacentHTML('afterend', '<span class="AnswerOldBlock_shortAnswer__item">42</span><div class="exercise__solution-text">Решение: проверка</div>');
                }</script>''' + card(1) + card(2, active=True) + card(3, control=False))
                result = reveal_answers(page, delay_ms=0, timeout_ms=1000)
                parser = TasksParser("https://3.shkolkovo.online/catalog/123", 1, "ege")
                parser.feed(page.content())
                records, skipped = parser.records()
            finally:
                browser.close()
        self.assertEqual(result["cards"], 3)
        self.assertEqual(result["opened"], 2)
        self.assertEqual(result["unavailable"][0]["card"], 3)
        self.assertEqual(len(records), 2)
        self.assertEqual(records[0]["answer"], "42")
        self.assertEqual(records[0]["solution"], "проверка")
        self.assertEqual(len(skipped), 1)


if __name__ == "__main__":
    unittest.main()
