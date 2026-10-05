# Validação empírica em PDFs reais — 2026-10-05

## Resultado e limites da conclusão

A engine **não é confiável para leitura e estudo de todos os PDFs deste corpus**. Um livro praticamente desaparece; outro apresenta alterações de ordem dentro de frases e ruído de OCR. Há também documentos cuja prosa se mostrou fiel. A classificação abaixo se refere ao conteúdo que o leitor recebe, não ao sucesso do processo.

Foram lidos `extraction.md`, `hardening.md`, pipeline, OCR, layout, tabelas, quality, worker, serialização do backend, reflow do frontend e testes. Nenhuma alteração foi feita por causa da porta 8000. Não houve merge nem deploy.

## Corpus

Todos os seis arquivos são byte-a-byte distintos: SHA-256 do arquivo inteiro, sem deduplicação por nome ou título. Total: **346 páginas, 32.562.625 bytes**. Identificadores abaixo são os primeiros 16 caracteres do hash; hashes completos constam em [real-validation-metrics.json](real-validation-metrics.json).

| ID | Arquivo | Páginas | Bytes | Características |
|---|---|---:|---:|---|
| 25ca296fd415afa1 | Montanari - Comida como cultura.pdf | 173 | 30.577.020 | PDF 1.7, Microsoft Print To PDF; texto convertido em curvas vetoriais, ilustrações e anúncios raster |
| 24175689c4bdc807 | Relatorio_A_Invencao_da_Cozinha_Montanari-1.pdf | 6 | 132.192 | Texto nativo, títulos, listas, tabela comparativa sem linhas |
| a9e8ae9b0409be6e | Relatorio_A_Invencao_da_Cozinha_Montanari.pdf | 24 | 186.377 | Texto nativo, seções, listas e cinco tabelas sem linhas |
| 6227b441dd2a3591 | Relatorio_A_Inversao_da_Cozinha_Montanari.pdf | 8 | 24.843 | PDF 1.4 ReportLab; tabela com linhas, questões e gabarito |
| 8559fd80b2ae290f | juliecavignac,+4716-11340-1-CE.pdf | 12 | 174.747 | Artigo acadêmico; títulos bilíngues, referências sobrescritas, notas, bibliografia e marca lateral |
| 0a3e5b35077a662f | umbanda-pc3a9-no-chc3a3o-ramatis.pdf | 123 | 1.467.446 | PDF 1.4 Writer; prosa justificada, sumário, quadro bibliográfico, imagens com palavras e cantos |

`validation-corpus/` está no `.gitignore`. PDFs, extrações integrais e renderizações permanecem exclusivamente em `artifacts/real-validation/`, também ignorado. Só metadados, pequenos anchors manuais e exemplos sintéticos entram no Git.

## Método e cobertura visual

Cada PDF passou pelo `main.run_worker`, com limites normais de produção (90 segundos e limite de memória do subprocesso), em container com Tesseract e idiomas português/inglês. Todos concluíram, sem necessidade de execução diagnóstica com limite ampliado. Isso não foi considerado evidência de fidelidade.

A referência foi a página renderizada pelo PyMuPDF. A saída comparada foi a estrutura JSON e o texto produzido pela função **real** `reflowPageText` do frontend, usando a serialização compatível do backend. As pranchas mostram original e texto lado a lado; o layout do lado direito é uma aproximação tipográfica para auditoria, não uma captura do DOM. Esta rodada não simula seleção de highlights nem sessões completas do usuário no navegador. A integração HTTP foi verificada separadamente pelo smoke sintético.

**287 páginas inspecionadas visualmente**:

- Montanari: todas as 173, em 15 pranchas de 12 miniaturas, mais ampliações diagnósticas. Como 172 saídas são vazias, a comparação verifica presença de conteúdo visível, não transcrição palavra por palavra. As oito páginas ilustradas sem prosa não são contadas como perda textual.
- Relatórios de 6, 24 e 8 páginas e artigo de 12: todas as 50, lado a lado.
- Umbanda: 64/123, incluindo início, fim, intervalos de dez páginas, todas as páginas com imagens, todos os acionamentos de OCR, sinais de duplicação, inversões verticais e fragmentação suspeita; prosa densa, notas, quadros, sumário, páginas curtas e mudanças de seção.

