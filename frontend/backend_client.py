"""Обращения к Go и демонстрационные ответы для оформления страниц."""

import json
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener, urlopen

from django.conf import settings


def get_json(path):
    if not path.startswith("/") or path.startswith("//"):
        raise ValueError("Путь должен начинаться с одного /, например /example/")

    url = settings.BACKEND_BASE_URL.rstrip("/") + path
    request = Request(
        url,
        method="GET",
        headers={"Accept": "application/json"},
    )

    with urlopen(request, timeout=settings.BACKEND_TIMEOUT) as response:
        body = response.read()

    text = body.decode("utf-8")
    data = json.loads(text)
    return data


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def get_solve_attempt(attempt_id, cookie_header):
    url = f"{settings.BACKEND_BASE_URL.rstrip('/')}/api/attempts/{attempt_id}/solve"
    request = Request(url, headers={"Accept": "application/json", "Cookie": cookie_header})
    opener = build_opener(ProxyHandler({}), NoRedirect())
    with opener.open(request, timeout=settings.BACKEND_TIMEOUT) as response:
        return json.load(response)


def get_teacher_overview():
    """Вымышленный ответ для главной преподавателя. Запрос к Go не выполняется."""
    return {
        "teacher": get_teacher_profile(),
        "date": "Сегодня, 3 ноября",
        "date_detail": "Вторник · 2026",
        "next_lesson": {
            "time": "18:00",
            "end_time": "19:00",
            "subject": "Математика",
            "student": "Иван Петров",
            "topic": "Производные и их применение",
            "format": "Индивидуальное занятие",
            "url": "/teacher/calendar/",
        },
        "lessons": [
            {
                "time": "16:00",
                "end_time": "17:00",
                "student": "Анна Волкова",
                "subject": "Математика",
                "topic": "Квадратные уравнения",
                "status": "Завершено",
                "is_next": False,
                "is_finished": True,
                "url": "/teacher/calendar/",
            },
            {
                "time": "18:00",
                "end_time": "19:00",
                "student": "Иван Петров",
                "subject": "Математика",
                "topic": "Производные и их применение",
                "status": "Следующее",
                "is_next": True,
                "is_finished": False,
                "url": "/teacher/calendar/",
            },
            {
                "time": "20:00",
                "end_time": "21:30",
                "student": "Группа «ЕГЭ · 11 класс»",
                "subject": "Математика",
                "topic": "Задачи с параметром",
                "status": "Запланировано",
                "is_next": False,
                "is_finished": False,
                "url": "/teacher/calendar/",
            },
        ],
        "attention": [
            {
                "count": 4,
                "title": "Работы ждут проверки",
                "description": "Ответы и решения учеников",
                "url": "/teacher/reviews/",
            },
            {
                "count": 2,
                "title": "Ученики с просроченными заданиями",
                "description": "Стоит обсудить на следующем занятии",
                "url": "/teacher/students/?status=attention",
            },
        ],
    }


def get_teacher_profile():
    return {"name": "Александр Морозов", "initials": "АМ", "subject": "Математика"}


