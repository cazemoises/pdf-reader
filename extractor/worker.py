"""One disposable parser process per document; no parser state shared between requests."""
import json
import logging
import resource
import sys
from settings import Settings

if __name__ == '__main__':
    settings = Settings.from_env()
    resource.setrlimit(resource.RLIMIT_AS, (settings.memory_bytes, settings.memory_bytes))
    resource.setrlimit(resource.RLIMIT_CPU, (settings.timeout_seconds, settings.timeout_seconds))
    resource.setrlimit(resource.RLIMIT_FSIZE, (settings.max_output_bytes, settings.max_output_bytes))
    logging.basicConfig(level=logging.INFO)
    from pipeline import extract_document, ExtractionError
    try:
        result = extract_document(sys.stdin.buffer.read(settings.max_bytes + 1), settings)
        payload = json.dumps(result, ensure_ascii=False).encode()
        if len(payload) > settings.max_output_bytes:
            raise ExtractionError('output size limit exceeded')
        sys.stdout.buffer.write(payload)
    except ExtractionError as exc:
        sys.stdout.write(json.dumps({'error': str(exc)}))
        sys.exit(2)
    except Exception:
        # Native/parser details may contain PDF content. Only expose a stable generic error.
        sys.stdout.write(json.dumps({'error': 'PDF processing failed'}))
        sys.exit(2)
