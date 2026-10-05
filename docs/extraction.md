# Extração de documentos

## Reconhecimento e diagnóstico

Fonte de verdade: código em `backend/cmd/server`, `backend/internal/adapters`,
`extractor/main.py` e `frontend/src/pages/ReaderPage.tsx`. O README anterior descrevia
um scaffold, apesar de a aplicação já estar implementada.

Fluxo existente: React envia multipart a `POST /books`; o servidor Go gera ID
aleatório, salva o PDF no volume de arquivos, cria Book `processing`, chama
sincronamente `POST /extract` e grava cada Page no PostgreSQL. O livro vira `ready`
ou `failed`. Não há fila, worker persistente ou broker. Migrações SQL embutidas
são aplicadas pelo backend no startup. Notas, highlights e progresso usam suas
próprias tabelas. Highlights são offsets no texto refluído do frontend.

Antes, o Python lia o arquivo inteiro e usava `get_text('blocks')` sem ordenação.
Retornava texto e bbox, mas Go guardava apenas texto, concatenado com `\n`.
O frontend junta linhas sem linha vazia: os parágrafos eram misturados. Não havia
OCR, células de tabelas, métricas de qualidade, limites de páginas, timeout do
cliente HTTP ou erros controlados para PDFs inválidos. O endpoint async executava
o parser nativo no event loop. O único teste Python era um Hello World.

O corpus original gerado em memória demonstra: ordem interna diferente da visual
(inclusive duas colunas no mesmo bloco PyMuPDF); ausência de texto em scan;
metade do conteúdo híbrido ausente; margens repetidas no texto principal;
células sem estrutura. Acentos foram preservados no baseline: não eram um bug
reproduzido. Não presumimos que toda página vazia seja scan.

Compose contém PostgreSQL 16, Go, Python/FastAPI/PyMuPDF e React/Vite/TypeScript
servido por nginx. A rede externa `shared-services` já existe e é preservada.
O workflow original fazia build/deploy em main e verificava healthchecks, sem
rodar testes. O runner remoto registrado é `sitio-enokizono-vm`, Linux ARM64,
online na investigação; a máquina local usa amd64. Nenhum runner foi instalado,
reconfigurado ou substituído.

## Arquitetura nova

```
upload limitado -> processo descartável -> inspeção por página
  -> blocos/linhas nativos -> decisão de OCR -> seleção do resultado
  -> tabelas nativas -> ordem geométrica -> margens repetidas -> diagnósticos
  -> JSON versionado -> texto compatível + JSONB por página
```

- `settings.py`: orçamentos operacionais e política centralizada, substituível nos testes.
- `pipeline.py`: loader, inspeção, extratores nativo/OCR/tabelas e composição.
- `layout.py`: normalização Unicode, ordenação por cortes de espaço vazio e margens.
- `tables.py`: filtro de grades vetoriais antes do parser especializado.
- `quality.py`: integridade de caracteres, duplicações e vazio.
- `worker.py`: processo descartável com limites Linux de endereço, CPU e saída.
- `limits.py`: limite HTTP antes de parsing multipart, incluindo requests sem Content-Length.
- `main.py`: endpoint compatível, exclusão mútua, subprocesso com timeout e erros HTTP.
- adaptador Go: mantém o contrato legado, separa parágrafos em schema v2 e conserva o JSON.
- domínio/repositório Page e migração `0008`: campo opcional `extraction` JSONB.
- `tests/corpus.py`: dez categorias sintéticas, CC0, sem binários/fonts externos.
- `tools/benchmark.py`: comparação em processos independentes por documento/estratégia.
- `tools/smoke.py`: ingestão real Go/Python/PostgreSQL e leitura da página persistida.
- `scripts/validate.sh`: ambiente efêmero de validação, sem volumes/portas de produção.

Cada bloco mantém texto, tipo, origem, bbox e, para texto, linhas/direções;
tabelas mantêm matriz de células. Páginas mantêm estratégia, warnings, diagnóstico
nativo/final, duração, rotação e geometria das imagens. Imagens não são exportadas
como arquivos: somente localização/dimensões, sem copiar megabytes de pixels.
A geometria de texto/imagens é **não rotacionada**, conforme PyMuPDF; campos
`coordinate_width/height` tornam isso explícito. `width/height` mantêm dimensões
visuais legadas. Não se deve misturar os dois sistemas de coordenadas.

A representação intermediária permite novos extratores sem mudar os endpoints
ou reler o PDF para reordenar blocos já extraídos. JSONB conserva campos de
estratégias futuras sem exigir uma tabela por tipo de bloco.

## Escolha das estratégias

