# Revisão adversarial da extração

Baseline: main (algoritmo legado), primeira evolução `bdffb782e62d7d835c895106ba28564fbb06b1b9`, e código desta rodada. Sem merge, deploy, instalação na VM ou modificação do runner. Fixtures novas são originais CC0, geradas em memória, sem binários externos. Relatório de CI e números repetidos são artefatos do workflow.

## Achados demonstrados

| Severidade | Problema | Evidência reproduzível | Correção |
|---|---|---|---|
| HIGH | Imagem decorativa grande disparava OCR; região híbrida pequena não disparava | `test_large_decorative_image_not_ocr`, `test_small_scanned_region_in_hybrid`, texto cinza claro no PDF real e fixture própria | Inspeção raster limitada, com máscara do texto nativo; área não aciona OCR |
| HIGH | Scan rotacionado gerava texto incorreto | Português exato em rotações 0/90/180/270 | OCR no referencial não rotacionado; restauração em finally |
| HIGH | OCR podia duplicar camada pesquisável ou substituir palavras nativas | Searchable scan e candidatos OCR adversariais | Merge por origem/geometria; preservação de palavras legíveis |
| HIGH | Repetição marginal apagava conteúdo legítimo | Frase legítima no topo repetida; corpo e primeira/última páginas diferentes | Apenas anotação `margin_candidate`; `exclude_from_text=false` |
| HIGH | Título abrangente fazia colunas intercalarem linhas | Título próximo de primeira linha, duas/três colunas desenhadas fora de ordem | Primeiro corte horizontal antes de subdivisão por gutter |
| HIGH | Bloco cruzando tabela duplicava células; candidato podia perder palavras | Tabela próxima de prosa e candidato incompleto injetado | Substituição por linha, cobertura de palavras exigida |
| HIGH | CID mascarava Unicode corrompido; NUL incompatível com JSONB | Fonte CJK embutida com CMap inválido, sem imagens | Desabilitar substituição por CID; NUL vira marcador explícito |
| HIGH | Uploads concorrentes ocupavam temporários antes da admissão | Requests concorrentes com body cuja leitura é proibida | Slot antes de multipart, prazo de upload, 503 sem ler corpo |
| HIGH | Parser nativo/OCR executava como root | Verificação de UID do processo e filho na imagem | UID 10001 declarativo; mesmas restrições Compose |
| HIGH | Leitura de uma página carregava JSONB do documento inteiro | Repositório fake falha se List for chamado | Consulta indexada por livro/número |
| MEDIUM | Cancelamento deixava livro processing; erro de persistência de página também | Extrator fake cancela request; contexto de Update observado | Cleanup com contexto independente limitado a 5s; marca failed nos erros |
| MEDIUM | Stderr nativo podia expor conteúdo mesmo com prefixo permitido | Teste de stderr com prefixo falsificado | Logs construídos a partir de campos estruturados, sem stderr/texto |
| MEDIUM | Score alto para texto semanticamente errado; whitespace escondia corrupção | `test_quality.py` | Sinais separados; denominator não branco; score explicitamente integridade |
| MEDIUM | Helpers de teste de migração dependiam de tabela criada por outro teste | Execução independente com PostgreSQL | Aplicar todas as migrations; teste isolado de legado/rollback |
| LOW | Script ESLint preexistente sem pacote/configuração | `artifacts/frontend-lint.txt` | Resultado registrado como warning; TypeScript obrigatório; não acrescentamos tooling cosmético |

Nenhum CRITICAL foi demonstrado. Os testes provaram regressões na primeira evolução; não inferimos que todos os PDFs reais tenham esses defeitos.

## Árvore de decisão OCR

1. Extrair linhas nativas, sem mascarar caracteres desconhecidos por CID. Calcular caracteres substitutos/controles sobre caracteres **não brancos**.
2. Razão inválida >5%: motivo `native_character_corruption`, modo full. O limite é política tolerante a artefatos pontuais, não probabilidade calibrada; testes cobrem os dois lados e diluição com espaços.
3. Havendo imagens, inspecionar até 32 regiões visíveis, escala máxima 1 e lado máximo 600. Mascarar linhas nativas utilizáveis. Componentes com nível de cinza ≤223 (inclui texto cinza claro, exclui fundo branco), com altura 3..60, largura até duas alturas, alinhados em fileiras de pelo menos quatro e distância até três alturas produzem `raster_glyph_rows_outside_native_text`.
4. Sem motivos: não OCR. Pouco texto e imagem grande **não são motivos**. Camada pesquisável cobrindo o raster não exige nova extração.
5. Com motivos e OCR habilitado/dentro do orçamento de raster: executar. Corrupção pede full; evidência raster pede partial. Engine indisponível/erro/orçamento gera warning e mantém nativo.
6. Merge: adicionar somente linhas da origem OCR sem sobreposição significativa (50% da menor área) com nativo utilizável. Bloco corrompido só é substituído com integridade melhor e todas as palavras nativas legíveis preservadas. Manter agrupamento de linhas do bloco OCR.

