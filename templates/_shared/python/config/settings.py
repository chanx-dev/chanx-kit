"""Django settings: the startproject defaults, plus Channels and chanx."""

import os
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent

# Development only: set your own secret and hosts before deploying.
SECRET_KEY = os.environ.get("DJANGO_SECRET_KEY", "dev-only-change-me")
DEBUG = os.environ.get("DJANGO_DEBUG", "1") == "1"
ALLOWED_HOSTS = ["localhost", "127.0.0.1"]

INSTALLED_APPS = [
    "daphne",
    "django.contrib.admin",
    "django.contrib.auth",
    "django.contrib.contenttypes",
    "django.contrib.sessions",
    "django.contrib.messages",
    "django.contrib.staticfiles",
    "channels",
    "chanx.channels",
    "app",
]

MIDDLEWARE = [
    "django.middleware.security.SecurityMiddleware",
    "django.contrib.sessions.middleware.SessionMiddleware",
    "django.middleware.common.CommonMiddleware",
    "django.middleware.csrf.CsrfViewMiddleware",
    "django.contrib.auth.middleware.AuthenticationMiddleware",
    "django.contrib.messages.middleware.MessageMiddleware",
    "django.middleware.clickjacking.XFrameOptionsMiddleware",
]

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [],
        "APP_DIRS": True,
        "OPTIONS": {
            "context_processors": [
                "django.template.context_processors.request",
                "django.contrib.auth.context_processors.auth",
                "django.contrib.messages.context_processors.messages",
            ],
        },
    },
]

ROOT_URLCONF = "config.urls"
ASGI_APPLICATION = "config.asgi.application"

DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": BASE_DIR / "db.sqlite3",
    }
}

# Redis when REDIS_URL is set (needed across processes), in memory otherwise.
CHANNEL_LAYERS = {
    "default": (
        {
            "BACKEND": "channels_redis.pubsub.RedisPubSubChannelLayer",
            "CONFIG": {"hosts": [os.environ["REDIS_URL"]], "prefix": "chanx-app"},
        }
        if os.environ.get("REDIS_URL")
        else {"BACKEND": "channels.layers.InMemoryChannelLayer"}
    )
}

CHANX = {
    "SEND_AUTHENTICATION_MESSAGE": False,
    "ASYNCAPI_TITLE": "chanx app",
    "ASYNCAPI_VERSION": "0.1.0",
    "ASYNCAPI_DESCRIPTION": "WebSocket API of this app.",
}

STATIC_URL = "static/"
DEFAULT_AUTO_FIELD = "django.db.models.BigAutoField"
USE_TZ = True
