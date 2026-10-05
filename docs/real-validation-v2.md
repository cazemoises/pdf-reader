# Validação real v2: detecção e recuperação de defeitos

Data: 2026-10-05. Baseline: `ecb2d0fe1f1a58b04e81601be05f68968acdc84b`. Separação de conjuntos: commit `c146b2b`, anterior às alterações. Implementação congelada: `a08a995`, anterior à execução do holdout. Nenhuma heurística foi ajustada após seus resultados.

**Resultado: a detecção melhorou substancialmente; a recuperação ainda não torna Montanari ou Umbanda confiáveis para leitura e estudo.** Os 171 casos históricos são agora sinalizados; 164 recebem uma decisão de recuperação e sete recebem apenas suspeita. Na execução normal, somente 23 das 164 páginas antes sem texto vetorial ganharam texto. Outras 141 continuam sem conteúdo, embora explicitamente detectadas. As sete perdas raster continuam presentes. Há erros de ordem e de reconhecimento no novo OCR. Detectar não significa corrigir.

## Corpus e protocolo

Foram relidos integralmente os documentos de extração, hardening e validação v1, suas métricas e ground truths, as reproduções sintéticas e o pipeline atual. O corpus permanece privado e ignorado pelo Git. Identidade dos documentos é SHA-256 dos bytes, não nome. Os seis arquivos presentes são únicos; eventuais cópias não integram métricas como novos documentos. Não foram versionados PDFs, renders, transcrições extensas ou saída textual completa.

| ID SHA-256 abreviado | Arquivo | Páginas | Bytes | Característica determinante |
| --- | --- | ---: | ---: | --- |
| `25ca296fd415afa1` | Montanari - Comida como cultura.pdf | 173 | 30.577.020 | Corpo com letras desenhadas como contornos; capa raster em faixas |
| `24175689c4bdc807` | Relatorio_A_Invencao_da_Cozinha_Montanari-1.pdf | 6 | 132.192 | Texto nativo, tabela sem bordas |
| `a9e8ae9b0409be6e` | Relatorio_A_Invencao_da_Cozinha_Montanari.pdf | 24 | 186.377 | Texto nativo, títulos e tabela sem bordas |
| `6227b441dd2a3591` | Relatorio_A_Inversao_da_Cozinha_Montanari.pdf | 8 | 24.843 | Texto nativo e tabela com grade recuperada |
| `8559fd80b2ae290f` | juliecavignac,+4716-11340-1-CE.pdf | 12 | 174.747 | Artigo com referências sobrescritas e notas |
| `0a3e5b35077a662f` | umbanda-pc3a9-no-chc3a3o-ramatis.pdf | 123 | 1.467.446 | Texto nativo justificado, figuras com texto raster, sumário e cantos |
| Total | 6 documentos | 346 | 32.562.625 | |

O [split congelado](real-validation-v2-split.json) tem 22 páginas de desenvolvimento e 324 de holdout. O holdout contém todos os documentos, conteúdo vetorial e raster, uma segunda ocorrência de ordem justificada e controles nativos. Os PDFs já foram vistos na v1: isso é um holdout de páginas não usadas para ajuste na v2, **não validação externa em documentos inéditos**.

Os [rótulos congelados](real-validation-v2-labels.json) delimitam a matriz: perda de cobertura ou corrupção por OCR previamente observada. São 175 páginas ruins (171 omissões + quatro OCRs insuficientes), 69 controles bons nessa dimensão e 102 páginas sem rótulo nessa dimensão. Uma página controle pode ter limitação estrutural conhecida, por exemplo tabela sem bordas. Ela não foi declarada universalmente fiel. Desenvolvimento: 12 ruins/6 boas anotadas; holdout: 163 ruins/63 boas anotadas. As outras páginas não entram em precision/recall.

Executamos todos os seis documentos antes/depois pelo `main.run_worker`, com timeout normal de 90 s e limite do filho de 1,5 GiB, em contêiner de 2 CPUs/2 GiB. Todos terminaram dentro desses limites. Isso é apenas evidência operacional. A avaliação de fidelidade utilizou o original renderizado lado a lado com o texto produzido pelo reflow TypeScript real do Reader; a disposição à direita é uma reconstrução aproximada para inspeção, não screenshot de navegador.

As 22 páginas de desenvolvimento foram comparadas visualmente. Após congelar o código, foram inspecionadas todas as 23 novas páginas com OCR completo, os quatro OCRs históricos, todas as 12 páginas alteradas do artigo e as páginas alteradas/defeituosas de Umbanda, incluindo 51–53 e 56 no holdout. Nos 141 casos vetoriais ainda vazios, a reavaliação integral cruzou o original já validado visualmente na v1 com saída novamente vazia e evidência vetorial por página: a perda continua; não há texto para validar palavra a palavra. As 38 páginas dos relatórios preservaram exatamente o resultado textual do Reader. Saídas nativas inalteradas e renders já inspecionados na v1 foram reutilizados como referência; não alegamos uma nova transcrição manual das 346 páginas. Os 171 casos têm registro individual nas [métricas v2](real-validation-v2-metrics.json).