A lista exata está no JSON de métricas. A amostra de Umbanda inclui 1–7, 10, 14, 17, 20, 24, 27, 30, 31, 40, 48–56, 60–62, 66, 70, 74, 75, 80, 90 e 94–123. As outras 59 páginas não receberam inspeção manual. Não se extrapola ausência de erro para essas páginas.

Além dos sinais do pipeline, uma busca geométrica independente procurou blocos que voltam verticalmente mais de 20 pontos e três ou mais fragmentos na mesma altura. Todas as páginas sinalizadas foram visualmente verificadas. A inversão é um **critério de investigação**, não prova de erro: pode ser coluna legítima ou cabeçalho deslocado. Foram examinados também blocos repetidos e páginas vazias. Não houve acionamento de recuperação deixado sem inspeção.

## Métricas por documento

“Nativa” significa estratégia final nativa, **inclusive páginas vazias**. “Suspeita” nesta tabela significa aviso do pipeline, acionamento de OCR/fallback ou `duplicate_blocks > 0`; problemas descobertos visualmente são descritos separadamente. OCR parcial/completo refere-se à estratégia aceita. Todos os quatro OCR foram solicitados, executados e aceitos; não houve recuperação de exceção adicional nem OCR completo de produção.

| Documento | Páginas | Nativas | OCR parcial | OCR completo | Suspeitas | Duplicação criada confirmada | Unicode inválido sinalizado | FP OCR | FN OCR observado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Montanari | 173 | 172 vazias | 1 | 0 | 173 | 0 | 0 | 0 | 164 |
| Relatório 6 | 6 | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| Relatório 24 | 24 | 24 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| Relatório 8 | 8 | 8 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| Artigo | 12 | 12 | 0 | 0 | 12 | 0 | 0 | 0 | 0 |
| Umbanda | 123 | 120 | 3 | 0 | 9 | 0 | 0 | 0 | 7 |

Os números de FN são **páginas com ausência visual confirmada de texto que exigiria OCR**, não estimativas de recall. As sete de Umbanda incluem duas de informações editoriais em capas; cinco contêm nomes nas ilustrações do corpo. Zero Unicode inválido não exclui erros de acento reconhecido como outra letra nem palavras aglutinadas: a capa de Montanari demonstra precisamente isso. Não há ground truth suficiente para publicar uma porcentagem de precisão.

| Documento | Erros/limitações observados | Ordem | Risco de conteúdo/significado |
|---|---|---|---|
| Montanari | 164 páginas textuais sem conteúdo; capa parcialmente recuperada, palavras aglutinadas e marcas inventadas | Ordem não avaliável nas saídas vazias | Perda crítica |
| Relatório 6 | Uma tabela achatada, p. 4 | Prosa preservada nas seis páginas | Relações de colunas exigem original |
| Relatório 24 | Cinco tabelas achatadas, pp. 2, 7, 11, 15, 19 | Prosa e sequência das células preservadas; estrutura de colunas perdida | Relações de tabelas exigem original |
| Relatório 8 | Quebra de subtítulo em dois parágrafos, p. 1 | Sem problema relevante observado | Sem falha relevante encontrada |
| Artigo | Referências de notas deslocadas em oito páginas; títulos/parágrafos fundidos; hifenização do reader | Referência sobrescrita antecipa a linha à qual pertence | Associação entre texto e nota fica ambígua |
| Umbanda | OCR inventa 19 linhas; omite palavras em imagens; quadro/sumário perdem relações; versos viram prosa | Fragmentos de frases pp. 50/53; títulos de cantos podem aparecer após o conteúdo | Risco relevante de interpretação |

### Estratégias e heurísticas por página

O JSON versionado contém **todas as 346 páginas**, estratégia, decisão/reasons de OCR, fallback, warnings, qualidade, tabelas e contagem de candidatos de margem; não contém prosa integral.