Extração nativa sempre vem primeiro. Área de imagem não decide OCR. A inspeção
raster limitada procura fileiras de componentes semelhantes a glifos fora das
linhas nativas utilizáveis. Essa evidência solicita OCR parcial; corrupção de
mais de 5% dos caracteres não brancos solicita OCR completo, inclusive sem imagens.
A decisão, motivos, execução, aceitação e contagens ficam em `page.ocr`.
OCR parcial acrescenta apenas linhas OCR sem sobreposição com texto nativo;
a camada nativa é preservada. Substituição de blocos corrompidos exige melhora de
integridade e preservação das palavras nativas legíveis. Ausência/erro do engine
preserva os blocos nativos e gera warning. Consulte [hardening.md](hardening.md)
para critérios, falsos positivos/negativos e evidências adversariais.

Tabelas com grade vetorial usam o extrator existente. A substituição só acontece
quando as células cobrem as palavras das linhas nativas contidas na tabela;
linhas externas de um bloco que cruza a grade são preservadas. Tabelas sem linhas
e escaneadas continuam sem reconhecimento semântico garantido.

Margens repetidas são candidatas no JSON, sem remoção automática da prosa:
repetição e posição não distinguem cabeçalho de frase legítima. A janela de 6%,
mínimo três páginas e metade do documento só produzem anotação. Limites e
heurísticas estão centralizados em Settings e são substituíveis nos testes.
A inspeção usa lado máximo 600 pixels, até 32 imagens, no máximo 50.000 componentes;
fileiras de quatro componentes reduzem confusão com ícones, mas não provam texto.
OCR mantém 150 DPI, por+eng, máximo 20 milhões de pixels de renderização;
imagens-fonte maiores que 40 milhões de pixels são rejeitadas antes de decode.
`PDF_OCR_ENABLED` e `PDF_OCR_LANGUAGE` continuam configuráveis pelo Compose.

Normalização usa NFC e remove soft hyphens; hífens ASCII são conservados.
A heurística anterior do frontend para hífen final permanece por compatibilidade
com páginas já gravadas e seus highlights. Ela ainda pode juntar palavras compostas.
Não removemos automaticamente blocos iguais: repetição pode ser legítima.

## Qualidade e benchmark

`quality.score` mede **integridade de caracteres**, não ordem, cobertura ou
fidelidade semântica. Página sem texto tem score null. Confiança do OCR também é
null porque o TextPage não expõe confiança calibrada; não inventamos um valor.
O benchmark mede cobertura de frases conhecidas e ordem contra ground truth,
caracteres, estratégia/fallback/warnings por página, tempo real e CPU por documento
e RSS máximo em processos separados. A geração das imagens sintéticas fica fora
do processo medido. O benchmark executa um warm-up e cinco amostras por estratégia, reportando
mediana, mínimo/máximo, CPU, RSS e páginas/s. RSS é pico do processo, incluindo
warm-up e caches; não se compara diretamente com uma execução fria. Tempo HTTP/startup
e transporte não fazem parte do benchmark do parser.

```sh
./scripts/validate.sh
# Relatório em artifacts/extraction-benchmark.json, também anexado pelo Actions.
docker build --target test -t pdf-reader-extractor-test extractor
docker run --rm pdf-reader-extractor-test python tools/benchmark.py
# Opcional: PDF próprio (sem ground truth; não imprime o texto)
docker run --rm -v /caminho/pdfs:/fixtures:ro pdf-reader-extractor-test \
  python tools/benchmark.py --pdf /fixtures/documento.pdf
```

Fixtures: texto simples, multipágina, colunas, tabela com linhas, scan, híbrido,
rotação de 90 graus, acentos, margens repetidas e título/lista/caption como texto.
São geradas de forma determinística com frases esperadas, em memória; não se
adicionam PDFs grandes. O PDF de 269 páginas já presente em `backend/dev-data`
é somente validação manual: sua origem/licença não está documentada no repositório,
portanto não o distribuímos como novo corpus nem afirmamos ground truth dele.

## Segurança, operação e compatibilidade

Upload PDF máximo 32 MiB (HTTP multipart 33 MiB), máximo 500 páginas, saída JSON
máximo 32 MiB, um parser ativo por instância; concorrentes recebem 503.
Subprocesso usa 90 segundos de timeout real/CPU, 1.5 GiB de espaço de endereço e
limite de tamanho de arquivo. Upload/resultado são temporários sem nomes fornecidos
pelo cliente; contexto gerencia cleanup. Cancelamento HTTP não libera o slot
antes de o subprocesso terminar. Não há subprocessos shell nem caminhos de PDF
fornecidos pelo usuário. Erros públicos são genéricos, 413/422/503/504.
Go limita upload, limpa multipart temporário e usa timeout HTTP de 100 segundos.
O container executa como UID 10001; admissão ocorre antes do multipart e upload
tem prazo de 90 segundos. Go admite uma ingestão por instância antes de ler o corpo
e persiste `failed` sob contexto independente e limitado após cancelamento.
Logs registram hash curto do documento, página, estratégia, duração, caracteres,
sinais de qualidade, decisão/motivos de OCR e warnings; não registram texto, filename ou exceções nativas detalhadas.