## Taxonomia dos 171 falsos negativos

Os 171 eram falsos negativos da **decisão de OCR**, não 171 páginas livres de quaisquer avisos. A página vazia de Montanari já tinha warning/score de vazio. O defeito era não distinguir ausência legítima de texto de ausência de texto visual, nem escolher recuperação para contornos. Esse esclarecimento evita atribuir à v2 uma descoberta de sinais que a v1 já emitia.

| Causa técnica principal | Páginas | Documentos/páginas | Detectável? | Recuperável? |
| --- | ---: | --- | --- | --- |
| Letras convertidas em contornos vetoriais, sem camada textual | 163 | Montanari, páginas 2–172 exceto 14,15,40,41,72,73,122,123 | Sim: contornos compactos curvos preenchidos, alinhados em linhas, fora de texto nativo | Em parte por OCR completo; sem Unicode nativo a reordenar |
| Página mista: texto publicitário em contornos e logos raster | 1 | Montanari 173 | Sim: contornos de letras; a evidência raster isolada era insuficiente | Parcial no desenvolvimento; não executada no orçamento normal medido |
| Letras curtas/claras dentro de ilustração raster complexa | 5 | Umbanda 49,51,52,53,56 | Só suspeita nesta versão; formas da ilustração e letras se confundem | Ainda não recuperadas |
| Texto em logos/editorial raster, com prosa nativa saudável | 2 | Umbanda 1,3 | Só suspeita nesta versão | Ainda não recuperado |
| Total | 171 | Montanari 164 + Umbanda 7 | Todos sinalizados no conjunto conhecido | 23 passaram de vazias a não vazias; isso não prova recuperação integral |

A página 173 foi refinada em relação à descrição v1: não é uma omissão puramente raster. Há texto desenhado e imagens. Nenhum caso desses 171 apresentou evidência de CMap quebrado como causa principal. Ordem justificada é uma classe adicional, sobreposta à omissão raster em Umbanda 53; Umbanda 50 já recebia OCR e não faz parte dos 171. Não somamos essas duas páginas novamente à taxonomia de omissões.

### Anatomia da perda vetorial

Nas páginas de desenvolvimento 4,42,71,87,173, `get_text("text")`, `blocks`, `words`, `dict` e `rawdict` não apresentam conteúdo textual utilizável. Não há fontes nem operadores `BT`/`ET`/`Tj`/`TJ` que contenham essas letras. Alterar `sort=True`, decodificação Unicode ou CMap não pode reconstruir texto inexistente nessas APIs. O render mostra letras porque os streams desenham contornos com `m`, `l`, `c`, `f`/`f*`.

| Página Montanari | Paths preenchidos | Items de desenho | Contornos compactos curvos candidatos | Texto nativo utilizável |
| --- | ---: | ---: | ---: | ---: |
| 4 | 419 | 9.154 | 329 | 0 |
| 42 | 1.010 | 23.443 | 864 | 0 |
| 71 | 674 | 14.545 | 542 | 0 |
| 87 | 1.352 | 32.348 | 1.165 | 0 |
| 173 | 321 | 4.849 | 114 | 0 |

Existem clips `W*` nessas páginas, mas a comparação visual/path não encontrou uma camada textual utilizável escondida pelo clipping; não é a causa das omissões. Há 5 imagens na página 173; os contornos explicam o restante visualmente textual. A capa 1 é diferente: 18 faixas raster, sem contornos/fontes utilizáveis. Em Umbanda, fontes e operadores de texto existem e as APIs retornam a prosa. A ausência dos títulos dentro das figuras é raster, não fonte sem Unicode.

A v1 procurava recuperação a partir de invalid_ratio e regiões **raster** com componentes semelhantes a glyphs. Uma página com milhares de contornos e zero texto não apresentava nenhuma dessas duas evidências. `invalid_ratio=0` sobre texto ausente não mede cobertura. Em páginas mistas, milhares de caracteres nativos corretos também não dizem nada sobre o texto pequeno dentro de uma figura. O teste raster exigia pelo menos quatro componentes alinhados; letras claras, conectadas, curtas ou associadas à arte não passam necessariamente. Nas ilustrações que acionaram OCR, formas do desenho contribuíram para o sinal, sem garantir recuperação da legenda.

### Anatomia da ordem justificada

