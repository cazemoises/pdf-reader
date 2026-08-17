# pdf-reader

Aplicação para leitura/extração de conteúdo de PDFs. Este repositório está,
por enquanto, **apenas com a estrutura de pastas e configs base** — a
implementação real é feita depois pelo [orchestrator](../orchestrator),
rodando de forma autônoma contra `backend/` (ver
`ORCH_REPO_DIR=/caminho/pdf-reader/backend` no ambiente do orchestrator).

## Estrutura

```
backend/      # Go, module "pdf-reader/backend" — apenas internal/domain/
              # e internal/ports/ com um doc.go de propósito, sem código
              # de implementação ainda.
extractor/    # Python/FastAPI, extração de PDF via PyMuPDF — tem um
              # health check mínimo funcional (GET /health).
frontend/     # Vite + React + TypeScript + Tailwind — só configs e o
              # boilerplate padrão do scaffold, sem `npm install` rodado
              # (não há node_modules/ nem package-lock.json commitados).
docker-compose.yml   # Postgres + backend + extractor + frontend
```

## Estado atual de cada serviço

| Serviço | Builda hoje? | Observação |
|---|---|---|
| `extractor` | Sim | `GET /health` já funciona. |
| `frontend` | Não (ainda) | `npm ci` no Dockerfile precisa de `package-lock.json`, que só existe depois de um `npm install` real (não rodado de propósito nesta etapa). |
| `backend` | Não (ainda) | Não existe `cmd/server/main.go` ainda — só `internal/domain` e `internal/ports` vazios. `docker build` falha até esse entrypoint existir. |

Isso é esperado: `docker-compose.yml` já está com toda a topologia,
variáveis de ambiente, healthchecks e rede interna definidos, para
funcionar sem alterações assim que o orchestrator preencher `backend/` e
alguém rodar `npm install` uma vez em `frontend/`.

## Rodando localmente (depois que backend/frontend tiverem código real)

```bash
docker compose up -d --build
```

## Backend

Módulo Go independente (`go.mod` próprio, não faz parte do módulo
`orchestrator`). Arquitetura hexagonal: `internal/domain` para os tipos
centrais, `internal/ports` para as interfaces; `internal/adapters` e
`cmd/server` serão criados pelo Dev agent conforme o backlog avança.

## Extractor

Serviço Python separado (FastAPI + PyMuPDF) para extração de conteúdo de
PDF, chamado pelo backend via HTTP (`EXTRACTOR_URL`, ver
`docker-compose.yml`).

```bash
cd extractor
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
uvicorn main:app --reload
```

## Frontend

Vite + React + TypeScript + Tailwind. Configs criadas manualmente
(`package.json`, `vite.config.ts`, `tsconfig.json`, `tailwind.config.js`,
etc) — ainda sem `npm install` rodado. Para começar a desenvolver:

```bash
cd frontend
npm install
npm run dev
```

## CI/CD

Push em `main` dispara `.github/workflows/deploy.yml`, que roda no
self-hosted runner da VM (ver `../orchestrator/deploy/setup-runner.md` para
o guia de instalação do runner — o mesmo processo se aplica aqui, com um
runner registrado separadamente contra este repo) e builda/sobe os três
serviços via `docker compose up -d --build`.