- Montanari: OCR parcial na 1, motivado por evidência raster fora do texto nativo. Nativa 2–173. Aviso `empty_page` nas páginas vazias sem imagens; `image_without_text` em 14, 15, 40, 41, 72, 73, 122, 123, 173. As primeiras oito são ilustrações sem prosa; a 173 tem anúncio com texto raster. Há conteúdo textual visível em todas as outras páginas, embora as curvas não entrem na política raster.
- Relatórios: nativa em todas. Reconhecimento de tabela somente na p. 2 do relatório de oito páginas. As tabelas não pautadas dos outros dois não são reconhecidas.
- Artigo: nativa em todas; `non_horizontal_text_order_uncertain` nas 12. O aviso decorre do elemento lateral; a leitura da prosa é melhor do que o aviso sugere. As referências sobrescritas precisam de inspeção adicional mesmo assim.
- Umbanda: OCR parcial em 48, 50, 55, mesma razão raster. Nativa nas demais; nenhum warning. Sinais de blocos repetidos em 1, 2, 3, 4, 10, 123 foram conferidos e correspondem a material do original. Candidatos de margem permanecem no texto conforme o contrato conservador atual.

Em Umbanda, a busca de inversões sinalizou 4–7, 48, 95–98, 100–113 e 115–123. Todos foram inspecionados; vários casos são apenas o autor do cabeçalho movido para o fim. Os fragmentos justificados de 50 e 53 são mais graves. Não se exige que a engine classifique semanticamente todo título, caption ou lista, mas a ordem e os vínculos legíveis continuam importantes para estudo.

## Resultados por documento e confiança

### Montanari — NÃO CONFIÁVEL

A extração inteira retorna só 40 caracteres na capa. O livro tem texto visível, convertido em desenhos vetoriais; as 172 páginas seguintes produzem zero texto. Excluindo oito páginas ilustradas, há **164 FN**: 163 páginas com texto vetorial e o anúncio raster da 173. A capa recupera parte do autor/título sem espaços, omite informação editorial e inclui marcas gráficas como caracteres.

OCR completo **forçado apenas como controle diagnóstico**, pp. 1, 4, 42, 71, 87 e 173, consegue texto nas páginas que a produção deixou vazias: p. 4 retorna 505 caracteres; p. 87, 1.606. Isso confirma que “não havia texto extraível nativamente” não equivale a “não havia conteúdo”. O controle também apresenta erros; não prova que OCR automático de todas as páginas resolveria fidelidade ou respeitaria o orçamento.

### Relatório de seis páginas — CONFIÁVEL COM RESSALVAS

Comparação de todas as páginas: texto, acentos, listas e títulos preservados sem perda de significado observada na prosa. Na p. 4, o quadro comparativo vira parágrafos individuais; os valores permanecem na ordem de linha, mas os vínculos com cabeçalhos não ficam explícitos. Pode ser usado para ler o texto; a tabela deve ser estudada no original.

### Relatório de 24 páginas — CONFIÁVEL COM RESSALVAS

Todas as páginas verificadas. Prosa, itens, negações e mudanças de seção permanecem legíveis e na ordem observada. As tabelas 2, 7, 11, 15 e 19 perdem a representação tabular. Na 19, nomes, função no capítulo e localização no PDF permanecem, mas a leitura exige reconstruir associações. Isso é perda de estrutura com risco de associação equivocada, não perda comprovada dos valores.

### Relatório de oito páginas — CONFIÁVEL

Todas as páginas verificadas. A tabela pautada da p. 2 preserva matriz e ordem; 30 questões e gabarito permanecem legíveis. Um subtítulo quebrado em dois parágrafos é cosmético. A grafia aparentemente suspeita “enfia dos” na p. 2 foi ampliada e **já está assim no original**: não é bug da extração. **Nenhuma falha relevante encontrada neste documento.** A classificação não promete inexistência de todo erro microscópico.

### Artigo — CONFIÁVEL COM RESSALVAS

As 12 páginas foram comparadas, inclusive todas com avisos. A prosa principal, notas finais e bibliografia são utilizáveis. Sobrescritos são classificados geometricamente antes da linha principal: na p. 5, a referência 6 antecede a linha em vez de vir após “divindades”. Casos observados em 2, 3, 4, 5, 6, 8, 9 e 10. O corpo do argumento não desaparece, mas não se deve confiar na posição da referência para estudar a nota sem consultar o PDF.

Títulos bilíngues e alguns parágrafos são fundidos. A normalização legada do reader remove hífen no fim de linha, podendo fundir compostos (como o composto que cruza a linha da p. 4); isso é distinto de corrupção Unicode. A marca lateral decorativa não foi usada para inflar FN de conteúdo necessário ao estudo.

### Umbanda — NÃO CONFIÁVEL para estudar exclusivamente a extração