def get_demo_students():
    """Каждый вызов возвращает новые вымышленные записи, ничего не сохраняется."""
    return [
        {
            "id": "s1", "name": "Иван Петров", "initials": "ИП",
            "subject": "Математика", "group": "ЕГЭ · 11 класс",
            "email": "ivan@example.invalid", "last_lesson": "2 ноября",
            "next_lesson": "Сегодня, 18:00", "progress": 74,
            "state": "active", "status_label": "Активен", "needs_attention": False,
            "completed": 8, "overdue": 0,
            "notes": "Отрабатываем применение производной. Хорошо справляется с короткими ответами.",
            "assignments": [
                {"title": "Домашняя работа №7", "deadline": "До 5 ноября", "status": "В процессе"},
                {"title": "Пробный вариант №3", "deadline": "1 ноября", "status": "Проверено"},
            ],
        },
        {
            "id": "s2", "name": "Анна Волкова", "initials": "АВ",
            "subject": "Математика", "group": "ОГЭ · 9 класс",
            "email": "anna@example.invalid", "last_lesson": "Сегодня, 16:00",
            "next_lesson": "Завтра, 17:00", "progress": 62,
            "state": "active", "status_label": "Требует внимания", "needs_attention": True,
            "completed": 6, "overdue": 1,
            "notes": "Нужно вернуться к раскрытию скобок и проверить решение домашней работы.",
            "assignments": [
                {"title": "Квадратные уравнения", "deadline": "До 2 ноября", "status": "Просрочено"},
                {"title": "Домашняя работа №6", "deadline": "30 октября", "status": "Проверено"},
            ],
        },
        {
            "id": "s3", "name": "Максим Соколов", "initials": "МС",
            "subject": "Математика", "group": "ЕГЭ · 11 класс",
            "email": "maxim@example.invalid", "last_lesson": "1 ноября",
            "next_lesson": "Сегодня, 20:00", "progress": 58,
            "state": "active", "status_label": "Требует внимания", "needs_attention": True,
            "completed": 5, "overdue": 2,
            "notes": "На следующем занятии разобрать тригонометрические уравнения.",
            "assignments": [
                {"title": "Тригонометрия · практика", "deadline": "До 1 ноября", "status": "Просрочено"},
                {"title": "Пробный вариант №2", "deadline": "До 2 ноября", "status": "Просрочено"},
            ],
        },
        {
            "id": "s4", "name": "Мария Кузнецова", "initials": "МК",
            "subject": "Математика", "group": "ОГЭ · 9 класс",
            "email": "maria@example.invalid", "last_lesson": "2 ноября",
            "next_lesson": "Завтра, 17:00", "progress": 86,
            "state": "active", "status_label": "Активен", "needs_attention": False,
            "completed": 12, "overdue": 0,
            "notes": "Уверенно решает алгебру. Следующая тема — геометрические задачи.",
            "assignments": [
                {"title": "Площади фигур", "deadline": "До 6 ноября", "status": "В процессе"},
            ],
        },
        {
            "id": "s5", "name": "Даниил Орлов", "initials": "ДО",
            "subject": "Математика", "group": "ЕГЭ · 2025",
            "email": "daniil@example.invalid", "last_lesson": "28 октября",
            "next_lesson": "Не запланировано", "progress": 71,
            "state": "paused", "status_label": "На паузе", "needs_attention": False,
            "completed": 9, "overdue": 0,
            "notes": "Занятия приостановлены. Материалы прошлых работ доступны для просмотра.",
            "assignments": [],
        },
        {
            "id": "s6", "name": "Елизавета Смирнова", "initials": "ЕС",
            "subject": "Математика", "group": "ЕГЭ · 11 класс",
            "email": "eliza@example.invalid", "last_lesson": "2 ноября",
            "next_lesson": "Сегодня, 20:00", "progress": 91,
            "state": "active", "status_label": "Активен", "needs_attention": False,
            "completed": 14, "overdue": 0,
            "notes": "Готова к более сложным задачам с параметрами.",
            "assignments": [
                {"title": "Задачи с параметром", "deadline": "До 6 ноября", "status": "В процессе"},
            ],
        },
    ]


def get_teacher_students(query="", status="all"):
    """Имитирует ответ Go с поиском и фильтром только по примерам."""
    students = get_demo_students()
    result = []
    active_count = 0
    attention_count = 0
    for student in students:
        if student["state"] == "active":
            active_count += 1
        if student["needs_attention"]:
            attention_count += 1

        text = student["name"] + " " + student["subject"] + " " + student["group"]
        matches_query = query.casefold() in text.casefold()
        matches_status = (
            status == "all"
            or (status == "active" and student["state"] == "active")
            or (status == "attention" and student["needs_attention"])
        )
        if matches_query and matches_status:
            result.append(student)

    return {
        "teacher": get_teacher_profile(), "students": result,
        "total": len(students), "active_count": active_count,
        "attention_count": attention_count,
    }


