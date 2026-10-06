<!-- docs/architecture/adr/ADR-005-extracao-semantica-exames-laboratoriais.md -->
# ADR-005 - Extracao semantica para exames laboratoriais

**Status:** Aceito  
**Data:** 2026-09-05  
**Contexto:** Sonnda API - extracao estruturada de exames laboratoriais

---

## Contexto

A aplicacao precisa receber exames laboratoriais e disponibilizar os dados mais importantes para o paciente: nome do exame, analitos, valores, unidades, datas e, quando possivel, referencias.

A ADR-004 propunha parser deterministico para exames laboratoriais comuns. A implementacao inicial mostrou que mesmo casos aparentemente simples, como hemograma e glicose/glicemia, exigem muitas regras especificas, aliases, tratamento de layout e excecoes de OCR.

Essa complexidade tende a crescer muito com a variedade real de exames laboratoriais:

- lipidograma;
- hormonios;
- vitaminas;
- enzimas hepaticas;
- urina/EAS;
- culturas;
- sorologias;
- exames com resultados qualitativos;
- diferentes laboratorios e layouts.

Manter um parser deterministico por exame como estrategia principal criaria alto custo de manutencao e risco de codigo morto ou incompleto.

---

## Decisao

Usar extracao semantica estruturada como estrategia principal para exames laboratoriais.

O pipeline alvo passa a ser:

```text
arquivo
  -> upload seguro
  -> extracao de texto/OCR quando aplicavel
  -> extrator semantico com schema estruturado
  -> normalizacao deterministica leve
  -> validacao conservadora
  -> persistencia dos resultados laboratoriais
  -> exibicao estruturada no cliente
```

O extrator semantico pode ser Document AI, LLM com schema ou outro servico equivalente. A decisao arquitetural nao fica presa a um fornecedor especifico; o contrato interno deve ser um schema de saida estavel.

Parsers deterministos nao serao usados como estrategia principal. Eles podem existir apenas como utilitarios pequenos e comprovadamente vantajosos, por exemplo:

- normalizar numero brasileiro;
- validar unidade;
- validar plausibilidade tecnica ampla;
- limpar texto bruto;
- checar consistencia de campos ja extraidos.

---

## Regras de desenho

### Schema estruturado como contrato interno

A saida esperada da extracao laboratorial deve seguir um contrato do tipo:

```go
type ExtractedLabReport struct {
    PatientName      *string
    LabName          *string
    ReportDate       *string
    RawText          *string
    Tests            []ExtractedLabTest
}

type ExtractedLabTest struct {
    TestName     string
    Material     *string
    Method       *string
    CollectedAt  *string
    ReleaseAt    *string
    Items        []ExtractedLabItem
}

type ExtractedLabItem struct {
    ParameterName string
    ResultValue   *string
    ResultUnit    *string
    ReferenceText *string
}
```

O schema pode evoluir, mas deve continuar priorizando dados estruturados em vez de texto corrido.

### Texto bruto continua sendo preservado

Mesmo usando extracao semantica, o texto bruto deve ser mantido quando disponivel.

Ele serve para:

- auditoria;
- debug de extracao;
- reprocessamento futuro;
- fallback de leitura humana.

### Validacao deterministica apos extracao

Regras deterministicas continuam uteis depois da extracao, mas como camada de controle, nao como parser principal.

Exemplos:

- converter `15,1` para `15.1` quando necessario;
- marcar resultado suspeito quando um valor estiver fora de faixa tecnica ampla;
- detectar item sem unidade quando unidade for esperada;
- preservar item como pendente de revisao quando a confianca for baixa.

### Falha parcial e aceitavel

Falhas em alguns itens nao devem invalidar o documento inteiro.

O sistema deve aceitar estados como:

- extraido com sucesso;
- extraido parcialmente;
- precisa revisao;
- falhou.

### Evitar codigo morto

Implementacoes experimentais que nao forem integradas ao fluxo principal devem ser removidas ou movidas explicitamente para uma area experimental documentada.

Para esta decisao, o pacote `internal/domain/labparser` deve ser removido do runtime principal.

---

## Alternativas consideradas

### Parser deterministico por exame

Substituido.