Em Umbanda 50, três fragmentos estão na mesma baseline 415,6, fonte 12, `wmode=0`, direção `(1,0)`. Começam em x≈120,6 / 201,408 / 284,796. O primeiro termina em 165,876: o gap é ≈35,532, quase três tamanhos de fonte. A continuação começa em x≈85,2 e vai até 565,1, baseline 429,4.

`words`, spans e `rawdict` preservam coordenadas suficientes. O agrupamento antigo por interseção comum de x juntava primeiro fragmento + continuação e deixava os outros fragmentos para depois do parágrafo. Ordenar blocos completos não podia corrigir uma frase já montada incorretamente. A correção reconstrói a linha **antes** dessa etapa: baseline relativa ao tamanho da fonte, não igualdade literal de y nem um limite específico de livro. Uma continuação que atravessa os fragmentos permite reconhecer espaçamento de justificação sem unir colunas de tabela indiscriminadamente. Umbanda 53 confirma essa mesma classe no holdout.

## Mudanças implementadas

| Alteração | Problema resolvido | Generalização e limites |
| --- | --- | --- |
| Reconstrução horizontal de fragmentos dentro do mesmo bloco PyMuPDF | Palavras na mesma linha colocadas após a continuação | Baseline, direção, tamanho de fonte e continuidade geométrica; tolerâncias relativas. Preserva linhas rotacionadas, writing mode diferente e GlyphLessFont do OCR |
| Evidência vetorial fora da camada textual | Contornos visíveis ignorados pela decisão raster | Curvas preenchidas compactas, alinhamento e proximidade, exclusão de regiões nativas. Bordas/retângulos não bastam; não é `vetores → OCR` |
| Render pequeno somente para paths complexos sem texto/candidatos suficientes | Produtores que consolidam contornos em um único path | Lado máximo 600 pixels antes da busca de componentes; teste sintético de texto em vetores retangulares consolidados |
| Suspeita separada para tinta raster complexa descoberta | Sete omissões raster não reconhecidas como texto | Marca incerteza sem OCR automático. Também marca imagens sem texto: falsos positivos observados |
| Validação de suporte geométrico das linhas novas de OCR | Linhas fora da página, vazias ou sem componentes compatíveis aceitas pelo merge | Bbox e tinta renderizada; não usa palavras, nomes ou idioma. Desenhos semelhantes a letras ainda passam |
| Estado de detecção e evidências separados da recuperação | Ausência de pedido confundida com extração fiel | `HEALTHY`, `SUSPICIOUS`, `RECOVERY_REQUIRED`; `recovery_verified=false` não promete fidelidade |
| Orçamento de recuperação por documento | Detectar vetores não pode tornar toda extração longa um timeout | Metade do timeout disponível para OCR + processamento candidato/merge; detecção continua após esgotamento. Recuperação incompleta fica explícita |

A implementação reutiliza `get_drawings()` na detecção e na tabela; não faz duas leituras desnecessárias dos desenhos. A inspeção vetorial usual usa geometria sem render. Reconstrução usa baseline ≤0,2 do menor tamanho de fonte e gap ≤4 tamanhos; gap maior que 1,5 requer continuação que atravessa ambos os fragmentos. São parâmetros geométricos testados com escala, não constantes da fonte de um livro. Ainda requerem validação em mais produtores de PDF.

Não houve alteração de score para mascarar defeitos, flexibilização de testes anteriores, regra por título/autor/página/texto, redesenho geral do OCR ou nova feature semântica. `HEALTHY` significa ausência das evidências implementadas, **não certificação de que o conteúdo serve para estudo**. A API agora registra evidências; a interface atual não foi redesenhada para transformar isso numa garantia ao usuário.

## Detection

Matriz principal na amostra anotada de 244 páginas. Colunas são rótulos reais da baseline, restritos a cobertura/OCR; linhas são decisão. Antes usa pedido de recuperação, compatível com a definição histórica dos 171 FNs. Depois usa estado diferente de `HEALTHY`.

| Antes | REAL BOA | REAL RUIM |
| --- | ---: | ---: |
| DETECTADA RUIM | FP 0 | TP 4 |
| DETECTADA BOA | TN 69 | FN 171 |

| Depois | REAL BOA | REAL RUIM |
| --- | ---: | ---: |
| DETECTADA RUIM | FP 12 | TP 175 |
| DETECTADA BOA | TN 57 | FN 0 |

Recall 2,29% → 100%; precision 100% sobre somente 4 positivos → 93,58%. **Essas porcentagens não são precisão textual, nem estimativa populacional sobre as 346 páginas ou futuros documentos.** A alteração da definição de positivo é intencional e explicitada: inclui suspeita fraca. Os 12 FP representam 17,39% dos 69 controles; esse custo não é desprezível.

