<!-- internal/features/patient/problem/README.md -->
# Problemas do paciente — B2 e B3

Todas as rotas exigem Bearer token, conta registrada e acesso ao paciente.
A autorização consulta o tipo atual da conta no backend. Apenas contas
`professional` podem criar; `professional` e `basic_care` podem consultar.
Edição, classificação e reabertura exigem `professional`. Qualquer conta
registrada com acesso pode resolver um problema agudo ativo.

| Método | Rota | Resultado |
| --- | --- | --- |
| POST | `/patients/{patientId}/problems` | `201`, problema criado e header `Location` |
| GET | `/patients/{patientId}/problems` | `200`, página de problemas |
| GET | `/patients/{patientId}/problems/{problemId}` | `200`, problema atual |
| GET | `/patients/{patientId}/problems/{problemId}/history` | `200`, página de eventos |
| PUT | `/patients/{patientId}/problems/{problemId}` | `200`, nome/CID substituídos |
| PUT | `/patients/{patientId}/problems/{problemId}/classification` | `200`, classificação alterada |
| POST | `/patients/{patientId}/problems/{problemId}/resolve` | `200`, problema agudo resolvido |
| POST | `/patients/{patientId}/problems/{problemId}/reopen` | `200`, problema reaberto |

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

## Alterações e controle de versão

Cada operação exige `version` inteiro positivo, igual ao valor obtido no
detalhe ou na última alteração bem-sucedida. A resposta contém o problema
atualizado, com o mesmo ID, autoria e data de criação, e a versão incrementada.
O evento registra a conta autenticada autora, paciente, ação, data e snapshots
anteriores/posteriores. Não envie autoria ou outros campos fora do contrato.

Exemplos de payloads, usando a versão atual em cada chamada:

| Operação | Payload |
| --- | --- |
| Editar nome e remover CID | `{"version":1,"name":"Cefaleia recorrente","cid11":null}` |
| Editar nome e informar CID | `{"version":1,"name":"Hipertensão","cid11":{"code":"BA00","system":"ICD-11","version":"2026"}}` |
| Classificar como crônico | `{"version":2,"classification":"chronic"}` |
| Resolver | `{"version":2}` |
| Reabrir | `{"version":3}` |

`PUT` no problema substitui os detalhes: `name` é obrigatório e `cid11` omitido
ou `null` remove o código anterior. Para manter um CID existente, envie o objeto
completo. Nome e CID são validados juntos e gravados em um único evento `edited`.
Situação clínica e classificação não são campos aceitos nessa operação.
Classificação usa evento `classified`; resolução e reabertura usam `resolved`
e `reopened`.

Somente `acute` + `active` permite resolução, inclusive para profissionais.
Para mudar um problema resolvido para `chronic`, um profissional deve primeiro
reabri-lo e depois classificar usando a nova versão. As operações são separadas;
payloads que tentam combinar classificação e reabertura são rejeitados.
Registros `merged` e `entered_in_error` não aceitam essas alterações.
Pedidos sem mudança efetiva e transições incompatíveis retornam `422` sem
incrementar versão nem adicionar histórico.

O backend consulta o estado atual, valida o estado final e grava problema e
auditoria na mesma transação, condicionando a atualização à versão esperada.
Se uma classificação concorrente vencer uma resolução, a resolução retorna
`409` sem sobrescrever a classificação nem adicionar evento. O inverso também
retorna `409`. Após um conflito, consulte novamente e avalie a operação com os
novos dados; não repita automaticamente usando uma versão atualizada. Repetir
uma chamada bem-sucedida com a versão antiga também retorna `409`.

## Erros e implantação

- `400`: corpo JSON malformado.
- `401`: autenticação ausente ou inválida.
- `403`: conta sem acesso ou sem permissão para a ação.
- `404`: problema inexistente ou pertencente a outro paciente, após autorizar
  acesso ao paciente solicitado; vale também para histórico.
- `409`: versão desatualizada, inclusive alteração concorrente durante a operação.
- `422`: campos, parâmetros ou regras clínicas inválidos; alteração sem mudanças.
- `500`: falha técnica, sem detalhes internos na resposta.

B2 e B3 usam a migration `supabase/migrations/20261005121819_patient_problems.sql`
de B1.2, que precisa estar aplicada no ambiente de execução. Esta etapa não
introduz migration adicional. As rotas estão conectadas ao bootstrap da API;
retificação e unificação são etapas posteriores.

Testes de persistência usam `PROBLEMS_TEST_DATABASE_URL`, aceitando apenas
PostgreSQL local; criam e removem um schema isolado por teste.
Os testes de concorrência sincronizam duas leituras da mesma versão antes de
liberar as atualizações, cobrindo resolução versus classificação e duas
resoluções simultâneas, conflito, autoria e ausência de auditoria da operação
rejeitada. Sem a variável, esses testes são ignorados pelo Go.