Era atraente por custo baixo e auditabilidade, mas mostrou alto custo de manutencao e baixa escalabilidade diante da variedade real de exames.

### Salvar apenas texto corrido

Rejeitado para laboratorio.

Texto corrido pode ser suficiente para alguns laudos de imagem, mas exames laboratoriais precisam de valores estruturados para comparacao e acompanhamento.

### Usar extracao semantica sem validacao

Rejeitado.

Mesmo com extrator semantico, a aplicacao deve preservar texto bruto e aplicar validacoes conservadoras para reduzir erro silencioso.

---

## Consequencias

### Positivas

- Escala melhor para muitos tipos de exames.
- Reduz manutencao manual de regex e aliases por exame.
- Mantem a experiencia principal: valores laboratoriais estruturados.
- Permite trocar fornecedor de extracao mantendo contrato interno.
- Mantem validacao deterministica onde ela agrega valor.

### Negativas / trade-offs

- Maior custo por processamento em comparacao com parser local.
- Dependencia de servico externo ou modelo semantico.
- Exige bons testes de contrato e observabilidade para avaliar qualidade.
- Pode precisar de fila/reprocessamento quando o servico externo falhar.

---

## Plano de evolucao

O plano original estabeleceu a base: remover o parser por exame, consolidar o
contrato de saida, compartilhar a persistencia laboratorial entre as rotas e
preservar os vinculos com os documentos. A troca do fornecedor ainda nao foi
implementada. O complemento abaixo detalha essa etapa e reorganiza as seguintes.

## Complemento de decisao - 2026-09-05

### Decisao mantida e implementacao escolhida

A decisao de usar extracao semantica continua aceita. Este complemento especifica
uma LLM que recebe texto como implementacao alvo, no lugar do Document AI que
processa o arquivo original. Nao substitui esta ADR nem retoma o parser deterministico.

Gemini 3.5 Flash-Lite e o candidato inicial para avaliacao. O modelo deve ser
configuravel, e sua adocao depende de confirmar disponibilidade, suporte ao schema
e qualidade nos exames de teste durante a implementacao. Precos e taxas estimadas
de acerto nao sao garantias nem criterios ja comprovados por esta decisao.

Nao havera fallback automatico para Document AI ou para um modelo mais caro nesta
primeira versao. A selecao do fornecedor fica no bootstrap; dominio e persistencia
continuam independentes do SDK escolhido.

### Estado atual e pipeline alvo

Na data deste complemento, o runtime ainda usa Document AI. O texto local serve
para classificar o documento, mas nao e a entrada do extrator laboratorial.
As transacoes e o tratamento de vinculos ja foram ajustados no codigo; isso nao
significa que a nova LLM esteja integrada ou que dados antigos tenham sido reparados.

O pipeline alvo para o upload unificado e:

```text
Flutter: POST /v1/patients/:patientId/exames, multipart com campo file
  -> salvar arquivo e exam_documents
  -> extrair texto localmente
       PDF nativo: pdftotext
       JPEG/PNG: Tesseract
  -> preservar texto original em exam_documents.extracted_text
  -> classificar documento
       laboratorio:
         texto -> LLM -> JSON conforme schema -> validacao em Go
         -> lab_reports + lab_panels + observations em transacao
         -> texto de apoio em exam_document_texts
       imagem ou desconhecido:
         -> texto disponivel em exam_document_texts
  -> cliente prioriza resultados estruturados quando existirem
```

A rota `/labs` continua suportada e prepara o texto antes de chamar o mesmo
extrator. Ela ja indica laboratorio e nao precisa repetir a classificacao.
O texto deve ser extraido uma vez por requisicao e reutilizado.

PDF escaneado nao e equivalente a PDF com texto nativo. O extrator atual nao
converte paginas em imagens para OCR. Esse suporte fica para uma etapa posterior:
exigira renderizacao das paginas, limites de recursos e testes de ordem de leitura.
A primeira versao deve informar quando nao conseguir obter texto utilizavel;
classificar pelo nome do arquivo nao supre a entrada exigida pela LLM.

### Escopo inicial do contrato

Reutilizar `ExtractedLabReport`, seus exames e itens, sem criar um schema por analito.
Os campos prioritarios sao:

- `TestName`: nome do exame ou painel encontrado no documento;
- `ParameterName`: nome original do analito;
- `ResultValue`: valor como texto, preservando decimais, comparadores e resultados qualitativos;
- `ResultUnit`: unidade quando constar no documento, ou nulo quando ausente.

Exemplos de valores validos como texto sao `15,1`, `453.000`, `< 5` e `Negativo`.
Nao converter automaticamente unidades nem calcular valores ausentes. No leucograma,
preservar a diferenca entre resultado percentual e absoluto, sem tratar unidades
distintas como duplicatas apenas porque o nome do analito coincide.

Referencias, nomes canonicos, faixas numericas e interpretacao clinica nao sao
requisitos desta primeira versao. Campos opcionais existentes permanecem compativeis;
nao precisam ser removidos nem preenchidos por suposicao. Datas e outros metadados
so devem ser preenchidos quando explicitamente disponiveis.

O texto original vem do extrator local, nao de uma reescrita da LLM.
`exam_document_texts.text` pode conter uma apresentacao montada a partir dos itens;
esse texto de apoio nao deve ser apresentado como se fosse a transcricao original.
Nao e necessaria uma migration apenas para trocar o fornecedor e preservar esse contrato.

### Limites e validacao obrigatoria

Saida conforme JSON Schema garante formato, nao que os valores correspondem ao exame.
Antes de persistir, o backend deve validar o JSON, a estrutura e a presenca de itens
utilizaveis. Uma resposta vazia, invalida ou truncada nao equivale a sucesso.

O prompt deve tratar o conteudo do documento como dados, nunca como instrucoes.
Nao deve inventar analitos, valores ou unidades para preencher o schema.
Definir timeout, limite de entrada e de saida; nao cortar silenciosamente paginas
ou linhas para fazer o texto caber. Textos sem associacao clara entre analito e
valor devem ser sinalizados para revisao, nao corrigidos por suposicao.

Usar fixtures sinteticas ou anonimizadas na avaliacao. Nao registrar texto clinico,
identificadores do paciente, prompts completos ou respostas completas em logs comuns.
Antes do uso com dados reais, verificar a configuracao de tratamento de dados do
fornecedor e reduzir identificacao desnecessaria no texto enviado. Uma limpeza
simples nao deve ser considerada garantia de anonimizacao.

## Implementacao por etapas

Esta numeracao pertence ao novo plano da LLM. Cada etapa deve ser entregue para
revisao separadamente; a atualizacao da ADR nao implementa essas etapas.

### Etapa 1 - Contrato de entrada por texto

**Objetivo:** definir o que o novo extrator recebe e devolve.

- Definir a entrada textual em `internal/domain/labextraction`, sem tipos do SDK.
- Reutilizar a saida `ExtractedLabReport` e documentar campos opcionais e valores textuais.
- Definir o schema de resposta do fornecedor a partir desse contrato.
- Criar fixtures pequenas para glicose, hemograma e resultado qualitativo.
- Preparar a transicao sem quebrar o runtime atual; a substituicao da assinatura
  baseada em URI e a remocao da compatibilidade temporaria terminam na etapa 4.

**Concluida quando:** contrato, exemplos e testes de formato puderem ser revisados
sem credenciais, chamadas externas ou mudancas no banco.

**Entrega da etapa 1 (2026-09-05):** entrada `ExtractLabReportInput`, interface
`LabReportTextExtractor`, schema JSON v1 e fixtures sinteticas implementados em
`internal/domain/labextraction`. O README do pacote documenta a correspondencia
com o banco e a utilizacao de `samples/`. Os testes validam o contrato e as saidas
esperadas; avaliacao semantica dos arquivos locais e integracao com LLM seguem pendentes.

### Etapa 2 - Implementacao do extrator Gemini

**Objetivo:** transformar texto em `ExtractedLabReport` usando o modelo candidato.

- Criar o adaptador em infraestrutura, com modelo configuravel e cliente testavel.
- Definir prompt, schema, timeout e limites de entrada/saida.
- Mapear a resposta para o contrato existente e aplicar a validacao obrigatoria.
- Tratar indisponibilidade, bloqueio, resposta vazia, JSON invalido e truncamento.
- Testar o adaptador com respostas simuladas e avaliar fixtures anonimizadas no
  modelo real antes de torna-lo padrao.

