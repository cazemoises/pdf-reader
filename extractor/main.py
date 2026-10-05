import asyncio
import json
import logging
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
from fastapi import FastAPI, HTTPException, UploadFile, Request
from settings import Settings
from limits import UploadLimit

app = FastAPI(title='pdf-reader extractor')
settings = Settings.from_env()
# A single disposable parser per service instance. Reject contention rather than queue uploads in RAM.
slot = threading.BoundedSemaphore(1)
app.add_middleware(UploadLimit, max_bytes=settings.max_bytes + 1024*1024, slot=slot, timeout_seconds=settings.timeout_seconds)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger('pdf_reader.extraction')


@app.get('/health')
def health() -> dict[str, str]:
    return {'status': 'ok'}


def isolated_extract(data):
    try:
        return run_worker(data)
    except OSError:
        raise HTTPException(503, 'PDF worker resources unavailable')


def run_worker(data):
    worker = str(Path(__file__).with_name('worker.py'))
    with tempfile.TemporaryFile() as output, tempfile.TemporaryFile() as errors:
        try:
            completed = subprocess.run([sys.executable, worker], input=data, stdout=output, stderr=errors,
                                       timeout=settings.timeout_seconds, check=False)
        except subprocess.TimeoutExpired:
            raise HTTPException(504, 'PDF processing timed out')
        except OSError:
            raise HTTPException(503, 'PDF worker resources unavailable')
        output.seek(0)
        payload = output.read(settings.max_output_bytes + 1)
        if len(payload) > settings.max_output_bytes:
            raise HTTPException(422, 'PDF output limit exceeded')
        if completed.returncode != 0:
            logger.warning('stage=worker exit_code=%s', completed.returncode)
            raise HTTPException(422, 'Invalid PDF or processing budget exceeded')
        try:
            result = json.loads(payload)
        except ValueError:
            raise HTTPException(422, 'PDF processing failed')
        # Never forward native stderr: parser diagnostics may contain attacker-controlled PDF data.
        # Emit only diagnostics from the pipeline's structured result, without block/line text.
        for page in result['pages']:
            decision = page.get('ocr', {})
            logger.info('document=%s page=%s strategy=%s ocr_reasons=%s ocr_executed=%s ocr_accepted=%s skip_reason=%s duration_ms=%s characters=%s invalid_ratio=%s native_characters=%s native_invalid_ratio=%s warnings=%s',
                        result['document_id'], page['page_number'], page['strategy'], decision.get('reasons'),
                        decision.get('executed'), decision.get('accepted'), decision.get('skip_reason'),
                        page['duration_ms'], page['quality']['characters'], page['quality']['invalid_ratio'],
                        decision.get('native_characters'), decision.get('native_invalid_ratio'), page['warnings'])
        return result


@app.post('/extract')
async def extract(file: UploadFile, request: Request) -> dict:
    if not request.scope.get('pdf_slot_reserved') and not slot.acquire(blocking=False):
        await file.close()
        raise HTTPException(503, 'Extractor busy; retry later')
    request.scope['pdf_slot_transferred'] = True
    task = None
    try:
        data = await file.read(settings.max_bytes + 1)
        if len(data) > settings.max_bytes:
            raise HTTPException(413, 'PDF file too large')
        task = asyncio.create_task(asyncio.to_thread(isolated_extract, data))
        # Cancellation must not release the slot while the native process is still running.
        return await asyncio.shield(task)
    finally:
        try:
            await file.close()
        finally:
            release_slot(task)


def release_slot(task):
    if task is not None and not task.done():
        task.add_done_callback(lambda completed: (completed.exception() if not completed.cancelled() else None, slot.release()))
    else:
        slot.release()
