<!-- docs/architecture/adr/ADR-004-parser-deterministico-exames-laboratoriais.md -->
# ADR-004 - Parser deterministico para exames laboratoriais comuns

**Status:** Substituida pela ADR-005  
**Data:** 2026-09-02  
**Contexto:** Sonnda API - ingestao de exames laboratoriais via rota unificada de exames

---

## Contexto

A rota unificada `POST /v1/patients/:patientId/exames` precisa receber documentos de exames diferentes sem piorar a experiencia do paciente.

Para laudos de imagem, o texto do laudo costuma ser suficiente nesta etapa. Para exames laboratoriais, porem, o dado mais importante nao e o texto corrido: sao os valores de cada analito, suas unidades e referencias.

O pipeline anterior de laboratorios (`POST /v1/patients/:patientId/labs`) conseguia gerar dados estruturados, mas dependia de um recurso mais caro de extracao semantica. Para exames comuns, como hemograma e EAS, o vocabulario e limitado e o formato costuma ser repetitivo. O Tesseract tambem ja entrega um texto relativamente previsivel para muitos desses documentos.

Isso cria uma oportunidade: usar OCR barato seguido de regras explicitas para transformar linhas conhecidas em dados estruturados, mantendo Document AI/LLM apenas como fallback futuro.

---

## Decisao

Adotar um parser deterministico para exames laboratoriais comuns, inicialmente focado em hemograma e depois EAS.

**Revisao em 2026-09-05:** esta decisao foi substituida pela ADR-005. Durante a implementacao inicial, mesmo recortes pequenos como hemograma e glicose/glicemia exigiram muitas regras, aliases e ajustes de layout/OCR. A abordagem se mostrou pouco escalavel para a variedade esperada de exames laboratoriais. O parser deterministico deixa de ser a estrategia principal de extracao laboratorial.

O pipeline alvo passa a ser:

```text
arquivo
  -> OCR barato
  -> texto bruto preservado
  -> pre-processamento textual
  -> classificador de tipo de exame
  -> parser especifico do exame
  -> normalizacao de analitos
  -> validacao conservadora
  -> dados estruturados no banco
```

Para laboratorios, o sistema deve priorizar dados estruturados em vez de exibir apenas `text`.

O texto bruto extraido continua sendo armazenado para auditoria, reprocessamento e fallback de leitura humana. O parser nunca deve depender da posicao fixa de linhas no PDF; ele deve ser baseado em conteudo e sinais textuais.

---

## Regras de desenho

### Preservar entrada original

O sistema deve guardar:

- texto bruto do OCR;
- texto normalizado usado pelo parser, quando necessario;
- linha original de cada resultado parseado.

Isso permite auditoria e melhora incremental do parser.

### Parsear por exame

Nao sera criado um parser generico tentando entender todos os documentos laboratoriais.

Cada tipo comum deve ter um parser proprio:

- `HemogramParser`;
- `UrinalysisParser`;
- outros somente quando houver necessidade real.

### Normalizar analitos por dicionario

Nomes diferentes devem apontar para um codigo interno estavel.

Exemplos:

```text
Hemoglobina, Hb, HGB -> hemoglobin
Hematocrito, Ht, HCT -> hematocrit
Hemacias, Eritrocitos, RBC -> rbc
Leucocitos, WBC -> leukocytes
Plaquetas, PLT -> platelets
```

O nome original tambem deve ser preservado.

### Usar regex pequenas

Regex pode ser usada para partes locais, como uma linha de resultado:

```text
- Hemoglobina 15,1 g/dL (Referencia: 13,5 a 17,5 g/dL)
```

Mas nao deve existir uma regex unica tentando resolver o documento inteiro.

### Referencia tem parser proprio

Referencia deve ser parseada separadamente porque pode aparecer como:

- `13,5 a 17,5 g/dL`;
- `< 200 mg/dL`;
- `Ate 40 U/L`;
- `Negativo`;
- `Ausente`;
- texto longo nao estruturavel com seguranca.

Quando a referencia nao for compreendida com seguranca, o texto deve ser preservado como `raw_reference`.

### Corrigir OCR apenas em contexto numerico