Para não esconder a informação anterior, a política comparável “qualquer pedido OCR, warning ou duplicate_blocks” já tinha TP170/FN5/FP13/TN56. Incluindo os novos sinais, passa a TP175/FN0/FP13/TN56: recall 97,14% → 100%; precision 92,90% → 93,09%. Logo, não seria correto dizer que todos os 171 pareciam saudáveis a todos os quality signals. O avanço maior é tornar os 164 casos vetoriais evidência **acionável e específica**, em vez de aviso genérico de vazio.

Decisão forte de recuperação, antes/depois: TP4/FN171/FP0/TN69 → TP168/FN7/FP0/TN69. Recall 2,29% → 96%. Os sete restantes são suspeitos raster, não pedidos de OCR. Entre os 171 históricos, 164 agora requerem recuperação e sete somente suspeita. A execução pode ser adiada pelo orçamento sem apagar a detecção.

No corpus completo, 197/346 páginas têm estado v2 não saudável: Montanari 172, artigo 12 e Umbanda 13. É contagem de sinais, não número de páginas efetivamente defeituosas.

FP anotados: Montanari 14,15,40,41,72,73,122 (ilustrações sem texto a recuperar); artigo 1,7,11,12 (fragmentos reconstruídos em páginas sem defeito relevante na dimensão rotulada); Umbanda 2 (foto). Nenhum deles acionou OCR. Outros oito sinais do artigo coincidem com páginas com problema de notas já registrado, mas essa classe está fora da matriz de cobertura/OCR.

## Recovery

| Medida observável | Antes | Depois | Interpretação |
| --- | ---: | ---: | --- |
| Páginas vetoriais defeituosas que recebem pedido | 0/164 | 164/164 | Detecção forte |
| Destas, execução normal de OCR | 0/164 | 23/164 | Limite de recuperação, não FN de detecção |
| Destas, retorno de texto antes totalmente ausente | 0/164 | 23/164 | Recuperação parcial de cobertura; 14,02%, não fidelidade integral |
| Perdas históricas ainda sem recuperação relevante | 171 | 148 | 141 vetoriais + 7 raster |
| Propriedades de ordem justificada nas duas ocorrências reais verificadas | 0/2 | 2/2 | Desenvolvimento 50, holdout 53; não mede toda a ordem do documento |
| Marcas espúrias dos quatro OCRs históricos removidas | 0 | 8 | 3 capa + 5 ilustrações; outros erros permanecem |

Entre as 171 páginas históricas detectadas, 23 retornam texto antes ausente: 13,45%. Esse é um proxy de **recuperação parcial**, não taxa de páginas integralmente corrigidas. Uma taxa de recuperação completa “TP detectados versus páginas efetivamente fiéis” permanece **não determinada**: não há transcrição integral de todas essas páginas e a inspeção já encontrou erros no novo texto. Não usamos `accepted=true`, score ou presença das anchors para contar página inteira como corrigida.

A produção realizou OCR completo nas páginas 2–13 e 16–26 de Montanari; a capa 1 continuou parcial. As demais 141 necessidades recebem `ocr_document_time_budget_exceeded` e `RECOVERY_REQUIRED`. O ponto de interrupção depende do custo, não de um número de página fixo. Desenvolvimento isolado recuperou trechos de 42,71,87,173 sem o orçamento do livro completo; esses resultados **não** foram usados como se tivessem ocorrido na execução normal.

A comparação visual das 23 páginas mostrou conteúdo substancial recuperado, mas também fragmentação de parágrafos, palavras reconhecidas incorretamente e frases fora de ordem. Em Montanari 12, o OCR distribui partes da primeira frase e da seguinte em blocos que se intercalam: a alteração compromete compreensão. Em 3, um ordinal é reconhecido como número diferente; em 7 aparecem marcas do desenho como texto; em 26 há erro na primeira palavra do título. Não declaramos essas páginas integralmente corrigidas.

## OCR: TP / FP / FN

Necessidade de OCR nos 175 casos anotados: 168 precisam de recuperação de camada visual, incluindo os quatro pedidos antigos; sete precisam de recuperação seletiva ainda não decidida. Mantemos as sete no denominador histórico de necessidade para comparação.

| Contagem por necessidade/decisão | Antes | Depois |
| --- | ---: | ---: |
| Pedidos necessários (TP de decisão) | 4 | 168 |
| Pedidos desnecessários nos controles (FP) | 0 | 0 |
| Necessários sem pedido (FN) | 171 | 7 |
| OCRs realmente executados | 4 | 27 |
| Necessários sem execução | 171 | 148 |
| OCR parcial / completo | 4 / 0 | 4 / 23 |

