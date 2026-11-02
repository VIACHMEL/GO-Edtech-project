import unittest

from parse import TasksParser


class ParseTests(unittest.TestCase):
    def test_saved_page_absolute_link_and_local_image(self):
        parser = TasksParser("https://3.shkolkovo.online/catalog/123", 1, "ege")
        parser.feed('''<div class="ExerciseStyles_exercise__abc"><a href="https://3.shkolkovo.online/catalog/123/456">#456</a><div class="ExerciseStyles_exercise__text__abc">Пример <img alt="x" src="page_files/image.svg"></div><span class="AnswerOldBlock_shortAnswer__abc">0</span></div>''')
        tasks, skipped = parser.records()
        self.assertEqual(skipped, [])
        self.assertEqual(tasks[0]["exam_type"], "ege")
        self.assertEqual(tasks[0]["answer"], "0")
        self.assertEqual(tasks[0]["metadata"]["local_images"], ["page_files/image.svg"])
        self.assertNotIn("images", tasks[0]["metadata"])

    def test_extracts_only_complete_tasks(self):
        html = '''<div class="ExerciseStyles_exercise__abc">
        <a href="/catalog/123/456">#456</a>
        <div class="ExerciseStyles_exercise__text__abc"><p>Найди <img alt="x + 1" src="/api/a.svg">.</p></div>
        <span class="AnswerOldBlock_shortAnswer__abc"> 42 </span></div>
        <div class="ExerciseStyles_exercise__abc"><a href="/catalog/123/457">#457</a>
        <div class="ExerciseStyles_exercise__text__abc">Без ответа</div></div>'''
        parser = TasksParser("https://3.shkolkovo.online/catalog/123", 11)
        parser.feed(html)
        tasks, skipped = parser.records()
        self.assertEqual(len(tasks), 1)
        self.assertEqual(tasks[0]["source_url"], "https://3.shkolkovo.online/catalog/123/456")
        self.assertEqual(tasks[0]["answer"], "42")
        self.assertIn("x + 1", tasks[0]["content_text"])
        self.assertEqual(tasks[0]["metadata"]["images"], ["https://3.shkolkovo.online/api/a.svg"])
        self.assertEqual(skipped[0]["missing"], ["answer"])


if __name__ == "__main__":
    unittest.main()