Erros como `15,l` ou `453.OOO` podem ser tratados somente quando o parser ja sabe que espera um numero.

Nao deve existir normalizacao global como `O -> 0` ou `l -> 1`, pois isso pode corromper palavras clinicas.

### Validar plausibilidade sem diagnosticar

O parser pode marcar valores como suspeitos quando estiverem fora de faixas amplas de plausibilidade tecnica.

Exemplo:

```text
Hemoglobina = 151 g/dL
```

deve ser marcado para revisao, nao salvo silenciosamente como dado confiavel.

### Resultado parcial e aceitavel

Falha em uma linha nao deve invalidar o documento inteiro.

Cada item parseado deve ter status:

- `parsed`: interpretado com seguranca;
- `partial`: analito ou valor identificado, mas alguma parte ficou incompleta;
- `unparsed`: linha preservada, mas nao interpretada.

---

## Modelo conceitual de resultado

Um item laboratorial parseado deve carregar, no minimo:

```go
type ParsedPanel struct {
    Code          string
    OriginalName  string
    Value         *float64
    RawValue      string
    Unit          *string
    ReferenceMin  *float64
    ReferenceMax  *float64
    RawReference  *string
    Status        ParseStatus
    RawLine       string
}
```

O modelo final do banco pode evoluir em outra decisao, mas a regra arquitetural e clara: o dado estruturado e a fonte principal para laboratorio; o texto e apoio.

---

## Alternativas consideradas

### Usar Document AI/LLM para todos os exames laboratoriais

Rejeitado como caminho principal nesta etapa.

Apesar de ser mais flexivel, aumenta custo, latencia e dificuldade de auditoria. Continua sendo opcao futura para fallback quando OCR + parser deterministico falharem.

### Salvar apenas texto corrido em `exam_document_texts`

Rejeitado para laboratorio.

Texto corrido e aceitavel para muitos laudos de imagem, mas em laboratorio o usuario precisa comparar valores, unidades e referencias.

### Criar um parser generico para qualquer exame laboratorial

Rejeitado.

Exames laboratoriais variam muito. Comecar por hemograma e EAS reduz escopo, permite testes com fixtures reais e evita falsa confianca.

### Parser baseado em posicao fixa no PDF

Rejeitado.

Laboratorios mudam layout com frequencia. O parser deve reconhecer conteudo, sinonimos e padroes locais, nao depender de "linha 8 significa hemoglobina".

---

## Consequencias

### Positivas

- Menor custo por upload para exames comuns.
- Resultado mais rapido e auditavel.
- Mesma entrada sempre gera a mesma saida.
- Facilidade para criar testes com fixtures reais de OCR.
- Melhor experiencia para exames laboratoriais, pois valores aparecem estruturados.

### Negativas / trade-offs

- Requer manutencao de dicionarios de aliases.
- Novos laboratorios podem quebrar regras existentes.
- Casos ambiguos precisam ser marcados como `partial` ou `unparsed`.
- O fallback caro ainda sera necessario para documentos fora do padrao.

---

## Plano de evolucao

1. Criar fixtures de OCR para hemograma.
2. Implementar pre-processamento conservador de texto.
3. Implementar deteccao de hemograma.
4. Implementar dicionario de aliases de hemograma.
5. Implementar parser de linhas e state machine para referencias quebradas em multiplas linhas.
6. Implementar validacao de plausibilidade ampla.
7. Persistir status por item: `parsed`, `partial`, `unparsed`.
8. Exibir no Flutter os valores estruturados como fonte principal para laboratorio.
9. Adicionar EAS seguindo o mesmo padrao.
10. Avaliar fallback com Document AI/LLM apenas para casos sem parse suficiente.

---

## Criterios de aceitacao

- Um hemograma comum deve produzir itens estruturados para hemoglobina, hematocrito, hemacias, leucocitos e plaquetas.
- Linhas parcialmente compreendidas nao devem ser descartadas.
- O texto bruto deve continuar disponivel.
- O parser deve ter testes com fixtures reais de OCR.
- Nenhum valor deve ser corrigido automaticamente fora de contexto numerico.
- Valores tecnicamente suspeitos devem ser marcados para revisao.