“TP de decisão” não equivale a “OCR bem-sucedido”. Os quatro OCRs históricos continuam **NECESSÁRIOS MAS INSUFICIENTES**. Os 23 novos são necessários e produzem recuperação parcial, sem certificação de página integral. Não houve transformação de ilustrações legítimas em OCR nem aumento de OCR em Umbanda.

| Caso real | Origem visual do conteúdo espúrio | Por que o merge antigo aceitava | Resultado |
| --- | --- | --- | --- |
| Montanari 1 | Bboxes do OCR de faixas, margens/suporte inválido; três marcas `o`, travessão, `o` | Texto Unicode válido sem nativo sobreposto; não verificava suporte no render | Três removidas; nome/título legítimos permanecem aglutinados |
| Umbanda 48 | Traços/texturas da ilustração interpretados como letras | Fora das caixas de prosa nativa, logo classificados como complemento | `_`, `ar`, `hale` removidos; 11 linhas espúrias permanecem |
| Umbanda 50 | Traços da figura próximos à legenda | Sem sobreposição nativa nem corrupção Unicode | `|` removido, legenda gráfica legítima mantida; `a` espúrio permanece |
| Umbanda 55 | Traços da figura com formas parecidas com glyphs | Mesmo mecanismo de complemento | `vu,` removido; `h` e `4` permanecem; legenda não recuperada |

Exemplos geométricos: na capa, o travessão ocupa x≈288,96 com largura≈595,65, ultrapassando a largura de 595,32; outra marca começa em y≈806,88, além da página. Em Umbanda 48, `ar` tem largura≈0,92 e altura≈46,08; o crop não contém glyph compatível com essa caixa. Em 50, o traço `|` tem largura≈0,91 e altura≈12,96. O filtro compara bbox OCR × tinta no crop; a prosa nativa é preservada pelo merge e a geometria nativa explica por que essas marcas externas não eram suprimidas por overlap.

O total histórico de ruído nas figuras cai de 19 para 14 linhas. Isso não é uma redução global de ruído do corpus: os 23 OCRs novos também introduzem erros. Uma textura com componentes compatíveis ainda passa no suporte geométrico; é necessário reconhecer esse limite em vez de descartar palavras por comprimento/vocabulário. Scans e híbridos sintéticos existentes continuam passando, inclusive preservação do texto nativo e não duplicação pelo merge.

## Holdout e ground truth

Resultados de detecção nas 226 páginas anotadas do holdout:

| Política | TP | FP | TN | FN | Precision | Recall |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Antes: pedido | 0 | 0 | 63 | 163 | Indefinida | 0% |
| Depois: estado | 163 | 12 | 51 | 0 | 93,14% | 100% |
| Depois: pedido | 159 | 0 | 63 | 4 | 100% | 97,55% |

Desenvolvimento anotado: antes TP4/FN8/TN6/FP0; depois TP12/FN0/TN6/FP0. O contraste com 12 FP no holdout é evidência real de um limite do detector, não motivo para alterar o holdout. A ordem da frase justificada foi corrigida em 53 sem usá-la para ajuste v2; sua legenda raster continua perdida. O holdout confirma detecção da classe vetorial e seus limites de recuperação/custo. Não confirma fidelidade integral do livro.

As [sete ground truths v1](../extractor/tests/real_ground_truth.json) foram mantidas. Resultado completo das relações: Montanari 4 agora contém as anchors ordenadas, mas o sumário ainda é fragmentado; relatório 8 p2 continua com tabela correta; relatórios 6 p4 e 24 p19 continuam sem preservar relações como tabela; artigo p5 continua com referência sobrescrita fora do lugar; Umbanda p50 corrige a frase e mantém a legenda, mas ainda contém marca alheia; Umbanda p98 mantém título após cantos. Não há remoção de falhas conhecidas.

Foram acrescentadas somente [cinco páginas com anchors mínimas](../extractor/tests/real_ground_truth_v2.json), validadas no original durante avaliação após congelamento, sem ajuste do código. Elas não substituem o split/rótulos congelados nem as relações v1.

| Página de holdout | Propriedade | Antes → depois |
| --- | --- | --- |
| Montanari 10 | Dois trechos curtos, incluindo negação, presentes na ordem | Falha → passa parcialmente |
| Montanari 86 | Frase visível em página tardia | Falha → falha; detectada, orçamento esgotado |
| Umbanda 53 | Cláusula justificada contínua antes da continuação | Falha → passa na propriedade nativa |
| Umbanda 56 | Legenda `Omulu` recuperada daquela região | Falha → falha |
| Relatório 24 p18 | Títulos antes da prosa correspondente | Passa → passa, texto do Reader idêntico |

