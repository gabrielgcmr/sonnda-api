<!-- internal/infrastructure/gemini/README.md -->
# Gemini - Processamento de documentos

Cliente de transporte baseado em `google.golang.org/genai`.
Recebe texto e, opcionalmente, um PDF, JPEG ou PNG inline. Devolve
`genai.GenerateContentResponse`, preservando candidatos, motivo de término e
consumo para os adaptadores de extração.

## Configuração

`config.Load()` disponibiliza `cfg.Gemini`.
`NewClient` exige chave explícita (`GEMINI_API_KEY`).
O cliente seleciona explicitamente `BackendGeminiAPI`, sem alternar para Vertex AI.

| Variável | Padrão | Finalidade |
| --- | --- | --- |
| `GEMINI_API_KEY` | Vazio | Credencial somente no backend |
| `GEMINI_MODEL` | `gemini-3.5-flash-lite` | Modelo configurável |
| `GEMINI_TIMEOUT` | `30s` | Prazo por chamada, no formato de duração Go |
| `GEMINI_MAX_INPUT_BYTES` | `131072` | Limite de bytes UTF-8 de entrada |
| `GEMINI_MAX_MEDIA_BYTES` | `10485760` | Limite de bytes de uma mídia inline |
| `GEMINI_MAX_OUTPUT_TOKENS` | `8192` | Limite de geração enviado ao SDK |

Um deadline menor da requisição é respeitado. Não há retry automático nem fallback.

## Cliente

Criar `NewClient(ctx, cfg.Gemini)` uma vez e reutilizar o cliente.
O método `Generate` aceita `GenerateRequest` com `Text`, `Media`,
`SystemInstruction` e `JSONSchema` opcional. A mídia tem limite próprio e sua
assinatura precisa corresponder ao MIME type declarado. Quando há schema,
configura `application/json` na resposta.

## Extrator laboratorial

`NewLabReportExtractor(client)` implementa `labextraction.LabReportExtractor`.
Ele recebe texto ou mídia em `ExtractLabReportInput`, chama o cliente Gemini com prompt
específico para exames laboratoriais e valida a resposta contra o schema local de
`internal/features/documentprocessing/labextraction`.

O prompt tem uma barreira para retornar `tests: []` em laudos de imagem, atestados,
pedidos, receitas e prescrições.

O adaptador trata:
- entrada vazia;
- erro do cliente;
- resposta sem candidato;
- resposta bloqueada;
- resposta truncada por limite de tokens;
- texto vazio;
- JSON inválido;
- JSON fora do schema;
- resposta válida sem itens, devolvida sem avaliação de qualidade pelo adaptador.

O serviço `documentprocessing/extraction` prepara o texto semântico ou encaminha a mídia, preserva o
texto original e aplica normalização, avisos e estados (`needs_review`, `partial`
ou `succeeded`). O adaptador valida a resposta e informa fornecedor/modelo.

Não registrar credenciais, texto clínico ou respostas completas nos logs comuns.

## Testes

`NewClientWithGenerator` recebe a interface `ContentGenerator`. Os testes injetam
uma implementação simulada, sem credenciais reais nem acesso a rede.

```powershell
go test ./internal/config ./internal/infrastructure/gemini -v
```