**Concluida quando:** os testes simulados passam e a avaliacao real compara nomes,
valores, unidades, omissoes e itens inventados com resultados esperados. Registrar
divergencias, latencia e consumo observado; nao aceitar apenas JSON bem formado.
Sem credenciais ou avaliacao real, a etapa permanece pendente de homologacao.

**Entrega 2.1 (2026-09-05):** cliente de transporte em `internal/infrastructure/gemini`
e configuracao `cfg.Gemini` implementados, com modelo, timeout, limite de entrada
em bytes, limite de saida em tokens e gerador substituivel nos testes. Nenhuma rota
foi alterada. O adaptador que implementara `LabReportTextExtractor`, o prompt e a
validacao da resposta pertencem a entrega 2.2; a avaliacao real, a entrega 2.3.

**Entrega 2.2 (2026-09-05):** adaptador `LabReportTextExtractor` implementado em
`internal/infrastructure/gemini`, recebendo texto, enviando prompt e schema ao
cliente Gemini, validando a resposta com o schema local e convertendo para
`ExtractedLabReport`. O schema enviado ao provedor remove palavras-chave locais
nao suportadas, enquanto a validacao final permanece no backend. O adaptador trata
erro do cliente, ausencia de candidato, bloqueio, truncamento, texto vazio, JSON
invalido e JSON fora do schema. Resposta valida sem itens estruturados retorna
`needs_review` com aviso. Nenhuma rota foi alterada; avaliacao real segue na 2.3.

**Entrega parcial 2.3 (2026-09-05):** comando local `cmd/lab-extraction-eval`
criado para avaliar o Gemini real a partir de texto ja extraido. O comando carrega
somente a configuracao do Gemini, chama `LabReportTextExtractor`, imprime o JSON
extraido e opcionalmente compara com um `.expected.json`. Ele nao altera banco,
rotas ou arquivos enviados pelo app. A homologacao da etapa 2.3 ainda exige rodar
amostras sinteticas ou anonimizadas com credencial real e registrar divergencias,
latencia e consumo observado.

**Ajuste de custo da etapa 2.3 (2026-09-06):** a avaliacao local passou a usar o
classificador heuristico antes de chamar Gemini. Documentos classificados como
`imaging` ou `unknown`, como laudos de ultrassonografia, atestados e pedidos sem
evidencia de resultado laboratorial, pulam a chamada ao fornecedor. A flag
`-force-lab` permite chamar o extrator manualmente para investigar o prompt, mas
nao deve representar o fluxo padrao. O prompt tambem foi reforcado para retornar
`tests: []` quando documentos nao laboratoriais forem enviados por engano.

### Etapa 3 - Preparacao e preservacao do texto

**Objetivo:** fornecer texto utilizavel ao extrator sem perder a origem.

- Reutilizar `internal/domain/textextraction` e a implementacao local existente.
- Preparar um fluxo comum para PDF nativo e JPEG/PNG, preservando paginas e linhas
  quando disponiveis; avaliar a ordem de colunas nas fixtures laboratoriais.
- Separar texto original preservado da entrada preparada para a LLM.
- Tratar texto vazio, extracao local indisponivel, limite excedido e PDF escaneado
  sem suporte como situacoes explicitas, sem acionar fallback caro.

**Concluida quando:** fixtures de PDF nativo e imagem geram entrada utilizavel;
arquivos sem texto nao chegam a LLM como se a preparacao tivesse funcionado.

### Etapa 4 - Integracao nas duas rotas e retirada do Document AI

**Objetivo:** ativar a LLM no fluxo real de laboratorio.

- Ajustar o caso de uso para receber texto em vez da URI usada pelo Document AI.
- Em `/exames`, reutilizar o texto ja extraido quando a classificacao for laboratorio.
- Em `/labs`, preparar o texto e chamar o mesmo caso de uso.
- Configurar o adaptador no bootstrap e documentar as variaveis de ambiente.
- Preservar transacao, vinculos, deduplicacao e conflitos de documento existentes.
- Remover o adaptador Document AI, configuracoes e dependencias exclusivas dele
  depois de verificar todos os consumidores. Manter dependencias usadas pelo storage.
- Remover contratos temporarios e atualizar documentacao que descreve o runtime.