def get_demo_groups():
    return [
        {
            "id": "g1", "name": "ЕГЭ · 11 класс", "subject": "Математика",
            "description": "Подготовка к профильному экзамену",
            "schedule": "Вт / Пт · 20:00", "next_lesson": "Сегодня, 20:00",
            "state": "active", "status_label": "Активна", "lesson_count": 14,
            "member_ids": ["s1", "s3", "s6"],
            "notes": "Работаем над заданиями второй части и оформлением развёрнутого ответа.",
            "assignments": [
                {"title": "Вариант №13", "deadline": "До 6 ноября", "status": "В процессе"},
                {"title": "Производные · практика", "deadline": "1 ноября", "status": "Проверено"},
            ],
        },
        {
            "id": "g2", "name": "ОГЭ · 9 класс", "subject": "Математика",
            "description": "Базовые навыки и уверенное решение первой части",
            "schedule": "Ср / Сб · 17:00", "next_lesson": "Завтра, 17:00",
            "state": "active", "status_label": "Активна", "lesson_count": 10,
            "member_ids": ["s2", "s4"],
            "notes": "Повторяем уравнения и геометрию. После занятия — короткая самостоятельная работа.",
            "assignments": [
                {"title": "Домашняя работа №7", "deadline": "До 7 ноября", "status": "В процессе"},
            ],
        },
        {
            "id": "g3", "name": "Геометрия · практика", "subject": "Математика",
            "description": "Дополнительный разбор геометрических задач",
            "schedule": "Чт · 16:00", "next_lesson": "5 ноября, 16:00",
            "state": "active", "status_label": "Активна", "lesson_count": 6,
            "member_ids": ["s4", "s6"],
            "notes": "Учимся строить вспомогательные линии и записывать доказательства.",
            "assignments": [
                {"title": "Окружности и касательные", "deadline": "До 8 ноября", "status": "В процессе"},
            ],
        },
        {
            "id": "g4", "name": "ЕГЭ · 2025", "subject": "Математика",
            "description": "Материалы завершённого курса",
            "schedule": "Курс завершён", "next_lesson": "Не запланировано",
            "state": "archived", "status_label": "Архив", "lesson_count": 32,
            "member_ids": ["s5"], "notes": "Архив группы. Новые занятия не назначены.",
            "assignments": [],
        },
    ]


def get_teacher_groups(query="", status="all"):
    groups = get_demo_groups()
    students = get_demo_students()
    result = []
    active_count = 0
    archived_count = 0
    for group in groups:
        group["members"] = []
        for student in students:
            if student["id"] in group["member_ids"]:
                group["members"].append(student)
        group["member_count"] = len(group["members"])
        if group["state"] == "active":
            active_count += 1
        if group["state"] == "archived":
            archived_count += 1

        text = group["name"] + " " + group["subject"] + " " + group["description"]
        matches_query = query.casefold() in text.casefold()
        matches_status = status == "all" or group["state"] == status
        if matches_query and matches_status:
            result.append(group)

    return {
        "teacher": get_teacher_profile(), "groups": result,
        "total": len(groups), "active_count": active_count,
        "archived_count": archived_count,
    }


def get_demo_tasks():
    """Банк примеров: здесь нет эталонных ответов, выдаваемых ученику."""
    return [
        {"id": "t1", "title": "Квадратное уравнение", "topic": "Алгебра", "kind": "Краткий ответ", "max_score": 2, "prompt": "Решите уравнение x² − 5x + 6 = 0. В ответе укажите меньший корень."},
        {"id": "t2", "title": "Производная функции", "topic": "Производные", "kind": "Краткий ответ", "max_score": 2, "prompt": "Найдите значение производной функции f(x) = x³ − 4x в точке x = 2."},
        {"id": "t3", "title": "Площадь треугольника", "topic": "Геометрия", "kind": "Развёрнутый ответ", "max_score": 3, "prompt": "Основание треугольника равно 12, высота к нему равна 7. Найдите площадь и запишите ход решения."},
        {"id": "t4", "title": "Вероятность события", "topic": "Вероятность", "kind": "Краткий ответ", "max_score": 2, "prompt": "В коробке 3 красных и 7 синих шаров. Найдите вероятность случайно достать красный шар."},
        {"id": "t5", "title": "Тригонометрическое уравнение", "topic": "Тригонометрия", "kind": "Развёрнутый ответ", "max_score": 3, "prompt": "Решите уравнение 2 sin x = 1 на отрезке [0; 2π]. Объясните выбор корней."},
        {"id": "t6", "title": "Арифметическая прогрессия", "topic": "Алгебра", "kind": "Краткий ответ", "max_score": 2, "prompt": "Первый член арифметической прогрессии равен 4, разность равна 3. Найдите десятый член."},
        {"id": "t7", "title": "Исследование функции", "topic": "Производные", "kind": "Развёрнутый ответ", "max_score": 4, "prompt": "Найдите промежутки возрастания и убывания функции f(x) = x³ − 3x. Обоснуйте ответ."},
        {"id": "t8", "title": "Задача с параметром", "topic": "Параметры", "kind": "Развёрнутый ответ", "max_score": 4, "prompt": "При каких значениях a уравнение x² − 2ax + 1 = 0 имеет два различных действительных корня?"},
    ]


