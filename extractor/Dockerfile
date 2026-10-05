FROM python:3.12-slim-bookworm AS runtime
WORKDIR /app
# OCR engine/data installed declaratively for both amd64 and arm64.
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-eng tesseract-ocr-por && rm -rf /var/lib/apt/lists/*
ENV OMP_THREAD_LIMIT=1 PYTHONDONTWRITEBYTECODE=1
RUN groupadd --gid 10001 pdfreader && useradd --uid 10001 --gid pdfreader --no-create-home pdfreader
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY *.py ./
EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1"]

FROM runtime AS test
COPY requirements-dev.txt .
RUN pip install --no-cache-dir -r requirements-dev.txt
COPY tests ./tests
COPY tools ./tools
CMD ["python", "-m", "pytest", "-q"]

FROM runtime AS production
USER 10001:10001
