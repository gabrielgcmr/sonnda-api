<!-- docs/architecture/problem-identity-and-access.md -->
# Contexto de conta e acesso — A1.2

Status: revisado pela A1.4. O contrato anterior de identidade própria confirmada
e cuidador autorizado foi revogado porque introduzia uma distinção que não faz
parte da regra de negócio.

## Contrato vigente

Para cada par `(account_id, patient_id)`, a autorização consulta somente:

- a conta registrada e seu `account_type` atual;
- o acesso ativo ao paciente, validado por `patient/access`.

`owner_user_id`, `relation_type`, identidade própria confirmada e condição de
cuidador não ampliam nem reduzem a permissão. O vínculo ainda pode guardar
`relation_type` como metadado de contexto, sem efeito autorizador.

Uma conta `basic_care` com acesso ativo pode consultar problemas e resolver os
explicitamente classificados como agudos. Uma conta `professional` com acesso
ativo também pode executar as ações profissionais definidas na A1.1. A regra
clínica de cronicidade pertence a `patient/problem`.

O request não pode declarar `account_type` ou acesso para obter permissão. Falha
ao consultar conta ou acesso resulta em erro ou negativa, conforme o contrato
central de erros; nunca em concessão implícita.

## Reversão do contrato anterior

A A1.4 remove do código o serviço de confirmação de identidade, seu adaptador de
persistência e os campos `self_verified`, `caregiver_authorized` e
`relationship_type` do contexto de autorização. A migration que criou
`patient_self_confirmations` permanece no histórico para não reescrever migrations
que podem ter sido aplicadas. A tabela fica sem consumidor e pode ser retirada por
uma migration compensatória depois de verificar os ambientes e a necessidade de
reter seus registros.

Plano geral: [Plano de implementação de problemas](problem-implementation-plan.md).