def get_teacher_builder():
    return {
        "teacher": get_teacher_profile(), "tasks": get_demo_tasks(),
        "draft_title": "Контрольная №3", "selected_ids": ["t1", "t3"],
    }


def get_teacher_reviews(query=""):
    students = get_demo_students()
    tasks = get_demo_tasks()
    examples = [
        {"id": "r1", "student": students[0], "title": "Домашняя работа №5", "submitted": "Сегодня, 14:32", "answer": "S = 12 · 7 / 2 = 42. Ответ: 42.", "expected_answer": "42. Площадь равна половине произведения основания на высоту.", "task": tasks[2], "score_presets": [{"label": "Засчитать", "value": 3}, {"label": "Частично", "value": 1}, {"label": "Не засчитать", "value": 0}]},
        {"id": "r2", "student": students[1], "title": "Квадратные уравнения", "submitted": "Сегодня, 12:10", "answer": "Корни: 2 и 3. Меньший корень — 2.", "expected_answer": "2", "task": tasks[0], "score_presets": [{"label": "Засчитать", "value": 2}, {"label": "Частично", "value": 1}, {"label": "Не засчитать", "value": 0}]},
        {"id": "r3", "student": students[2], "title": "Практика по производным", "submitted": "Вчера, 20:14", "answer": "f′(x) = 3x² − 4. При x = 2 получаем 8.", "expected_answer": "8", "task": tasks[1], "score_presets": [{"label": "Засчитать", "value": 2}, {"label": "Частично", "value": 1}, {"label": "Не засчитать", "value": 0}]},
        {"id": "r4", "student": students[3], "title": "Геометрия · практика", "submitted": "Вчера, 18:05", "answer": "Площадь равна 12 · 7 = 84.", "expected_answer": "42. В решении нужно разделить произведение на 2.", "task": tasks[2], "score_presets": [{"label": "Засчитать", "value": 3}, {"label": "Частично", "value": 1}, {"label": "Не засчитать", "value": 0}]},
    ]
    result = []
    for review in examples:
        text = review["student"]["name"] + " " + review["title"]
        if query.casefold() in text.casefold():
            result.append(review)
    return {"teacher": get_teacher_profile(), "reviews": result, "total": len(examples)}