A maioria da prosa amostrada está preservada. Contudo, as pp. 50 e 53 alteram a ordem **dentro de frases**: palavras de uma mesma linha justificada viram blocos separados depois da continuação. Na p. 50, o vínculo “Aspectos positivos: justiça, discernimento” se rompe. Na 53, palavras introdutórias de uma ressalva ficam deslocadas depois do parágrafo, afetando a interpretação.

As pp. 48, 50 e 55 incorporam marcas reconhecidas como texto; na 48, “Oxalá” da imagem não é recuperado; na 55, “Nanã” não é recuperado. A 50 recupera “Xangô”, mas também duas marcas sem significado. Na 56, perder o nome da imagem é especialmente ruim porque o texto segue o assunto da página anterior. Os nomes não recuperados em outras imagens e capas são descritos em OCR.

Na p. 4, o quadro bibliográfico perde os vínculos título/autor; o sumário 5–7 separa entradas e números. Títulos centralizados podem vir depois dos cantos: pp. 98 e 121 dão exemplos claros. Nos cantos finais, quebras de verso viram prosa; refrões repetidos no original não são bugs de duplicação. Cabeçalhos e números de página preservados são ruído cosmético adicional. Essas limitações impedem confiar no documento completo para estudo sem consulta frequente ao original, mesmo que grande parte da prosa seja útil.

## Falhas encontradas

Severidade: crítica = grande perda de conteúdo; alta = risco de associação/alteração de significado; média = estrutura/vínculo degradado; baixa = cosmética. “Aberto” significa observado e não corrigido; não transforma o relatório em promessa de implementação futura.

| Documento / página | Categoria | Severidade | Sintoma | Causa | Status |
|---|---|---|---|---|---|
| Montanari, 2–173 exceto 14/15/40/41/72/73/122/123 | Perda de conteúdo / FN | Crítica | 164 páginas com texto visível e saída vazia | `ocr.decide` cobre imagens/Unicode inválido, não texto em curvas; anúncio 173 também escapa à evidência raster | Aberto; controles OCR reproduzem recuperação parcial |
| Montanari, 1 | OCR / fidelidade | Alta | Autor/título aglutinados, omissões e três marcas gráficas | Reconhecimento insuficiente sobre arte da capa; score não mede fidelidade | Aberto |
| Relatório 6, 4; relatório 24, 2/7/11/15/19 | Estrutura | Média | Células viram parágrafos sem associação explícita de colunas | Detector não reconhece estes quadros sem linhas | Limitação observada; sem exigir nova feature |
| Artigo, 2/3/4/5/6/8/9/10 | Ordem / notas | Média | Marcador sobrescrito antecipado à linha | Ordenação por y superior e fragmentos de linha não recompostos | Aberto |
| Artigo, 4 | Fidelidade / hifenização | Média | Hífen de composto no fim de linha suprimido no reader | Reflow legado remove hífen de quebra sem contexto linguístico | Limitação conhecida; não alterada |
| Umbanda, 50/53 | Ordem / significado | Alta | Rótulo e palavras da mesma frase separados da continuação | `native_blocks` agrupa por interseção em x, separando fragmentos justificados; `reading_order` não reconstrói a linha | Aberto; reprodução sintética estrita |
| Umbanda, 48/50/55 | OCR / texto inventado | Alta | 19 linhas espúrias entram na saída; captions parcialmente perdidas | Tesseract reconhece traços de desenhos; merge aceita texto fora da área nativa sem validação semântica | Aberto |
| Umbanda, 1/3/49/51/52/53/56 | OCR / perda de conteúdo | Média | Palavras visíveis em imagens ausentes | Inspeção de componentes/linhas raster não encontra evidência suficiente | Aberto; duas capas e cinco captions |
| Umbanda, 4 | Estrutura / associações | Alta | Referências e autores do quadro perdem vínculo | Corte geométrico de colunas, sem estrutura de tabela reconhecida | Aberto |
| Umbanda, 5–7 | Estrutura / ordem | Média | Entradas separadas dos números do sumário | Gutter vertical priorizado à relação horizontal | Aberto |
| Umbanda, 98/121, exemplos | Ordem / títulos | Média | Título de seção depois dos cantos | Gutter interpreta título estreito centralizado como outra coluna | Aberto; reprodução sintética estrita |
| Umbanda, cantos 95–123 amostrados | Estrutura | Média | Versos fundidos em prosa | Reflow une linhas dentro do bloco | Limitação observada |
| Relatório 8, 1; Umbanda, cabeçalhos finais | Cosmética | Baixa | Subtítulo fragmentado / autor deslocado / número visível | Fragmentação e preservação conservadora de margens | Sem correção |

