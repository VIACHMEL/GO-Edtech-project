"""Минимальные настройки только для локального запуска."""

import os
from pathlib import Path

from django.core.management.utils import get_random_secret_key

BASE_DIR = Path(__file__).resolve().parent.parent

# Служебный ключ Django: новый при каждом запуске, не хранится в файлах.
SECRET_KEY = get_random_secret_key()

DEBUG = os.environ.get("DJANGO_DEBUG", "true").lower() == "true"
ALLOWED_HOSTS = os.environ.get("DJANGO_ALLOWED_HOSTS", "localhost,127.0.0.1,[::1]").split(",")
ROOT_URLCONF = "config.urls"
WSGI_APPLICATION = "config.wsgi.application"

INSTALLED_APPS = ["django.contrib.staticfiles"]
MIDDLEWARE = ["django.middleware.csrf.CsrfViewMiddleware"]
DATABASES = {}

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [BASE_DIR / "templates"],
        "APP_DIRS": False,
    }
]
STATIC_URL = "/static/"
STATICFILES_DIRS = [BASE_DIR / "static"]
STATIC_ROOT = BASE_DIR / "staticfiles"

# Адрес Go-сервера и время ожидания сетевой операции в секундах.
BACKEND_BASE_URL = os.environ.get("GO_API_BASE_URL", "http://127.0.0.1:8080")
BACKEND_TIMEOUT = 3

SESSION_COOKIE_SECURE = os.environ.get("COOKIE_SECURE", "false").lower() == "true"