def get_demo_calendar(audience="teacher"):
    """Одна демонстрационная неделя; построение сетки — только отображение."""
    from calendar import monthcalendar

    events = [
        {"id": "c1", "date": "2026-11-03", "time": "16:00", "end_time": "17:00", "title": "Анна Волкова", "topic": "Квадратные уравнения", "kind": "lesson", "kind_label": "Занятие", "subject": "Математика", "participants": ["s2"], "payment": "Оплачено", "details_url": "/teacher/students/?student=s2"},
        {"id": "c2", "date": "2026-11-03", "time": "18:00", "end_time": "19:00", "title": "Иван Петров", "topic": "Производные и их применение", "kind": "lesson", "kind_label": "Занятие", "subject": "Математика", "participants": ["s1"], "payment": "Оплачено", "details_url": "/teacher/students/?student=s1"},
        {"id": "c3", "date": "2026-11-03", "time": "20:00", "end_time": "21:30", "title": "ЕГЭ · 11 класс", "topic": "Задачи с параметром", "kind": "lesson", "kind_label": "Групповое занятие", "subject": "Математика", "participants": ["s1", "s3", "s6"], "payment": "По абонементу", "details_url": "/teacher/groups/?group=g1"},
        {"id": "c4", "date": "2026-11-04", "time": "17:00", "end_time": "18:00", "title": "Максим Соколов", "topic": "Тригонометрия", "kind": "lesson", "kind_label": "Занятие", "subject": "Математика", "participants": ["s3"], "payment": "Ожидает оплаты", "details_url": "/teacher/students/?student=s3"},
        {"id": "c5", "date": "2026-11-05", "time": "18:00", "end_time": "19:00", "title": "Иван Петров", "topic": "Практика по производным", "kind": "lesson", "kind_label": "Занятие", "subject": "Математика", "participants": ["s1"], "payment": "Оплачено", "details_url": "/teacher/students/?student=s1"},
        {"id": "c6", "date": "2026-11-06", "time": "20:00", "end_time": "", "title": "Домашняя работа №7", "topic": "Срок выполнения", "kind": "deadline", "kind_label": "Срок сдачи", "subject": "Математика", "participants": ["s1"], "payment": "", "details_url": "/student/assignments/"},
        {"id": "c7", "date": "2026-11-07", "time": "17:00", "end_time": "18:00", "title": "ОГЭ · 9 класс", "topic": "Геометрические задачи", "kind": "lesson", "kind_label": "Групповое занятие", "subject": "Математика", "participants": ["s2", "s4"], "payment": "По абонементу", "details_url": "/teacher/groups/?group=g2"},
    ]
    visible_events = []
    for event in events:
        if audience == "teacher" or "s1" in event["participants"]:
            if audience == "student":
                event["details_url"] = "/student/assignments/"
            visible_events.append(event)

    week = []
    labels = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"]
    for index, number in enumerate(range(2, 9)):
        day_id = f"2026-11-{number:02d}"
        day_events = []
        for event in visible_events:
            if event["date"] == day_id:
                day_events.append(event)
        week.append({"id": day_id, "number": number, "label": labels[index], "events": day_events})

    month_weeks = []
    for row in monthcalendar(2026, 11):
        cells = []
        for number in row:
            day_id = f"2026-11-{number:02d}" if number else ""
            day_events = []
            for event in visible_events:
                if event["date"] == day_id:
                    day_events.append(event)
            cells.append({"id": day_id, "number": number, "events": day_events})
        month_weeks.append(cells)
    return {
        "month": "Ноябрь 2026", "week_label": "2–8 ноября 2026", "today": "2026-11-03",
        "weekdays": labels, "week": week, "month_weeks": month_weeks, "events": visible_events,
        "days": [{"id": f"2026-11-{number:02d}", "label": f"{number} ноября"} for number in range(1, 31)],
    }


def get_student_profile():
    student = get_demo_students()[0]
    return {"name": student["name"], "initials": student["initials"], "subject": student["subject"]}


def get_demo_assignments():
    return [
        {"id": "a1", "title": "Домашняя работа №7", "subject": "Математика", "topic": "Алгебра и геометрия", "task_count": 3, "completed": 1, "duration": "Без ограничения времени", "deadline": "До 6 ноября, 20:00", "state": "active", "status": "В процессе", "variant": "w1", "score": "", "comment": ""},
        {"id": "a2", "title": "Пробный вариант №4", "subject": "Математика", "topic": "Подготовка к экзамену", "task_count": 6, "completed": 0, "duration": "90 минут", "deadline": "До 8 ноября, 18:00", "state": "active", "status": "Не начато", "variant": "w2", "score": "", "comment": ""},
        {"id": "a3", "title": "Пробный вариант №3", "subject": "Математика", "topic": "Алгебра и производные", "task_count": 6, "completed": 6, "duration": "90 минут", "deadline": "1 ноября", "state": "completed", "status": "Проверено", "variant": "", "score": "78 / 100", "comment": "Хороший результат. Повторите тригонометрию и внимательно проверяйте знаки."},
        {"id": "a4", "title": "Домашняя работа №6", "subject": "Математика", "topic": "Функции и графики", "task_count": 3, "completed": 3, "duration": "Без ограничения времени", "deadline": "30 октября", "state": "completed", "status": "Проверяется", "variant": "", "score": "", "comment": "Результат появится после подтверждения преподавателем."},
        {"id": "a5", "title": "Домашняя работа №5", "subject": "Математика", "topic": "Уравнения", "task_count": 3, "completed": 3, "duration": "Без ограничения времени", "deadline": "28 октября", "state": "completed", "status": "Проверено", "variant": "", "score": "9 / 10", "comment": "Метод выбран правильно. Проверяйте область определения."},
    ]


