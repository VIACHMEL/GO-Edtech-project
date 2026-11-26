"""Страницы сайта: данные и авторизация приходят из Go."""


from django.conf import settings
from django.contrib.staticfiles import finders
from django.http import Http404
from django.middleware.csrf import rotate_token
from django.shortcuts import redirect, render
from django.views.decorators.http import require_GET, require_POST, require_http_methods

import backend_client as backend
import learning_ui


@require_GET
def index(request):
    return render(request, "index.html")


@require_http_methods(["GET", "POST"])
def register(request):
    fields = {name: request.POST.get(name, "") for name in ("name", "email", "role")}
    if request.method == "POST":
        password = request.POST.get("password", "")
        try:
            profile = backend.get_json("/api/v1/register", method="POST", data={
                **fields, "password": password,
            })
        except backend.BackendError as error:
            message = "Этот email уже зарегистрирован" if error.status == 409 else error.message
            return render(request, "register.html", {"fields": fields, "error": message}, status=error.status)
        try:
            data = backend.get_json("/api/v1/login", method="POST", data={
                "email": fields["email"], "password": password,
            })
        except backend.BackendError as error:
            return render(request, "login.html", {
                "error": "Аккаунт создан, но автоматический вход не удался. Войдите вручную.",
            }, status=error.status)
        rotate_token(request)
        response = redirect("/teacher/" if profile["role"] == "teacher" else "/student/")
        response.set_cookie("go_session", data["token"], max_age=86400,
                            httponly=True, secure=settings.SESSION_COOKIE_SECURE,
                            samesite="Lax", path="/")
        return response
    return render(request, "register.html", {"fields": {"role": "student"}})


@require_GET
def not_connected(request, title, active):
    try:
        profile = backend.get_json("/api/v1/me", request.COOKIES.get("go_session", ""))
    except backend.BackendError as error:
        return backend_error(request, error)
    teacher = profile["role"] == "teacher"
    return render(request, "not_connected.html", {
        "profile": profile, "live": True, "page_title": title, "active_page": active,
        "base_template": "teacher_base.html" if teacher else "student_base.html",
        "workspace_role": "Преподаватель" if teacher else "Ученик",
        "workspace_label": "Кабинет", "workspace_home": "/teacher/" if teacher else "/student/",
    })


@require_http_methods(["GET", "POST"])
def teacher_builder(request):
    token = request.COOKIES.get("go_session", "")
    filters = {name: request.GET.get(name, "").strip()
               for name in ("subject", "year", "task_number", "task_type", "limit")}
    try:
        profile = backend.get_json("/api/v1/me", token)
        if profile["role"] != "teacher":
            raise backend.BackendError(403, "Требуется роль преподавателя.")
        if request.method == "POST":
            worksheet_id = learning_ui.create_worksheet(request, token)
            return redirect(f"/teacher/builder/?worksheet={worksheet_id}")
        builder = backend.get_teacher_builder(filters, token)
        assignment_context = learning_ui.builder_assignment_context(request, token)
    except backend.BackendError as error:
        return backend_error(request, error)
    builder["user_id"] = profile["id"]
    return render(request, "teacher_builder.html", {
        "profile": profile, "live": True, "page_title": "Конструктор", "active_page": "builder",
        "builder": builder, "filters": filters, **assignment_context,
    })


@require_GET
def solve(request):
    attempt = request.GET.get("attempt", "")
    if not attempt:
        return redirect("/student/assignments/")
    if not attempt.isascii() or not attempt.isdecimal() or len(attempt) > 19:
        raise Http404("Попытка не найдена")
    try:
        variant = backend.get_json(f"/api/v1/attempts/{int(attempt)}/solve", request.COOKIES.get("go_session", ""))
    except backend.BackendError as error:
        if error.status == 404:
            raise Http404("Попытка не найдена") from error
        return backend_error(request, error)
    response = render(request, "solve.html", {
        "variant": variant,
        "board_assets_ready": bool(finders.find("solve-board/dist/solve-board.js")),
    })
    response["Cache-Control"] = "no-store"
    return response


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
