"""Reserve capacity before multipart spooling; cap upload bytes and wall time."""
import asyncio
import threading
from starlette.responses import JSONResponse


class UploadLimit:
    def __init__(self, app, max_bytes, slot=None, timeout_seconds=90):
        self.app = app
        self.max_bytes = max_bytes
        self.slot = slot or threading.BoundedSemaphore(1)
        self.timeout_seconds = timeout_seconds

    async def __call__(self, scope, receive, send):
        if scope['type'] != 'http' or scope['path'].rstrip('/') != '/extract':
            return await self.app(scope, receive, send)
        if not self.slot.acquire(blocking=False):
            return await JSONResponse({'detail': 'Extractor busy; retry later'}, 503)(scope, receive, send)
        scope['pdf_slot_reserved'] = True
        try:
            return await self.limited(scope, receive, send)
        finally:
            # Once the endpoint owns the reservation, its worker completion/cancellation releases it.
            if not scope.get('pdf_slot_transferred'):
                self.slot.release()

    async def limited(self, scope, receive, send):
        headers = dict(scope.get('headers', []))
        try:
            length = int(headers.get(b'content-length', b'0'))
        except ValueError:
            length = 0
        if length > self.max_bytes:
            return await JSONResponse({'detail': 'PDF upload too large'}, 413)(scope, receive, send)
        received = 0
        failure = None
        deadline = asyncio.get_running_loop().time()+self.timeout_seconds

        async def bounded_receive():
            nonlocal received, failure
            try:
                remaining = max(0, deadline-asyncio.get_running_loop().time())
                message = await asyncio.wait_for(receive(), remaining)
            except TimeoutError:
                failure = (408, 'PDF upload timed out')
                return {'type': 'http.disconnect'}
            received += len(message.get('body', b''))
            if received > self.max_bytes:
                failure = (413, 'PDF upload too large')
                return {'type': 'http.disconnect'}
            return message

        async def bounded_send(message):
            if failure is None:
                await send(message)

        try:
            await self.app(scope, bounded_receive, bounded_send)
        except Exception:
            if failure is None:
                raise
        if failure:
            await JSONResponse({'detail': failure[1]}, failure[0])(scope, receive, send)
