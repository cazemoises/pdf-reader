"""Cap uploads before multipart parsing/spooling, including chunked request bodies."""
from starlette.responses import JSONResponse


class UploadLimit:
    def __init__(self, app, max_bytes):
        self.app = app
        self.max_bytes = max_bytes

    async def __call__(self, scope, receive, send):
        if scope['type'] != 'http' or scope['path'] != '/extract':
            return await self.app(scope, receive, send)
        headers = dict(scope.get('headers', []))
        try:
            length = int(headers.get(b'content-length', b'0'))
        except ValueError:
            length = 0
        if length > self.max_bytes:
            return await JSONResponse({'detail': 'PDF upload too large'}, 413)(scope, receive, send)
        received = 0
        exceeded = False

        async def bounded_receive():
            nonlocal received, exceeded
            message = await receive()
            received += len(message.get('body', b''))
            if received > self.max_bytes:
                exceeded = True
                # Trigger parser disconnect/cleanup without buffering the oversized body.
                return {'type': 'http.disconnect'}
            return message

        async def bounded_send(message):
            if not exceeded:
                await send(message)

        try:
            await self.app(scope, bounded_receive, bounded_send)
        except Exception:
            if not exceeded:
                raise
        if exceeded:
            await JSONResponse({'detail': 'PDF upload too large'}, 413)(scope, receive, send)
