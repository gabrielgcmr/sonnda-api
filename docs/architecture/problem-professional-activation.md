<!-- docs/architecture/problem-professional-activation.md -->
# Habilitação profissional por senha — A1.3

Status: contrato e implementação concluídos. O endpoint, a validação bcrypt, a
persistência do tipo da conta e o limite de tentativas estão conectados à API.

## Contrato HTTP

`POST /me/professional-activation` exige autenticação e uma conta registrada. O
corpo contém somente `password`. A operação atua sempre sobre a própria conta e
não aceita identificador ou tipo de conta informado pelo cliente.

Uma senha válida altera `users.account_type` de `basic_care` para `professional`
e atualiza `updated_at`. A resposta `200` devolve o perfil atualizado. Repetir a
operação para uma conta já profissional devolve o perfil atual sem validar a
senha novamente e sem nova escrita.

Uma senha inválida recebe `403`. Payload inválido recebe `400`. Depois do limite,
novas tentativas recebem `429`. Falha ou ausência da configuração de senha ou do
Redis recebe `500`, sem conceder a habilitação.

## Senha e configuração

`PROFESSIONAL_ACTIVATION_PASSWORD_HASH` contém somente um hash bcrypt. A senha em
texto claro permanece separada da senha de login e não é persistida nem escrita
em respostas ou logs. O `.env.example` contém apenas um valor vazio de exemplo;
o valor real deve existir no `.env` local ignorado pelo Git e no gerenciador de
segredos do ambiente publicado.

Trocar o hash afeta apenas ativações futuras. Contas já profissionais não são
rebaixadas nem precisam repetir a ativação. Hash ausente ou inválido deixa o
endpoint indisponível de forma fechada.

## Limite de tentativas

Somente senhas inválidas consomem tentativas. O padrão permite cinco falhas em
uma janela de quinze minutos, configurável por
`PROFESSIONAL_ACTIVATION_MAX_ATTEMPTS` e `PROFESSIONAL_ACTIVATION_WINDOW`.

O Redis mantém contadores independentes por conta e por origem. A origem é o IP
do par TCP direto, sem confiar em `X-Forwarded-For`; portanto, a infraestrutura
de proxy deve preservar origens distintas ou aceitar que clientes atrás do mesmo
proxy compartilhem o limite. Os identificadores de origem são convertidos em
SHA-256 antes de compor as chaves do Redis.

Falha no Redis não é interpretada como contador vazio: a ativação falha com erro
técnico. A habilitação não cria acesso a pacientes nem qualquer outro vínculo.

Plano geral: [Plano de implementação de problemas](problem-implementation-plan.md).
