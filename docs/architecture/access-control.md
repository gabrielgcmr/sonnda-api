<!-- docs/architecture/access-control.md -->
# Controle de acesso aos pacientes

O acesso verifica a quais pacientes uma conta está vinculada. Ele não decide
quais ações a conta pode executar. A política por ação para problemas fica em
[`authz`](authz/README.md).

A autenticação continua em `internal/features/auth`: valida a identidade externa.
O middleware de account resolve o cadastro local. O checker em
`internal/features/patient/access` recebe os identificadores da conta e do
paciente solicitado, sem depender de HTTP ou de um perfil profissional.

## Regra atual

`RequireAccess(ctx, accountID, patientID)` permite acesso quando a conta é
o dono (`OwnerUserID`) do paciente ou tem um vínculo ativo em `patient_access`.
Sem vínculo, a resposta é 403 (`ACCESS_DENIED`). Um paciente inexistente recebe
a mesma resposta, evitando revelar sua existência. Falhas de consulta não
concedem acesso e são retornadas pelo contrato central de erros.

A checagem é compartilhada pelo serviço de pacientes e pelos handlers de exames
e laudos, antes de ler, alterar ou processar dados. Ela é uma dependência
obrigatória desses fluxos. Os métodos de exclusão do serviço também exigem o
vínculo; não há mais bloqueio por ação. Nenhuma nova rota foi exposta.

A criação do paciente grava seu vínculo com o criador na mesma transação.
`GET /me/patients` é servido pela feature de acesso e lista os pacientes
vinculados à conta. A resposta contém apenas `id`, `full_name` e `avatar_url`;
`relation_type` continua armazenado como metadado interno e não é exposto.
Ser médico ou ter `AccountType=professional` não concede acesso a outros pacientes.

## Modelos e persistência

- `internal/features/account/domain`: `Account`, `Identity`, `Profile` e `AccountType`.
- `internal/features/patient/access/domain`: vínculo com o paciente e tipo de relacionamento.
- `internal/features/patient/access`: serviço de listagem, contratos de persistência e checker de acesso.
- `internal/features/patient/access/http`: handler e resposta da listagem de pacientes acessíveis.
- `internal/features/patient/access/postgres`: adaptador PostgreSQL de acesso.

A entidade, o serviço e o repositório antigos de profissionais e as políticas
RBAC antigas foram removidos. `AccountType` e o tipo de relacionamento permanecem
como dados existentes. `relation_type` é metadado do relacionamento, não concede
ações e não faz parte das listagens. `authz` pode usar o `AccountType` persistido
depois que este checker confirmar o acesso. Contas mínimas `basic_care` são
provisionadas automaticamente a partir da identidade autenticada; não existe
endpoint separado de cadastro da conta.

Tabelas, migrações e código SQLC gerado de profissionais foram preservados.
A remoção desses artefatos de persistência deve ocorrer em uma etapa própria.

## Verificação

Os testes do checker de acesso cobrem dono, vínculo ativo, ausência de vínculo,
paciente inexistente, identidade ausente e falhas de consulta. Os testes dos
consumidores verificam que a negativa interrompe o fluxo antes de leituras,
alterações ou processamento de documentos. A listagem verifica paginação, falhas
de persistência e ausência de metadados de relacionamento na resposta HTTP.
