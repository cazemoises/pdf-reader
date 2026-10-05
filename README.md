# PDF-Reader

Leitor de PDFs com React/TypeScript, backend Go com arquitetura hexagonal,
PostgreSQL e serviço Python/FastAPI/PyMuPDF para extração e OCR seletivo.
Uploads são processados sincronamente; páginas, notas, highlights e posição
de leitura são persistidos. PDFs originais ficam em um volume do backend.

```sh
docker compose up -d --build
```

A rede externa `shared-services` deve existir no ambiente de deploy existente.
Variáveis opcionais e portas: [.env.example](.env.example).
Frontend: 8081; backend: 8080; extractor: 8000. PostgreSQL não publica porta.
O backend aplica automaticamente as migrações embutidas.

A extração preserva blocos/linhas, coordenadas, tabelas, imagens como metadados,
estratégias e diagnósticos, mantendo texto compatível para o leitor.
Detalhes, limites, benchmark, corpus e rollback:
[documentação de extração](docs/extraction.md).

```sh
./scripts/validate.sh
```

Validação isolada em Docker: Python/OCR, Go e PostgreSQL real, ingestão HTTP,
benchmark, build do frontend e configuração Compose. O workflow existente roda
no self-hosted runner Linux; deploy de main exige aprovação desses checks.
Não é necessário instalar Tesseract ou bibliotecas Python manualmente na VM.