**Concluida quando:** ambas as rotas geram resultados estruturados com a nova
implementacao; testes cobrem sucesso, falha, duplicado e laudo antigo sem vinculo.
Nao ha chamadas nem configuracoes obrigatorias residuais de Document AI.

### Etapa 5 - Resposta e listagem estruturada

**Objetivo:** permitir que o cliente encontre os resultados pelo documento.

- Expor o laboratorio vinculado e seus itens na consulta usada pelo app.
- Priorizar resultados estruturados quando existirem, mantendo texto como apoio.
- Distinguir texto original, texto de apoio e ausencia de resultados.
- Atualizar o contrato HTTP e testar documentos laboratoriais, de imagem e sem extracao.

**Concluida quando:** o cliente consegue identificar o laboratorio e consultar seus
valores pelo vinculo, sem interpretar o texto formatado.

### Etapa 6 - Detalhe laboratorial no Flutter

**Objetivo:** mostrar primeiro os valores ao paciente.

- Exibir analito, valor e unidade agrupados por exame/painel.
- Oferecer acesso separado ao texto original quando disponivel.
- Atualizar a listagem estruturada depois do upload.
- Tratar estados de carregamento, falha, revisao e conflito de duplicidade.

**Concluida quando:** um upload de teste aparece com valores estruturados, e o app
nao mostra o texto de apoio como fonte principal dos resultados.

### Etapa 7 - Validacoes leves e revisao

**Objetivo:** ampliar os sinais de qualidade alem da validacao obrigatoria inicial.

- Detectar valor vazio, unidade ausente quando esperada e duplicacao contextual.
- Preservar itens utilizaveis quando outros estiverem incompletos.
- Marcar `partial` ou `needs_review` conforme o contrato, sem corrigir valores
  automaticamente. Nao usar confianca declarada pelo modelo como prova de exatidao.
- Verificar como avisos e estados serao persistidos e expostos; criar migration
  pelo fluxo oficial somente se faltarem campos necessarios.

**Concluida quando:** resultados parciais e pendencias ficam visiveis ao cliente,
sem confundir falta de dados com um resultado normal ou com sucesso completo.

### Etapa 8 - Observabilidade e reprocessamento

**Objetivo:** acompanhar qualidade/custo e permitir novas tentativas controladas.

- Registrar fornecedor, modelo, versao de prompt/schema, duracao, consumo informado
  pelo fornecedor e resultado do processamento, sem conteudo clinico nos logs.
- Distinguir falha de extracao de texto, falha da LLM, ausencia de itens e revisao.
- Definir reprocessamento a partir do arquivo/texto preservado, com idempotencia
  e politica explicita para atualizar resultados sem duplicar ou perder vinculos.
- Manter fallback para outro modelo como decisao futura, fora deste plano inicial.

**Concluida quando:** uma falha pode ser localizada e reprocessada com rastreabilidade,
sem criar laudos duplicados nem sobrescrever silenciosamente resultados anteriores.


---

## Complemento Historico — Reorganizacao da Feature `documentprocessing`

Como evolucao das decisoes registradas nesta ADR e complementadas pela ADR-006:

1. **Consolidacao na feature:** Os contratos antes dispersos em `internal/domain/textextraction` e `internal/domain/labextraction` foram reorganizados para dentro de `internal/features/documentprocessing`:
   - `internal/features/documentprocessing/textextraction`: contrato de leitura, qualidade e normalizacao de texto.
   - `internal/features/documentprocessing/labextraction`: contrato, tipos e schema da extracao estruturada.
   - `internal/features/documentprocessing/extraction`: coordenacao, normalizacao de entrada semantica, avaliacao e geracao de resumo.
2. **Infraestrutura:** O adaptador do Gemini foi movido para `internal/infrastructure/gemini`.
3. **Persistencia e Snapshot:** A fotografia versionada da extracao reside na raiz de `documentprocessing` (`snapshot.go`), mantendo a extracao livre de dependencias de persistencia.
4. **Remocao de execucao via terminal:** Utilitarios CLI de extracao textual e laboratorial foram descontinuados e removidos para prevenir usos descontrolados, restringindo o processamento aos fluxos autenticados da API (extracao temporaria e rascunhos de exames).
