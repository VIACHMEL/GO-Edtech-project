"""Страницы прототипа: выбор демонстрационных данных и передача в шаблоны."""

from urllib.parse import urlencode

from django.conf import settings
from django.http import Http404
from django.middleware.csrf import rotate_token
from django.shortcuts import redirect, render
from django.views.decorators.http import require_GET, require_POST, require_http_methods

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
    token = request.COOKIES.get("go_session", "")
    filters = {name: request.GET.get(name, "").strip()
               for name in ("subject", "year", "task_number", "task_type", "limit")}
    try:
        profile = backend.get_json("/api/v1/me", token)
        builder = backend.get_teacher_builder(filters, token)
    except backend.BackendError as error:
        return backend_error(request, error)
    builder["user_id"] = profile["id"]
    return render(request, "teacher_builder.html", {
        "profile": profile, "live": True, "page_title": "Конструктор", "active_page": "builder",
        "builder": builder, "filters": filters,
    })


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
    return render(request, "solve.html", {"variant": variant})


def backend_error(request, error):
    if error.status == 401:
        response = redirect("/login/")
        response.delete_cookie("go_session")
        return response
    return render(request, "login.html", {"error": error.message}, status=error.status)


@require_http_methods(["GET", "POST"])
def login(request):
    error, status = "", 200
    if request.method == "POST":
        try:
            data = backend.get_json("/api/v1/login", method="POST", data={
                "email": request.POST.get("email", ""),
                "password": request.POST.get("password", ""),
            })
            profile = backend.get_json("/api/v1/me", data["token"])
            rotate_token(request)
            response = redirect("/teacher/" if profile["role"] == "teacher" else "/student/")
            response.set_cookie("go_session", data["token"], max_age=86400,
                                httponly=True, secure=settings.SESSION_COOKIE_SECURE,
                                samesite="Lax", path="/")
            return response
        except backend.BackendError as exc:
            error, status = exc.message, exc.status
    return render(request, "login.html", {"error": error}, status=status)


@require_POST
def logout(request):
    try:
        backend.get_json("/api/v1/logout", request.COOKIES.get("go_session", ""), "POST")
    except backend.BackendError as error:
        return backend_error(request, error)
    response = redirect("/login/")
    response.delete_cookie("go_session")
    return response


@require_GET
def account(request):
    try:
        profile = backend.get_json("/api/v1/me", request.COOKIES.get("go_session", ""))
    except backend.BackendError as error:
        return backend_error(request, error)
    teacher = profile["role"] == "teacher"
    return render(request, "teacher.html" if teacher else "student.html", {
        "profile": profile, "live": True, "page_title": "Профиль",
        "workspace_role": "Преподаватель" if teacher else "Ученик",
        "workspace_label": "Кабинет", "workspace_home": "/teacher/" if teacher else "/student/",
    })
