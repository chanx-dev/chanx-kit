import json

from django.http import HttpRequest, JsonResponse
from django.views.decorators.csrf import csrf_exempt
from django.views.decorators.http import require_POST

from asgiref.sync import async_to_sync

from app.ws_kits.notification import (
    BroadcastNotificationTopic,
    NotificationPayload,
    UserNotificationTopic,
)


@csrf_exempt
@require_POST
def notify(request: HttpRequest) -> JsonResponse:
    """Push to one user's tabs, or to everyone; the socket does the rest."""
    data = json.loads(request.body)
    payload = NotificationPayload(title=data["title"], body=data.get("body"))
    if user := data.get("user"):
        async_to_sync(UserNotificationTopic.notify_user)(user, payload)
        return JsonResponse({"sent_to": user})
    async_to_sync(BroadcastNotificationTopic.notify_all)(payload)
    return JsonResponse({"sent_to": "everyone"})