Compose limita extractor a 2 GiB, 2 CPUs, 64 processos, root filesystem readonly,
/tmp de 96 MiB, sem capabilities e sem elevar privilégios. Isso limita impacto
operacional; o subprocesso sozinho não é sandbox contra exploração de código
nativo. A publicação atual da porta do extractor é preservada.

API pública continua com book/page/text/dimensões e acrescenta `extraction`
opcional em páginas novas. Resposta interna `/extract` mantém `pages` e acrescenta
`schema_version: 2`. O adaptador continua aceitando a resposta anterior com suas
quebras originais. Documentos antigos não são reextraídos automaticamente: isso
mudaria os offsets de highlights/progresso. Tabelas novas têm linhas separadas
no texto principal; a UI permanece um leitor de parágrafos, sem componente de
tabela semântica. Texto novo contém fronteiras de parágrafos com `\n\n`.

Mudança operacional: rebuild da imagem para incluir Tesseract/dados e aplicação
da migração aditiva. Risco: OCR aumenta latência/memória e tabelas/layout possuem
heurísticas. Rollback: voltar imagens/código/Compose anteriores, conservando a
coluna nullable extra (não é preciso apagá-la). Para desabilitar OCR sem rollback,
`PDF_OCR_ENABLED=false`. Nenhuma instalação manual na VM é necessária.

## Dependências e alternativas

PyMuPDF já era dependência; fixado em 1.28.2, versão efetivamente validada. Seu
suporte a coordenadas, tabelas e OCR evita um segundo parser. Avaliados conceitualmente:
pdfplumber para layouts/tabelas (outro parser e custos sem necessidade para o bug
reproduzido), pypdf para texto/metadata (não resolve OCR/layout), modelos de layout
com pesos (maior custo operacional, sem corpus justificando essa complexidade).
Tesseract e dados eng/por são a única nova ferramenta nativa, instalados pelo apt
na imagem bookworm para amd64/arm64; licença Apache 2.0. PyMuPDF mantém sua licença
AGPL/comercial existente. O projeto não declara uma licença própria; não presumimos
compatibilidade jurídica além de não mudar a dependência/obrigação já existente.

Referências técnicas primárias:
[PyMuPDF Page/OCR/tables](https://pymupdf.readthedocs.io/en/latest/page.html),
[PyMuPDF licença](https://pymupdf.io/licensing),
[Tesseract instalação/licença](https://tesseract-ocr.github.io/tessdoc/Installation.html).

## CI/CD

O mesmo workflow usa o runner existente. O job validate compila Python, roda testes
incluindo OCR e erros HTTP, gera benchmark, builda Go, executa testes com PostgreSQL
efêmero e go vet, faz ingestão HTTP real, builda frontend e valida Compose.
Imagens/redes/containers possuem nomes únicos e cleanup via trap. Banco de teste
não expõe porta e não usa volume persistente. Deploy em main depende de validate;
a branch de validação só executa testes, sem deploy. Forks/PRs não executam código
arbitrário no runner: o trigger continua restrito a pushes internos e dispatch.

## Limitações e prioridades futuras

1. Adicionar ground truth de documentos reais autorizados, com anotação de ordem
   e células; calibrar decisões OCR/margens e orçamento contra PDFs da aplicação.
2. Calibrar inspeção raster para imagens de baixo contraste/invertidas, texto muito
   pequeno, fotos e fontes corrompidas que produzam caracteres aparentemente válidos.
3. Reconhecer títulos/listas/captions/footnotes com dados de fontes e validar
   visualmente; suportar tabelas sem linhas/escaneadas e renderização semântica na UI.
4. Melhorar layouts com colunas intercaladas, RTL, texto diagonal e parágrafos
   atravessando páginas. Corte geométrico de espaço vazio é uma heurística LTR.
5. Considerar fila com concorrência limitada e retry para 503, operações transacionais
   de ingestão e isolamento nativo mais forte, conforme demanda real.

## Evidência da rodada de hardening

Resultados, regressões reproduzidas, benchmarks repetidos e limites estão em
[hardening.md](hardening.md). Os artefatos de cada execução do workflow incluem
benchmark, auditoria OCR por fixture/página, recursos, concorrência e segurança.