def get_student_assignments(query="", status="all"):
    assignments = get_demo_assignments()
    result = []
    for assignment in assignments:
        text = assignment["title"] + " " + assignment["topic"]
        if query.casefold() in text.casefold() and (status == "all" or assignment["state"] == status):
            result.append(assignment)
    return result


def get_demo_comments():
    return [
        {"id": "f1", "date": "Сегодня", "teacher": "Александр Морозов", "work": "Пробный вариант №3", "kind": "variant", "assignment": "a3", "text": "В третьем задании верно выбран метод, но потерян знак после раскрытия скобок. Проверьте этот переход ещё раз."},
        {"id": "f2", "date": "1 ноября", "teacher": "Александр Морозов", "work": "Пробный вариант №3", "kind": "variant", "assignment": "a3", "text": "Хороший результат. Уверенно решаете задания по производным. Стоит повторить тригонометрию."},
        {"id": "f3", "date": "30 октября", "teacher": "Александр Морозов", "work": "Домашняя работа №5", "kind": "homework", "assignment": "a5", "text": "Оформление стало понятнее. Записывайте область определения перед преобразованием выражений."},
    ]


def get_student_overview():
    active = []
    for assignment in get_demo_assignments():
        if assignment["state"] == "active":
            active.append(assignment)
    return {
        "profile": get_student_profile(), "date": "Сегодня, 3 ноября", "date_detail": "Вторник · 2026",
        "next_lesson": {"time": "18:00", "subject": "Математика", "topic": "Производные и их применение", "teacher": "Александр Морозов"},
        "assignments": active, "last_comment": get_demo_comments()[0],
    }


def get_student_statistics(period="30"):
    scores = [70, 74, 78, 82] if period == "30" else [62, 66, 70, 74, 78, 82]
    points = []
    for index, score in enumerate(scores):
        x = 40 + index * 360 / (len(scores) - 1)
        y = 160 - score * 1.3
        points.append({"x": round(x, 1), "y": round(y, 1), "score": score, "label": f"Вариант {index + 1}"})
    line = " ".join(f"{point['x']},{point['y']}" for point in points)
    return {
        "subject": "Математика", "period_label": "Последние 30 дней" if period == "30" else "Последние 90 дней",
        "points": points, "line": line,
        "ticks": [{"y": 30, "label": "100"}, {"y": 95, "label": "50"}, {"y": 160, "label": "0"}],
        "topics": [
            {"name": "Производные", "result": 84, "note": "Сильная тема"},
            {"name": "Квадратные уравнения", "result": 91, "note": "Сильная тема"},
            {"name": "Тригонометрия", "result": 62, "note": "Стоит повторить"},
            {"name": "Геометрия", "result": 71, "note": "Стоит повторить"},
        ],
    }


def get_demo_variant(variant_id="w1"):
    examples = {
        "w1": {"id": "w1", "title": "Домашняя работа №7", "duration": "Без ограничения времени", "task_ids": ["t1", "t2", "t3"]},
        "w2": {"id": "w2", "title": "Пробный вариант №4", "duration": "90 минут", "task_ids": ["t1", "t2", "t3", "t4", "t5", "t6"]},
    }
    variant = examples.get(variant_id)
    if variant is None:
        return None
    variant["tasks"] = []
    for task_id in variant["task_ids"]:
        for task in get_demo_tasks():
            if task["id"] == task_id:
                variant["tasks"].append(task)
    return variant


def get_student_comments(query="", kind="all"):
    result = []
    for comment in get_demo_comments():
        text = comment["text"] + " " + comment["work"] + " " + comment["teacher"]
        if query.casefold() in text.casefold() and (kind == "all" or comment["kind"] == kind):
            result.append(comment)
    return result
