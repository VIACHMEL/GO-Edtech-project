"""Страницы прототипа: выбор демонстрационных данных и передача в шаблоны."""

from urllib.parse import urlencode

from django.contrib.staticfiles import finders
from django.http import Http404
from django.shortcuts import render
from django.views.decorators.http import require_GET

import backend_client as backend


def teacher_context(title, active):
    return {"profile": backend.get_teacher_profile(), "page_title": title, "active_page": active}


def student_context(title, active):
    return {
        "profile": backend.get_student_profile(), "page_title": title, "active_page": active,
        "workspace_role": "Студент", "workspace_label": "Кабинет студента", "workspace_home": "/student/",
    }


def find_record(records, record_id):
    for record in records:
        if record["id"] == record_id:
            return record
    raise Http404("Запись не найдена в демонстрационных данных")



@require_GET
def index(request):
    return render(request, "index.html")


@require_GET
def register(request):
    return render(request, "register.html")


@require_GET
def teacher_reviews(request):
    query = request.GET.get("q", "").strip()
    data = backend.get_teacher_reviews(query)
    selected_id = request.GET.get("review", "")
    selected = None
    if selected_id:
        selected = find_record(data["reviews"], selected_id)
    elif data["reviews"]:
        selected = data["reviews"][0]
    for review in data["reviews"]:
        review["url"] = "?" + urlencode({"q": query, "review": review["id"]})
    context = teacher_context("Проверка работ", "reviews")
    context.update({"reviews": data["reviews"], "selected_review": selected, "query": query, "total": data["total"]})
    return render(request, "teacher_reviews.html", context)


@require_GET
def teacher_builder(request):
    context = teacher_context("Конструктор", "builder")
    context["builder"] = backend.get_teacher_builder()
    return render(request, "teacher_builder.html", context)


def calendar_context(request, audience):
    calendar = backend.get_demo_calendar(audience)
    mode = request.GET.get("view", "week")
    if mode not in ["day", "week", "month"]:
        mode = "week"
    day = request.GET.get("day", calendar["today"])
    selected_day = None
    for item in calendar["days"]:
        if item["id"] == day:
            selected_day = item
    if selected_day is None:
        raise Http404("Дата вне демонстрационного месяца")
    for event in calendar["events"]:
        event["url"] = "?" + urlencode({"view": mode, "day": day, "event": event["id"]})
    selected_event = None
    if request.GET.get("event"):
        selected_event = find_record(calendar["events"], request.GET["event"])
    day_events = []
    for event in calendar["events"]:
        if event["date"] == day:
            day_events.append(event)
    modes = []
    for key, label in [("day", "День"), ("week", "Неделя"), ("month", "Месяц")]:
        modes.append({"label": label, "active": mode == key, "url": "?" + urlencode({"view": key, "day": day})})
    return {
        "calendar": calendar, "calendar_mode": mode, "selected_day": selected_day,
        "day_events": day_events, "selected_event": selected_event, "calendar_modes": modes,
        "today_url": "?" + urlencode({"view": "day", "day": calendar["today"]}),
        "calendar_close_url": "?" + urlencode({"view": mode, "day": day}),
        "calendar_audience": audience,
    }


@require_GET
def teacher_calendar(request):
    context = teacher_context("Календарь + CRM", "calendar")
    context.update(calendar_context(request, "teacher"))
    return render(request, "teacher_calendar.html", context)


@require_GET
def student(request):
    context = student_context("Обзор", "overview")
    context["overview"] = backend.get_student_overview()
    return render(request, "student.html", context)


@require_GET
def student_calendar(request):
    context = student_context("Календарь", "calendar")
    context.update(calendar_context(request, "student"))
    return render(request, "student_calendar.html", context)


@require_GET
def student_assignments(request):
    query = request.GET.get("q", "").strip()
    status = request.GET.get("status", "all")
    if status not in ["all", "active", "completed"]:
        status = "all"
    assignments = backend.get_student_assignments(query, status)
    selected = None
    if request.GET.get("assignment"):
        selected = find_record(assignments, request.GET["assignment"])
    for assignment in assignments:
        assignment["url"] = "?" + urlencode({"q": query, "status": status, "assignment": assignment["id"]})
    tabs = []
    for key, label in [("all", "Все"), ("active", "Активные"), ("completed", "Завершённые")]:
        tabs.append({"label": label, "active": key == status, "url": "?" + urlencode({"q": query, "status": key})})
    context = student_context("Задания и варианты", "assignments")
    context.update({
        "assignments": assignments, "selected_assignment": selected, "query": query,
        "status": status, "tabs": tabs, "close_url": "?" + urlencode({"q": query, "status": status}),
    })
    return render(request, "student_assignments.html", context)


@require_GET
def student_comments(request):
    query = request.GET.get("q", "").strip()
    kind = request.GET.get("kind", "all")
    if kind not in ["all", "homework", "variant"]:
        kind = "all"
    context = student_context("Комментарии", "comments")
    context.update({"comments": backend.get_student_comments(query, kind), "query": query, "kind": kind})
    return render(request, "student_comments.html", context)


@require_GET
def student_statistics(request):
    period = request.GET.get("period", "30")
    if period not in ["30", "90"]:
        period = "30"
    context = student_context("Статистика", "statistics")
    context.update({"statistics": backend.get_student_statistics(period), "period": period})
    return render(request, "student_statistics.html", context)


@require_GET
def solve(request):
    variant = backend.get_demo_variant(request.GET.get("variant", "w1"))
    if variant is None:
        raise Http404("Вариант не найден")
    board_assets_ready = bool(finders.find("solve-board/dist/solve-board.js"))
    return render(request, "solve.html", {
        "variant": variant,
        "board_assets_ready": board_assets_ready,
    })