`page.ocr` registra requested/executed/accepted, reasons, skip_reason, mode, caracteres nativos, razão inválida, imagens inspecionadas, evidência raster e sinais do merge. Logs usam hash/página e metadados, sem conteúdo. `tools/ocr_audit.py` registra texto **sintético** e decisão por fixture/página no artefato; nenhum texto de PDF do usuário vai para logs.

Essa inspeção é uma heurística LTR explicável, não reconhecimento semântico: fotos podem conter componentes alinhados; texto invertido, claro, muito pequeno, menos de quatro glifos ou ruído extremo pode não ser detectado. Limites de inspeção geram incerteza/warnings. Fontes que produzam caracteres errados mas válidos não são detectadas automaticamente.

## Quality evaluation

| Sinal | Mede / detecta | Não demonstra / falsos resultados |
|---|---|---|
| characters | Volume incluindo separadores | Mais caracteres não provam cobertura; duplicação aumenta volume |
| non_whitespace_characters, empty | Conteúdo não branco / página vazia | Página vazia pode ser legítima; scan não reconhecido também fica vazio |
| invalid_characters, invalid_ratio | U+FFFD e controles não brancos / corrupção aparente | Texto semanticamente errado com Unicode válido passa; U+FFFD literal legítimo penaliza |
| duplicate_blocks | Blocos com texto idêntico | Refrão legítimo sinaliza; duplicação parcial ou segmentação diferente escapa |
| score, score_kind | 1-invalid_ratio, compatibilidade; character_integrity | Ordem invertida e palavras erradas podem ter 1.0; não é score de extração |
| ocr_confidence | null | TextPage não fornece confiança calibrada; não inventamos |
| warnings/ocr/strategy | Decisão, limites, origem e falhas de estágio | Não substituem ground truth |

Ground truth verifica conteúdo exato compacto, ordem, contagens de ocorrência, blocos/linhas, células, ausência de duplicação, Unicode e preservação de prosa. Não há snapshot gigante. Benchmarks ainda reportam cobertura de frases, sinal mais fraco que esses testes; PDF real não tem ground truth semântico.

## Casos adversariais

- Layout: duas/três colunas, título abrangente, ordem de criação diferente da visual, sidebar, posições incomuns e texto sobreposto. Conteúdo/ordem esperados preservados; sobrepostos não removidos silenciosamente.
- OCR: scan, imagem decorativa grande com pouco texto legítimo, híbrido pequeno, scan pesquisável, português acentuado, quatro rotações, baixa resolução e fonte corrompida sem imagem. Decisão e resultado explícitos; imagem decorativa nunca invoca engine nos testes.
- Margens: header/footer repetidos, número variável, frase legítima repetida no corpo/topo, primeira/última diferentes. Conteúdo conservado, candidatos anotados.
- Tabelas: grade, prosa alinhada com/sem caixas, texto próximo, células vazias/multilinha, bloco cruzando grade. Células exatas quando grade suportada; prosa alinhada não vira tabela. Sem linhas/scan não têm suporte semântico garantido.
- Segurança: 501 páginas, upload >32MiB, imagem-fonte gigantesca, timeout real com filho morto/reaped, multipart inválido, upload lento, cancelamento com slot retido, temporários/erro de filesystem, stderr confidencial, concorrência antes do body.
- Compatibilidade: schema antigo, JSONB null, textos/offsets de highlights antigos, migrations reaplicadas e SQL de aplicação antiga após migration aditiva. Não reextraímos páginas antigas.

## Segurança e recursos

Sem dependências novas nesta rodada. Mesmas bibliotecas nativas, instaladas no Docker; sem instalação manual na VM. Um upload/parser ativo por instância, Uvicorn explícito com um worker, OMP_THREAD_LIMIT=1. Concorrentes recebem 503/Retry-After; não há fila distribuída. Replicar containers multiplica slots e exige orçamento próprio.

Limites: 32MiB PDF/JSON, 33MiB multipart, 500 páginas, 90s de upload/subprocesso, 1.5GiB de endereço no worker, 20M pixels de raster, 40M pixels de imagem-fonte. Compose limita 2CPU/2GiB/pids64/tmpfs96MiB, filesystem readonly, capabilities removidas/no-new-privileges. Tempfiles têm nomes gerados e cleanup; cancelamento não libera capacidade antes do worker terminar. Sem shell/subprocesso parametrizado por filename.

Teste runtime confirma UID/filho10001, capabilities zero, root readonly, tmp cleanup e limites cgroup. Oito requests OCR simultâneos testam admissão e health. Um processo limitado não é sandbox suficiente contra exploração nativa; cgroups/timeout contêm custo, não provam ausência de vulnerabilidades do parser.

## Validação e benchmark

`./scripts/validate.sh` executa testes Python (71), Go com PostgreSQL real, vet, frontend (3 testes de compatibilidade), builds, Compose, quatro ingestões HTTP e verificações runtime/concorrência. ESLint preexistente falha por instalação/configuração ausente; esse check não está verde e fica registrado como warning. Há uma depreciação TestClient/httpx. Não há nova migration: 0008 continua aditiva/nullable, rollback conserva coluna extra.