Os [resultados das anchors](real-validation-v2-ground-truth-results.json) registram os checks parciais, não uma precisão textual. Nenhum PDF ou capítulo foi transformado em fixture.

## Documentos e confiança para ler/estudar

Contagens abaixo são da execução normal. “Nativa” inclui página sem texto na qual não se executou/aceitou OCR; não significa extração completa. “Suspeitas” inclui os dois estados não saudáveis. Unicode refere-se ao sinal invalid_characters, não a todas as substituições visualmente possíveis.

| Documento | Páginas | Nativas | OCR parcial | OCR completo | Suspeitas v2 | FN de pedido OCR antes → depois | Classificação |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Montanari | 173 | 149 | 1 | 23 | 172 | 164 → 0 | **NÃO CONFIÁVEL** |
| Relatório 6 | 6 | 6 | 0 | 0 | 0 | 0 → 0 | **CONFIÁVEL COM RESSALVAS** |
| Relatório 24 | 24 | 24 | 0 | 0 | 0 | 0 → 0 | **CONFIÁVEL COM RESSALVAS** |
| Relatório 8 | 8 | 8 | 0 | 0 | 0 | 0 → 0 | **CONFIÁVEL** |
| Artigo | 12 | 12 | 0 | 0 | 12 | 0 → 0 | **CONFIÁVEL COM RESSALVAS** |
| Umbanda | 123 | 120 | 3 | 0 | 13 | 7 → 7 | **NÃO CONFIÁVEL** |

Montanari: 40 → 28.933 caracteres extraídos, mas 141 páginas com conteúdo ainda retornam zero. Parte do texto novo altera ordem/significado. Não permite estudar o livro como substituto do original. Umbanda: prosa nativa mantida, duas frases corrigidas, mas perdas de figuras, ruído OCR, sumário/bibliografia e títulos deslocados permanecem. Melhora localizada não autoriza elevar sua confiança.

Relatórios 6/24: prosa/títulos preservados, texto do Reader byte a byte idêntico; associação nas tabelas sem bordas permanece uma ressalva para estudo comparativo. Relatório 8: tabela com grade e prosa continuam preservadas dentro do conjunto observado. Artigo: todas as 12 páginas pós-alteração comparadas visualmente; multiconjunto de palavras nativas preservado, prosa principal sem nova perda observada; notas/sobrescritos continuam deslocados. Classificações são condicionadas ao corpus e amostragem v1, não garantias universais.

Invalid_characters: zero antes/depois nos seis documentos. Isso não detecta ordinal trocado, palavra inventada ou ordem incorreta. duplicate_blocks passou de 0 para 10 páginas de Montanari após OCR: inspeção mostra palavras comuns iguais em blocos separados, não confirmação de duplicação de linha/parágrafo. Umbanda mantém seis páginas com esse sinal, incluindo repetições realmente presentes no original; demais documentos zero. Não surgiu duplicação de conteúdo nativo no merge verificado. Contagens de problemas textuais completos não são calculáveis sem transcrição: as classes e exemplos abaixo são evidência mínima, não um inventário exaustivo de todas as palavras erradas.

Páginas corretas degradadas: nenhuma perda/alteração nova de significado confirmada nos controles comparados. Os 38 resultados de relatório são idênticos; artigo preserva palavras; Umbanda conserva todas as palavras nativas, alterando agrupamento em 4/6/50/53 e retirando somente ruído em 48/50/55. Isso não prova ausência de toda regressão possível. Montanari 12 recebeu novo texto incorreto em uma página anteriormente vazia: é erro introduzido pela recuperação, não degradação de uma página antes correta.

## Falhas encontradas e restantes

