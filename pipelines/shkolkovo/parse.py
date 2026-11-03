"""Extract tasks from a saved, rendered Shkolkovo OGE-math catalog page."""

import argparse
import json
import re
import sys
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urljoin, urlparse


class TasksParser(HTMLParser):
    def __init__(self, base_url, task_number, exam_type="oge"):
        super().__init__(convert_charrefs=True)
        self.base_url = base_url
        self.task_number = task_number
        self.exam_type = exam_type
        self.stack = []
        self.card = None
        self.cards = []
        self.question_depth = None
        self.answer_depth = None
        self.solution_depth = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        classes = attrs.get("class", "").split()
        if tag == "div" and any(c.startswith("ExerciseStyles_exercise__") for c in classes):
            if self.card is None and not any("__main" in c or "__text" in c for c in classes):
                self.card = {"depth": len(self.stack), "text": [], "images": [], "local_images": [], "answer": [], "url": "", "solution": [], "solution_images": [], "solution_local_images": []}
        if self.card is not None:
            href = urljoin(self.base_url, attrs.get("href", ""))
            parsed_href = urlparse(href)
            if tag == "a" and parsed_href.hostname == "3.shkolkovo.online" and re.fullmatch(r"/catalog/\d+/\d+/?", parsed_href.path):
                self.card["url"] = href
            if any(c.startswith("ExerciseStyles_exercise__text__") for c in classes):
                self.question_depth = len(self.stack)
            if any(c.startswith("AnswerOldBlock_shortAnswer__") for c in classes):
                self.answer_depth = len(self.stack)
            if "exercise__solution-text" in classes:
                self.solution_depth = len(self.stack)
            if self.question_depth is not None and tag == "img":
                src = attrs.get("src", "").strip()
                if src:
                    if urlparse(src).scheme in ("http", "https") or src.startswith("/"):
                        self.card["images"].append(urljoin(self.base_url, src))
                    else:
                        self.card["local_images"].append(src)
                alt = attrs.get("alt", "").strip()
                if alt:
                    self.card["text"].append(" " + alt + " ")
            if self.question_depth is not None and tag in ("p", "div", "br", "li"):
                self.card["text"].append(" ")
            if self.solution_depth is not None and tag == "img":
                src = attrs.get("src", "").strip()
                if src:
                    key = "solution_images" if urlparse(src).scheme in ("http", "https") or src.startswith("/") else "solution_local_images"
                    self.card[key].append(urljoin(self.base_url, src) if key == "solution_images" else src)
                alt = attrs.get("alt", "").strip()
                if alt:
                    self.card["solution"].append(" " + alt + " ")
            if self.solution_depth is not None and tag in ("p", "div", "br", "li"):
                self.card["solution"].append(" ")
        if tag not in {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}:
            self.stack.append(tag)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in {"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"}:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        if tag not in self.stack:
            return
        while self.stack:
            depth = len(self.stack) - 1
            closed = self.stack.pop()
            if self.question_depth == depth:
                self.question_depth = None
            if self.answer_depth == depth:
                self.answer_depth = None
            if self.solution_depth == depth:
                self.solution_depth = None
            if self.card is not None and self.card["depth"] == depth:
                self.cards.append(self.card)
                self.card = None
            if closed == tag:
                break

    def handle_data(self, text):
        if self.card is None or any(tag in ("style", "script") for tag in self.stack):
            return
        if self.question_depth is not None:
            self.card["text"].append(text)
        if self.answer_depth is not None:
            self.card["answer"].append(text)
        if self.solution_depth is not None:
            self.card["solution"].append(text)

    def records(self):
        valid = []
        skipped = []
        for card in self.cards:
            content = " ".join("".join(card["text"]).split())
            answer = " ".join("".join(card["answer"]).split())
            url = card["url"]
            if not url or not content or not answer:
                skipped.append({"source_url": url, "missing": [name for name, value in (("source_url", url), ("content_text", content), ("answer", answer)) if not value]})
                continue
            metadata = {"images": list(dict.fromkeys(card["images"]))} if card["images"] else {}
            if card["local_images"]:
                metadata["local_images"] = list(dict.fromkeys(card["local_images"]))
            if card["solution_images"]:
                metadata["solution_images"] = list(dict.fromkeys(card["solution_images"]))
            if card["solution_local_images"]:
                metadata["solution_local_images"] = list(dict.fromkeys(card["solution_local_images"]))
            solution = " ".join("".join(card["solution"]).split())
            if solution.startswith("Решение:"):
                solution = solution[len("Решение:"):].strip()
            valid.append({"source_url": url, "source_site": urlparse(url).hostname, "subject": "math", "exam_type": self.exam_type, "task_number": self.task_number, "task_type": "short_answer", "content_text": content, "answer": answer, "solution": solution, "metadata": metadata})
        return valid, skipped


def main():
    arg = argparse.ArgumentParser(description="Разбор сохранённой страницы каталога Школково ОГЭ")
    arg.add_argument("--html", required=True, help="HTML сохранённой после загрузки страницы")
    arg.add_argument("--url", required=True, help="URL этой страницы каталога")
    arg.add_argument("--number", required=True, type=int, choices=range(1, 20), help="Номер задания с кратким ответом (1–19)")
    arg.add_argument("--exam", choices=("ege", "oge"), default="oge", help="Тип экзамена")
    arg.add_argument("--output", required=True, help="Куда сохранить JSON с валидными заданиями")
    args = arg.parse_args()
    parsed = urlparse(args.url)
    if parsed.scheme != "https" or parsed.hostname != "3.shkolkovo.online":
        arg.error("URL должен принадлежать https://3.shkolkovo.online")
    parser = TasksParser(args.url, args.number, args.exam)
    parser.feed(Path(args.html).read_text(encoding="utf-8"))
    records, skipped = parser.records()
    if not parser.cards:
        sys.exit("Не найдены карточки заданий: проверьте загруженную разметку и доступ к странице")
    Path(args.output).write_text(json.dumps(records, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"cards": len(parser.cards), "accepted": len(records), "skipped": skipped}, ensure_ascii=False), file=sys.stderr)


if __name__ == "__main__":
    main()