Benchmark: um warm-up e cinco execuções por estratégia/documento, processo separado por estratégia, mesmos budgets 2CPU/2GiB. Mediana/min/max, CPU, RSS máximo incluindo caches, páginas/s e páginas OCR reais. Não compara RSS quente com os números frios anteriores (~59/90MiB). HTTP/startup não entram no tempo do parser; throughput sob concorrência é outra medição. Números e recursos do runner ficam nos artefatos, associados ao commit final.

## Riscos restantes, por prioridade

1. Inspeção raster heurística/sem ground truth real: baixo contraste (fixture cinza 0.7 aciona OCR mas Tesseract não recupera texto; warning explícito), inversão, fotos, minúsculos textos e caracteres errados válidos; coletar corpus autorizado antes de calibrar.
2. Ingestão não transacional: erro pode deixar páginas parciais num livro failed; queda abrupta do processo pode deixar processing. Fila/retry/transação precisam demanda concreta.
3. Layout LTR por gutters: colunas intercaladas, diagonal, RTL e elementos sobrepostos ainda podem exigir revisão visual. Preservar tudo pode deixar margens e duplicações legítimas/aparentes.
4. Tabelas sem linhas/scan e tipos semânticos (títulos/listas/captions) não reconhecidos de forma geral. Frontend ainda exibe parágrafos, sem tabela semântica.
5. Score não mede semântica; cobertura de caracteres/frases no PDF real não prova qualidade. OCR não fornece confiança.
6. ESLint legado incompleto; dependências e isolamento nativo devem seguir manutenção normal. Porta publicada do extractor permanece como infraestrutura existente.

### Medição local repetida do PDF existente (269 páginas)

Host amd64, 12 CPUs/15.6GiB; container 2CPU/2GiB. Um warm-up, cinco execuções; mesma versão PyMuPDF para os três algoritmos.

| Medida | Legado | Primeira evolução | Após hardening |
|---|---:|---:|---:|
| Wall mediana | 0.319s | 1.282s | 1.477s |
| Wall min..max | 0.316..0.328s | 1.264..1.303s | 1.462..1.508s |
| CPU mediana | 0.318s | 1.223s | 1.429s |
| RSS pico quente | 66.9MiB | 118.0MiB | 119.3MiB |
| Páginas/s | 843.5 | 209.9 | 182.1 |
| OCR executado | nenhuma | 1,2,4,5 | 1,5,136 |

Página 5 inicialmente regressou com corte de intensidade escura; inspeção visual e fixture cinza reproduziram o problema. O corte centralizado de cinza 223 recuperou seus 83 caracteres novamente. Páginas 2/4 continuaram vazias em ambas as versões, com warning. Página 136 possui tabela de associações rasterizada pequena acima da prosa: inspeção visual confirmou conteúdo adicional; extração preservou 1922 caracteres nativos e acrescentou 60. Isso não constitui ground truth do restante do documento.

O custo final cresce cerca de 15% em wall e 17% em CPU versus primeira evolução, RSS cerca de 1%; inclui OCR adicional útil, inspeção e salvaguardas. Profiling (com overhead, portanto fora da tabela) aponta avaliação de caracteres e extração nativa como principais custos cumulativos, seguidos pela decisão raster/OCR. Não foi feita otimização prematura. O algoritmo legado é rápido porque não recupera scan/estrutura.

### Corpus por categoria (local, cinco execuções após warm-up)

| Categoria / estratégia | Wall mediana (min..max) | CPU mediana | RSS pico | Páginas/s | Páginas OCR |
|---|---:|---:|---:|---:|---|
| Texto / legado | 0.0006s (0.0006..0.0007) | 0.0006s | 89.2MiB | 1575 | 0 |
| Texto / primeira evolução | 0.0009s (0.0008..0.0011) | 0.0009s | 89.2MiB | 1163 | 0 |
| Texto / hardening | 0.0009s (0.0008..0.0010) | 0.0009s | 89.2MiB | 1119 | 0 |
| Scan / legado | 0.0011s (0.0010..0.0018) | 0.0011s | 89.2MiB | 949 | 0 (conteúdo ausente) |
| Scan / primeira evolução | 0.1101s (0.1066..0.1280) | 0.0965s | 185.8MiB | 9.1 | 1 |
| Scan / hardening | 0.1205s (0.1186..0.1319) | 0.1092s | 215.2MiB | 8.3 | 1 |
| Híbrido / legado | 0.0014s (0.0013..0.0022) | 0.0014s | 89.2MiB | 696 | 0 (metade ausente) |
| Híbrido / primeira evolução | 0.1178s (0.1114..0.1229) | 0.1009s | 185.8MiB | 8.5 | 1 |
| Híbrido / hardening | 0.1230s (0.1178..0.1305) | 0.1078s | 216.1MiB | 8.1 | 1 |

As páginas textuais mínimas têm duração submilissegundo e dispersão relativa grande; servem como check de ausência de OCR, não como microbenchmark para otimização. RSS dos processos inclui carregamento dos geradores/fixtures e warm-up, além do pipeline. Nos sintéticos OCR há aumento aproximado de 29–30MiB versus primeira evolução; orçamento do container continua 2GiB.