| Página/classe | Categoria | Severidade | Sintoma | Causa | Status |
| --- | --- | --- | --- | --- | --- |
| Montanari: 164 históricas | Perda de conteúdo | Crítica | Texto visual sem camada textual | Letras em contornos, decisão só raster | Detecção corrigida; 23 recuperações parciais, 141 pendentes |
| Montanari 12 | Alteração de ordem/significado | Alta | Partes de frases intercaladas no novo texto | Agrupamento/ordem de blocos OCR | Reproduzida e comparada; não corrigida nesta rodada |
| Montanari 3/7/26 | Corrupção/ruído | Alta quando muda palavra/número; cosmética nas marcas | Ordinal, palavra de título, marcas inventadas | Reconhecimento OCR e desenho | Persistente; sem regra específica |
| Montanari 1 | Ruído OCR | Cosmética | Três marcas sem suporte adequado | Bboxes de OCR/margens | Corrigidas; espaços entre palavras ainda ausentes |
| Umbanda 1/3/49/51/52/53/56 | Perda de conteúdo | Alta | Logos/legendas omitidos | Letras raster curtas/claras em imagem complexa | Suspeita adicionada, recuperação pendente |
| Umbanda 50 e 53 | Alteração de ordem | Alta | Palavras da linha após sua continuação | Agrupamento por interseção de x antes de reconstruir baseline | Corrigida; 53 confirma generalização no holdout |
| Umbanda 48/50/55 | Conteúdo espúrio | Moderada; potencial alteração de sentido | Desenho convertido em caracteres | Merge aceita complemento Unicode válido | Cinco linhas rejeitadas, 14 espúrias permanecem |
| Umbanda 4–7 | Perda de estrutura | Alta para consultas | Bibliografia/sumário perdem relações com colunas/números | Agrupamento e ordem de colunas assimétricas | Permanece; conservação de palavras não basta |
| Umbanda 98/121 | Ordem de título | Alta | Título abaixo do conteúdo correspondente | Gutter/ordenação de bloco central curto | Permanece, teste strict xfail preservado |
| Relatórios 6 p4 / 24 p19 | Perda de estrutura | Alta para comparação de conceitos | Células sem associação confiável | Tabela sem grade não reconhecida | Inalterado, sem nova feature nesta rodada |
| Artigo, exemplo p5 | Ordem/estrutura de notas | Moderada | Sobrescrito deslocado da referência | Coordenadas de nota e ordem de bloco | Inalterado, GT preservado |

Não criamos testes específicos por livro. As correções generalizáveis têm reproduções sintéticas mínimas: fragmentos justificados, escala 0,5/1/3, coluna dupla na mesma baseline, texto rotacionado preservado, vetores consolidados, borda sem OCR, falta de suporte visual e orçamento esgotado com detecção mantida. A falha remanescente de OCR da página 12 não foi “resolvida” por warning nem mascarada; está registrada como problema prioritário para uma futura reprodução geral de ordem OCR.

## Performance antes/depois

Uma execução fria por versão/documento, mesmos limites e mesma imagem/dependências (PyMuPDF 1.28.2/Tesseract). Não são medianas nem SLA. Tempo é wall do pedido ao worker, CPU/RSS são do processo filho. O harness intercepta apenas a instrumentação e executa o worker real, com seus limites. Renders são chamadas Python explícitas a `get_pixmap`, incluindo crops de suporte e renderização de tabela; não representam páginas completas e não contam rasterizações internas C do OCR.

| Documento | Wall s antes → depois | s/página antes → depois | Páginas/s antes → depois | CPU s antes → depois | Pico RSS MiB antes → depois | Renders antes → depois | OCRs antes → depois |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Montanari | 26,885 → 80,419 | 0,1554 → 0,4648 | 6,435 → 2,151 | 26,704 → 79,917 | 152,3 → 194,8 | 96 → 929 | 1 → 24 |
| Relatório 6 | 0,605 → 0,569 | 0,1009 → 0,0949 | 9,909 → 10,541 | 0,537 → 0,515 | 61,8 → 61,9 | 0 → 0 | 0 → 0 |
| Relatório 24 | 0,603 → 0,622 | 0,0251 → 0,0259 | 39,833 → 38,599 | 0,563 → 0,549 | 61,6 → 62,4 | 0 → 0 | 0 → 0 |
| Relatório 8 | 0,675 → 0,576 | 0,0844 → 0,0720 | 11,845 → 13,896 | 0,656 → 0,548 | 63,3 → 63,6 | 2 → 2 | 0 → 0 |
| Artigo | 0,549 → 0,612 | 0,0458 → 0,0510 | 21,852 → 19,613 | 0,516 → 0,574 | 62,0 → 61,8 | 0 → 0 | 0 → 0 |
| Umbanda | 1,882 → 2,078 | 0,0153 → 0,0169 | 65,368 → 59,182 | 1,784 → 1,955 | 113,5 → 121,0 | 17 → 37 | 3 → 3 |

Montanari fica aproximadamente 3 vezes mais lento, com +42,5 MiB de pico. OCR em si consumiu 31,706 s versus 0,560 s; o orçamento de 45 s inclui construção e merge/validação do candidato, por isso não equivale à soma do tempo Tesseract isolado. Muitos dos 929 renders são crops de linha. Ainda há custo considerável de leitura de milhares de paths por página. Umbanda cresce cerca de 10,5% em wall, sem OCR extra. Variações pequenas nos relatórios podem ser ruído de uma única amostra.

O orçamento evita executar OCR em todas as páginas do livro nesta requisição, mas não garante uma margem fixa de timeout para PDFs arbitrários: há detecção/custos nativos após a recuperação e um OCR individual não é preemptado pelo orçamento interno. O timeout externo continua necessário. Não há evidência para relaxar os limites nesta rodada.