Não foi comprovada duplicação nova de linhas ou parágrafos. Os sinais de repetição de Umbanda foram comparados com o original: capas, referências, cabeçalhos e refrões têm repetições legítimas. Não se confundiu erro editorial do original com erro da engine.

## OCR: TP / FP / FN

Duas dimensões distintas: necessidade da decisão e suficiência da recuperação. Houve **4 decisões necessárias (TP quanto à necessidade), 0 FP observados, mas todos os quatro acionamentos são classificados como NECESSÁRIO MAS INSUFICIENTE**. Não se somam as duas contagens como oito eventos. Nenhum acionamento merece a classificação de recuperação satisfatória TRUE POSITIVE. FN observado: 164 + 7 = **171 páginas**, segundo o escopo textual descrito.

| Documento / página | Por que acionou | Necessário? | Recuperação / erro / duplicação | Classificação |
|---|---|---|---|---|
| Montanari 1 | Evidência de letras em região raster sem nativo | Sim | Recupera parte da capa; palavras unidas, dados ausentes, marcas inventadas; sem nativo a duplicar | NECESSÁRIO MAS INSUFICIENTE |
| Umbanda 48 | Mesma razão raster | Sim, nome na figura | Acrescenta 14 linhas espúrias, não recupera Oxalá; mantém texto nativo | NECESSÁRIO MAS INSUFICIENTE |
| Umbanda 50 | Mesma razão raster | Sim, nome na figura | Recupera Xangô, acrescenta `|` e `a`; não duplica nativo; ordem nativa já defeituosa | NECESSÁRIO MAS INSUFICIENTE |
| Umbanda 55 | Mesma razão raster | Sim, nome na figura | Acrescenta três marcas, não recupera Nanã; mantém texto nativo | NECESSÁRIO MAS INSUFICIENTE |

FN de Umbanda: 1/3 (informações editoriais), 49 (Iemanjá), 51 (Ogum), 52 (Iansã), 53 (Oxum), 56 (Omulu). A fotografia/símbolo da p. 2 não foi tratada como texto faltante. A definição não converte desenho sem legenda em necessidade de OCR.

Os controles forçados completos em Umbanda 48/50/55 não substituíram a produção nem demonstraram uma solução limpa: OCR completo também fragmenta e pode corromper texto já nativo. Isso contraindica uma troca global de parcial para completo baseada apenas nestes exemplos.

## Ground truth real

[real_ground_truth.json](../extractor/tests/real_ground_truth.json) registra **sete páginas manualmente validadas**, por hash e número PDF, com pequenos anchors e relações esperadas: Montanari 4 (texto vetorial ausente), relatório 6 p. 4 (tabela achatada), relatório 24 p. 19 (quadro denso), relatório 8 p. 2 (tabela preservada), artigo p. 5 (sobrescrito), Umbanda 50 (fragmentação/OCR) e 98 (título fora de ordem).

É um ground truth **parcial de conteúdo e relações**, não uma transcrição completa. Os anchors foram conferidos contra as renderizações; incluem situações positivas e falhas. Ele não é apresentado como uma suíte automática aprovada: as relações que falham hoje permanecem declaradas. O conjunto é pequeno, não contém PDFs nem capítulos, e não sustenta métricas globais de precisão. Artefatos visuais privados permitem nova verificação local.

## Correções e testes generalizáveis

**Nenhuma correção de engine foi implementada.** Não há evidência suficiente nesta rodada para trocar os cortes de coluna, inferir títulos, filtrar pequenos tokens de OCR ou aplicar OCR a todo desenho sem causar regressão em outras classes ou exceder recursos. Remover `a`, por exemplo, eliminaria ruído de uma página e poderia eliminar palavra legítima em outra. O texto vetorial exige uma decisão geral com avaliação de necessidade e custo; ativá-la indiscriminadamente seria uma nova heurística sem validação suficiente.

