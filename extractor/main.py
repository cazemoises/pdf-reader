import asyncio
import json
import logging
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
from fastapi import FastAPI, HTTPException, UploadFile
from settings import Settings
from limits import UploadLimit

app = FastAPI(title='pdf-reader extractor')
settings = Settings.from_env()
app.add_middleware(UploadLimit, max_bytes=settings.max_bytes + 1024*1024)
# A single disposable parser per service instance. Reject contention rather than queue uploads in RAM.
slot = threading.BoundedSemaphore(1)
logger = logging.getLogger('pdf_reader.extraction')


@app.get('/health')
def health() -> dict[str, str]:
    return {'status': 'ok'}


def isolated_extract(data):
    worker = str(Path(__file__).with_name('worker.py'))
    with tempfile.TemporaryFile() as output, tempfile.TemporaryFile() as errors:
        try:
            completed = subprocess.run([sys.executable, worker], input=data, stdout=output, stderr=errors,
                                       timeout=settings.timeout_seconds, check=False)
        except subprocess.TimeoutExpired:
            raise HTTPException(504, 'PDF processing timed out')
        output.seek(0)
        payload = output.read(settings.max_output_bytes + 1)
        if len(payload) > settings.max_output_bytes:
            raise HTTPException(422, 'PDF output limit exceeded')
        if completed.returncode != 0:
            logger.warning('stage=worker exit_code=%s', completed.returncode)
            raise HTTPException(422, 'Invalid PDF or processing budget exceeded')
        errors.seek(0)
        # Worker only logs metadata, never extracted text, filename or parser exception details.
        for line in errors.read(64*1024).decode(errors='replace').splitlines():
            if line.startswith('INFO:pdf_reader.extraction:'):
                logger.info('stage=worker %s', line)
        try:
            return json.loads(payload)
        except ValueError:
            raise HTTPException(422, 'PDF processing failed')


@app.post('/extract')
async def extract(file: UploadFile) -> dict:
    if not slot.acquire(blocking=False):
        await file.close()
        raise HTTPException(503, 'Extractor busy; retry later')
    task = None
    try:
        data = await file.read(settings.max_bytes + 1)
        if len(data) > settings.max_bytes:
            raise HTTPException(413, 'PDF file too large')
        task = asyncio.create_task(asyncio.to_thread(isolated_extract, data))
        # Cancellation must not release the slot while the native process is still running.
        return await asyncio.shield(task)
    finally:
        await file.close()
        if task is not None and not task.done():
            task.add_done_callback(lambda completed: (completed.exception() if not completed.cancelled() else None, slot.release()))
        else:
            slot.release()