## Regressões e reprodução

`./scripts/validate.sh` concluiu com exit 0: **86 passed, 1 xfailed**, compileall, testes Go e vet, smoke do backend/extractor, segurança runtime, concorrência, build/test TypeScript e builds das imagens. O xfail estrito é o título central deslocado já conhecido, mantido. Duas reproduções que eram xfail na v1 (vetor e justificação) passaram a testes normais com recuperação/conteúdo verificados. Nenhum teste anterior foi relaxado. Uma warning de depreciação Python é incidental.

Corpus sintético anterior: simple, multipage, columns, table, scan, hybrid, rotated, unicode, margins e complex executados novamente pelo benchmark/audit; testes existentes de uma/duas/três colunas, título, sidebar, listas, rotação, Unicode, scans e merge passaram. Nas dez famílias do benchmark, todas as anchors esperadas ocorreram uma vez e na ordem esperada nas páginas atuais (incluindo as três páginas de multipage e margins); essa cobertura de anchors não é precisão textual de PDF real. O resumo está nas métricas v2. Benchmark sintético do script mantém comparação com baseline pré-hardening, não é a medição v1→v2 dos PDFs desta tabela. A concorrência aceitou 1/8 e rejeitou 7 ocupados; runtime verificou uid não root, filesystem read-only, capabilities zero e limites. Não há alteração relacionada à antiga porta 8000.

O comando ESLint existente do frontend continua falhando por configuração/dependência ausente já documentada, e o script reporta essa falha explicitamente; os checks TypeScript obrigatórios passaram. Não declaramos lint aprovado. Nenhum serviço de produção, merge ou deploy foi feito.

Regressão real: todos os seis PDFs executados antes/depois; todas as diferenças textuais relevantes inspecionadas como descrito no protocolo; ground truths v1 mantidas, novos checks parciais separados; nenhum upgrade de classificação sem evidência. A amostra rotulada não cobre todas as dimensões de fidelidade.

Dados versionados: split, rótulos, métricas por página dos 346 casos, ledger dos 171 e anchors mínimas. Dados privados de reprodução permanecem em `artifacts/real-validation-v2/`: envelopes `before-*.json`/`after-*.json`, reader reflow, análises de camada, comparações PNG, `benchmark.log` e `regression.log`. Não são distribuídos pelo Git. O [harness instrumentado](../scripts/real-validation-v2/run_bench.py) foi preservado sem dados dos livros; requer Docker/imagem com dependências de OCR e os PDFs locais. Para repetir:

```sh
docker build --target test -t pdf-reader-extractor-hardening:latest extractor
python3 scripts/real-validation-v2/run_bench.py
./scripts/validate.sh
```

O harness deduplica por bytes e sobrescreve somente os envelopes privados before/after. Para reproduzir exatamente esta versão do pipeline, use `a08a995` (ou o commit de documentação final, cujo código de engine é idêntico). A medição foi realizada antes de versionar o wrapper, com as mesmas duas funções de instrumentação preservadas. Renderização para inspeção não entra na tabela de custo do pedido de produção.

## Próximos gargalos, por impacto observado

1. **Recuperação fiel e viável de livros com texto em contornos.** 141 páginas continuam vazias no pedido normal; as 23 recuperadas têm erros. O detector resolveu seleção, não a utilidade do livro. Validar ordem do OCR e estratégia de execução dentro dos limites é anterior a elevar confiança.
2. **Ordenação da camada OCR recuperada.** Montanari 12 prova risco de alterar significado mesmo com Unicode limpo e muito texto. A correção de baseline nativa não foi aplicada cegamente a GlyphLessFont: suas caixas/fontes artificiais exigem reprodução independente.
3. **Texto curto/claro em ilustração raster e rejeição de arte.** Sete perdas e 14 linhas falsas remanescentes em Umbanda; suspeita fraca não escolhe recuperação adequada. Melhorar especificidade precisa de outros PDFs/controles, pois 12 falsos alarmes no holdout são relevantes.
4. **Relações de estrutura e ordem global.** Sumários, tabelas sem bordas, títulos centrais e referências de notas continuam prejudicando estudo. Conservação de palavras não resolve associações ou sequência.
5. **Representatividade externa.** Seis PDFs não justificam extrapolar recall observado para outros produtores/fontes/idiomas; novo corpus inédito deve validar thresholds relativos e glyphs/encoding de outras classes sem retunar este holdout.

O objetivo de descobrir por que os casos escapavam foi alcançado com causas verificáveis e testes gerais. O objetivo de leitura confiável dos dois livros permanece não alcançado. As classificações negativas e ground truths que falham foram preservadas.