Foram criadas três reproduções sintéticas mínimas em [test_real_failure_classes.py](../extractor/tests/test_real_failure_classes.py), sem palavras dos livros: texto visível convertido em curvas sem texto nativo/imagem; fragmentos justificados na mesma baseline antes da continuação; título centralizado antes de corpo curto alinhado à esquerda. Todas são `xfail(strict=True)` com motivo explícito. **São bugs conhecidos ainda abertos, não testes corrigidos ou sucessos.** Um XPASS falha a suíte e exige revisão. Nenhum teste anterior foi relaxado.

Outras falhas ficam reproduzíveis por hash/página, decisão e artefato privado; não receberam testes artificiais que pressupõem uma nova classificação semântica ou solução não estabelecida. A única mudança operacional é ignorar o corpus privado.

## Regressões

`./scripts/validate.sh` concluiu com exit 0 em containers isolados:

- Extrator: primeira rodada **71 passed**; repetição completa após os novos testes **71 passed, 3 xfailed**; corpus sintético e adversarial anterior preservado.
- Backend: testes de filestorage, httpextractor, httpserver, postgres e domain passaram; `go vet ./...` passou.
- Integração de ingestão/leitura HTTP e auditoria OCR sintética concluíram; benchmark anterior executado com cinco medições e warm-up, sem transformá-lo em prova de fidelidade.
- Concorrência: oito requisições, uma aceita e sete rejeitadas com 503 conforme limite.
- Runtime: UID 10001 inclusive subprocesso, capabilities zeradas, no-new-privileges, filesystem somente leitura e limpeza temporária verificados; limites de memória/CPU/PIDs registrados.
- Frontend: **3 testes passaram**, builds de teste/produção e TypeScript passaram; configuração Compose válida. O script ESLint preexistente falhou (comando/dependência/configuração não fornecidos no projeto); o script registra esse aviso e não o considera sucesso de lint.

Após adicionar as reproduções: **71 passed, 3 xfailed**, no container com OCR. A última execução isolada desabilitou o cache pytest; resta um aviso de depreciação Starlette/httpx. A execução local sem Tesseract não é usada como validação de OCR. Não houve mudança de heurística, portanto as seis extrações auditadas correspondem à mesma engine testada; não há melhoria de corpus alegada nem reextração apresentada como efeito de correção.

Logs, benchmarks, métricas de segurança/concorrência, resultados e comparações ficam em `artifacts/`. São artefatos de execução, não material redistribuído.

## Reprodução local

A extração pode ser repetida com a imagem Docker de testes e o corpus privado montado somente leitura, chamando `main.run_worker` para cada arquivo. Os scripts locais `artifacts/real-validation/run.py`, `reflow.mjs`, `render.py`, `compare.py` e `controls.py` registram a execução usada nesta rodada. `catalog.json`, `native-inventory.json`, `sample-manifest.json`, seis JSON de resultados, seis saídas do reader, pranchas e `forced-full-controls.json` permanecem disponíveis no workspace, fora do Git. Para repetir em outro checkout, os PDFs e esses scripts privados precisam ser transportados separadamente; o relatório versionado não pressupõe redistribuição do corpus.

O JSON de métricas é autocontido para consultar modos, decisões e páginas inspecionadas. Tempos medidos são observações de uma execução com limites de produção; não são SLA.

## Próximos gargalos, por impacto observado

1. **Texto visível em curvas vetoriais não entra na recuperação:** perda de quase um livro inteiro. Detectar/avisar inadequação é tão necessário quanto discutir recuperação; score de integridade sozinho não protege o usuário.
2. **Reconstrução de fragmentos da mesma linha justificada:** altera o vínculo das palavras dentro de frases nativas em páginas de estudo.
3. **OCR de pequenas palavras em ilustrações, com rejeição de ruído:** tanto FN quanto aceitação de marcas sem significado; trocar tudo por OCR completo não resolve automaticamente.
4. **Relações de quadros/sumários e títulos centralizados:** preservar ordem e vínculos antes de ampliar classificação semântica.
5. **Associação de sobrescritos às notas, compostos hifenizados e versos:** perdas localizadas de estrutura/fidelidade do conteúdo de estudo.

Esta lista descreve evidência e impacto, não uma nova rodada de arquitetura. Há documentos suficientes para uso com as ressalvas indicadas; a implementação atual não sustenta uma promessa universal de leitura fiel.
