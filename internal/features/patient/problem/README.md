<!-- internal/features/patient/problem/README.md -->
# Problemas do paciente — B2, B3, B4 e B5

Todas as rotas exigem Bearer token, conta resolvida com onboarding concluído e acesso ao paciente.
A autorização consulta o tipo atual da conta no backend. Apenas contas
`professional` podem criar; `professional` e `basic_care` podem consultar.
Edição, classificação, reabertura, retificação e unificação exigem `professional`. Qualquer conta
registrada com acesso pode resolver um problema agudo ativo.

A feature Ã© dona de `Action`, `RequireAction` e `Authorizer`. O pacote compartilhado
`internal/features/authz` fornece apenas `PatientContext` e `PatientContextResolver`,
isto Ã©, os fatos confiÃ¡veis de conta e acesso usados pela policy local.

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
| POST | `/patients/{patientId}/problems/{problemId}/rectify` | `200`, registro retificado por engano |
| POST | `/patients/{patientId}/problems/{problemId}/merge` | `200`, destino consolidado e origens unificadas |

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
| Retificar | `{"version":3,"reason":"Registro criado para o paciente errado"}` |

`PUT` no problema substitui os detalhes: `name` é obrigatório e `cid11` omitido
ou `null` remove o código anterior. Para manter um CID existente, envie o objeto
completo. Nome e CID são validados juntos e gravados em um único evento `edited`.
Situação clínica e classificação não são campos aceitos nessa operação.
Classificação usa evento `classified`; resolução e reabertura usam `resolved`
e `reopened`. Retificação usa `rectified`, exige motivo não vazio e altera apenas
`administrative_status` para `entered_in_error`. Nome, CID, classificação e
situação clínica são preservados.

Somente `acute` + `active` permite resolução, inclusive para profissionais.
Para mudar um problema resolvido para `chronic`, um profissional deve primeiro
reabri-lo e depois classificar usando a nova versão. As operações são separadas;
payloads que tentam combinar classificação e reabertura são rejeitados.
Registros `merged` e `entered_in_error` não aceitam essas alterações.
Pedidos sem mudança efetiva e transições incompatíveis retornam `422` sem
incrementar versão nem adicionar histórico.

A retificação é exclusiva de profissionais com acesso ativo ao paciente. O
registro deixa de aparecer na listagem padrão, permanece disponível nas consultas
com `administrative_status=entered_in_error` ou `all`, e conserva todo o conteúdo
e histórico. A versão esperada evita que a retificação sobrescreva uma alteração
concorrente.

## Unificação manual

`POST /patients/{patientId}/problems/{problemId}/merge` usa o problema da rota
como destino e exige um profissional com acesso ativo. Destino e origens devem
pertencer ao mesmo paciente e estar com condição administrativa `valid`.

O payload informa a versão atual do destino, ao menos uma origem com sua versão
atual e todas as escolhas clínicas finais. `cid11` é obrigatório: envie o objeto
completo para escolher um código ou `null` para escolher explicitamente não usar
CID. Omissão não significa remoção e retorna `422`.

```json
{
  "version": 2,
  "sources": [
    {"id": "7ab49c75-e1ea-4bbd-883f-7d45d7605cad", "version": 1},
    {"id": "6d581aa6-46d0-47d3-9916-38061d1d16e9", "version": 3}
  ],
  "name": "Hipertensão arterial",
  "cid11": {"code": "BA00", "system": "ICD-11", "version": "2026"},
  "classification": "chronic",
  "clinical_status": "active"
}
```

Nome, CID, classificação e situação clínica são escolhas explícitas, mesmo
quando coincidem com algum registro. A combinação `chronic` + `resolved` é
inválida. Auto-unificação, origens repetidas, versões inválidas e registros
terminais também são rejeitados.

O destino permanece `valid` e recebe um evento `merged_destination` com todos os
IDs em `source_problem_ids`. Cada origem preserva seus campos clínicos, passa a
`merged`, recebe `merged_into_id` e um evento `merged_source`. Todos os registros
e eventos são gravados em uma única transação. Qualquer conflito retorna `409` e
reverte o conjunto inteiro. Os IDs e históricos permanecem consultáveis. Em
unificações sucessivas, os eventos permitem percorrer a cadeia de origens sem
copiar ou apagar o histórico anterior.

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

B2, B3, B4 e B5 usam a migration `supabase/migrations/20261005121819_patient_problems.sql`
de B1.2, que precisa estar aplicada no ambiente de execução. Esta etapa não
introduz migration adicional. As rotas estão conectadas ao bootstrap da API.

Testes de persistência usam `PROBLEMS_TEST_DATABASE_URL`, aceitando apenas
PostgreSQL local; criam e removem um schema isolado por teste.
Os testes de concorrência sincronizam duas leituras da mesma versão antes de
liberar as atualizações, cobrindo resolução versus classificação e duas
resoluções simultâneas, conflito, autoria e ausência de auditoria da operação
rejeitada. Sem a variável, esses testes são ignorados pelo Go.

Os testes de B5 também verificam no PostgreSQL a gravação conjunta do destino,
das origens e dos eventos, além do rollback integral quando uma origem muda
concorrentemente. Eles usam a mesma variável e são ignorados quando ela não está
configurada.
