<!-- internal/features/patient/problem/README.md -->
# Problemas do paciente — B2

Todas as rotas exigem Bearer token, conta registrada e acesso ao paciente.
A autorização consulta o tipo atual da conta no backend. Apenas contas
`professional` podem criar; `professional` e `basic_care` podem consultar.

| Método | Rota | Resultado |
| --- | --- | --- |
| POST | `/patients/{patientId}/problems` | `201`, problema criado e header `Location` |
| GET | `/patients/{patientId}/problems` | `200`, página de problemas |
| GET | `/patients/{patientId}/problems/{problemId}` | `200`, problema atual |
| GET | `/patients/{patientId}/problems/{problemId}/history` | `200`, página de eventos |

## Criação

`name` é texto livre obrigatório, com espaços nas extremidades removidos.
`classification` é obrigatório: `acute` ou `chronic`.
`cid11` pode ser omitido ou `null`; quando informado exige `code`, `system` e
`version` não vazios. Não há consulta externa ao catálogo CID-11.
Nomes e CIDs repetidos são permitidos.

Exemplo em shell POSIX, com `API_URL`, `TOKEN` e `PATIENT_ID` definidos:

```sh
curl -i -X POST "$API_URL/patients/$PATIENT_ID/problems" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Cefaleia","classification":"acute"}'
```

O servidor define autoria, IDs, datas, versão `1`, situação clínica `active`
e condição administrativa `valid`. Problema e evento `created` são gravados
na mesma transação. Esses campos não podem ser enviados para alterar a criação.

## Consultas e paginação

Listagem e histórico aceitam `limit` (padrão `20`, de `1` a `100`) e `offset`
(padrão `0`, de `0` a `2147483647`). A resposta contém:

```json
{"items":[],"limit":20,"offset":0,"has_more":false}
```

`has_more` indica que existe uma próxima página no momento da consulta.
Para avançar, some `limit` a `offset`. Não há contagem total. Como a paginação
é por deslocamento, alterações concorrentes podem mudar os itens entre páginas.

A lista ordena por `updated_at DESC, id DESC` e oferece:

- `clinical_status`: `all` (padrão), `active` ou `resolved`.
- `administrative_status`: `valid` (padrão), `merged`, `entered_in_error` ou `all`.

```sh
curl "$API_URL/patients/$PATIENT_ID/problems?limit=20&offset=0&clinical_status=active" \
  -H "Authorization: Bearer $TOKEN"

curl "$API_URL/patients/$PATIENT_ID/problems?administrative_status=all" \
  -H "Authorization: Bearer $TOKEN"

curl "$API_URL/patients/$PATIENT_ID/problems/$PROBLEM_ID" \
  -H "Authorization: Bearer $TOKEN"

curl "$API_URL/patients/$PATIENT_ID/problems/$PROBLEM_ID/history?limit=20" \
  -H "Authorization: Bearer $TOKEN"
```

Detalhe e histórico também permitem consultar registros unificados ou
retificados. O histórico ordena por `version DESC`; cada evento contém ação,
autor, data, versão, `before_snapshot`, `after_snapshot`, motivo quando houver
e `source_problem_ids`. Na criação, `before_snapshot` é `null` e as origens
formam uma lista vazia. Snapshots preservam os valores daquele evento.
O histórico desta rota contém apenas os eventos do problema solicitado.

O OpenAPI gerado pelo Huma é a referência completa dos schemas de resposta.

## Erros e implantação

- `400`: corpo JSON malformado.
- `401`: autenticação ausente ou inválida.
- `403`: conta sem acesso ou sem permissão para a ação.
- `404`: problema inexistente ou pertencente a outro paciente, após autorizar
  acesso ao paciente solicitado; vale também para histórico.
- `422`: campos, parâmetros ou regras de criação inválidos.
- `500`: falha técnica, sem detalhes internos na resposta.

B2 usa a migration `supabase/migrations/20261005121819_patient_problems.sql`
de B1.2, que precisa estar aplicada no ambiente de execução. Esta etapa não
introduz migration adicional. As rotas estão conectadas ao bootstrap da API;
edição, resolução, reabertura, retificação e unificação são etapas posteriores.

Testes de persistência usam `PROBLEMS_TEST_DATABASE_URL`, aceitando apenas
PostgreSQL local; criam e removem um schema isolado por teste.
