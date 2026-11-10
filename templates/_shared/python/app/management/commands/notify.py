"""Send a notification from another process. Needs the server's REDIS_URL.

REDIS_URL=redis://localhost:6399/0 uv run python manage.py notify --user chris "Hi"
"""

import os
from typing import Any

from django.core.management.base import BaseCommand, CommandError

from asgiref.sync import async_to_sync

from app.ws_kits.notification import (
    BroadcastNotificationTopic,
    NotificationPayload,
    UserNotificationTopic,
)


class Command(BaseCommand):
    help = "Send a notification to one user, or to everyone."

    def add_arguments(self, parser: Any) -> None:
        parser.add_argument("title", nargs="*", help="notification title")
        parser.add_argument("--user", help="address one user instead of everyone")

    def handle(self, *args: Any, **options: Any) -> None:
        if not os.environ.get("REDIS_URL"):
            raise CommandError(
                "REDIS_URL is not set: an in-memory layer cannot reach the server. "
                "Use the same REDIS_URL as the server."
            )
        title = " ".join(options["title"]) or "Hello from another process"
        payload = NotificationPayload(title=title, body="sent from manage.py notify")
        if user := options["user"]:
            async_to_sync(UserNotificationTopic.notify_user)(user, payload)
        else:
            async_to_sync(BroadcastNotificationTopic.notify_all)(payload)
        self.stdout.write(f"Sent {title!r} to {user or 'everyone'}")
