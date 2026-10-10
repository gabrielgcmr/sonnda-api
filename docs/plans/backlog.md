<!-- docs/plans/backlog.md -->
# Backlog técnico da API

## OpenAPI: permitir geração com dependências vazias

**Prioridade:** alta.

`OpenAPI()` registra as rotas usando uma estrutura `APIDependencies` vazia. Com
as novas rotas de captura e as rotas existentes de contas e pacientes,
`registerHumaRoutes` pode acessar campos de handlers ou middlewares que estejam
com valor `nil`, causando um panic durante a exportação da especificação
OpenAPI.

Corrigir adotando uma das seguintes abordagens:

1. tornar `registerHumaRoutes` tolerante a dependências ausentes, registrando
   cada rota somente quando os handlers e middlewares correspondentes não forem
   `nil`; ou
2. construir handlers reais ou stubs exclusivamente para a geração da
   especificação.

**Critério de aceite:** a exportação do OpenAPI deve concluir sem panic e incluir
as rotas esperadas de captura, contas e pacientes, com cobertura automatizada
para o cenário de geração da especificação.

## Autenticação de captura: validar credencial nula

**Prioridade:** alta.

O middleware `RequireCaptureToken`, em `internal/api/middleware/auth.go`,
desreferencia `credential` sem verificar se o valor retornado pelo autenticador
é `nil`. Se `Authenticate` retornar `(nil, nil)`, seja por uma implementação
incorreta ou por uma alteração futura, a requisição causará um panic.

Adicionar uma validação de `nil` antes de desreferenciar a credencial. Como
`(nil, nil)` representa uma violação do contrato interno do autenticador, a
resposta recomendada é um `AppError` interno escrito por `humaerror.Write`, sem
expor detalhes da causa ao cliente. Tratar o resultado como não autorizado é
uma alternativa caso esse comportamento seja definido explicitamente no
contrato de autenticação.

**Critério de aceite:** quando o autenticador retornar `(nil, nil)`, o
middleware deve responder com o erro definido, não deve executar o próximo
handler e não deve causar panic. O cenário deve possuir teste automatizado.

## Repositório de capturas: padronizar validação de configuração

**Prioridade:** média.

O método `RevokeSessionsByAccount` do adaptador PostgreSQL de capturas usa
`r.queries` sem verificar antes se `r` ou `r.queries` são `nil`. Outros métodos
do mesmo pacote também não aplicam essa validação de forma consistente. Se
`NewRepository(nil)` for utilizado ou o repositório estiver configurado
incorretamente, esses métodos podem causar um panic.

Adicionar a mesma validação defensiva de configuração já utilizada por outros
métodos do adaptador. Revisar todos os métodos públicos de `Repository` no
pacote `internal/features/capture/postgres` para que adotem uma política única,
retornando um erro interno de persistência em vez de causar panic quando as
dependências obrigatórias não estiverem configuradas.

**Critério de aceite:** nenhum método público do repositório deve causar panic
quando o receiver ou suas dependências obrigatórias forem `nil`. Os métodos
devem retornar erros consistentes com a arquitetura de persistência, e os
cenários de repositório não configurado devem possuir testes automatizados.

## Limpeza de capturas: documentar e validar o papel de acesso ao banco

**Prioridade:** média.

A tabela `capture_cleanup_runs` possui RLS habilitado e não possui policies. A
migration revoga o acesso de `public`, `anon` e `authenticated` e concede
`SELECT`, `INSERT` e `UPDATE` a `service_role`. No Supabase hospedado,
`service_role` possui `BYPASSRLS` por definição; portanto, policies não são
avaliadas para esse papel e adicionar uma policy `TO service_role` não tornaria
o acesso mais explícito nem mais seguro.

O ponto a esclarecer é qual papel efetivamente acessa a tabela em cada
ambiente. A API usa uma conexão PostgreSQL direta configurada por
`DATABASE_URL`, que pode operar como proprietário da tabela ou com outro papel,
enquanto testes locais podem criar um `service_role` sem reproduzir o atributo
`BYPASSRLS` do Supabase.

Documentar o modelo de acesso esperado e validar em ambiente local e de staging
que o papel utilizado pelo processo de limpeza possui os grants necessários e
o comportamento de RLS esperado. Se futuramente for adotado um papel de banco
personalizado sem `BYPASSRLS`, criar policies explícitas para esse papel, com
privilégios mínimos, em vez de criar policies ineficazes para `service_role`.

**Critério de aceite:** a documentação deve identificar o papel usado pela API
e pelo agendador; testes de banco devem comprovar que `anon` e `authenticated`
não acessam a tabela e que o papel real do processo de limpeza consegue executar
somente as operações necessárias. A ausência de policy para `service_role` deve
ser registrada como intencional.

## Capturas: unificar o limite máximo de tamanho do arquivo

**Prioridade:** média.

O comando de limpeza ainda configura o limite de 5 MiB como
`5 * 1024 * 1024`, enquanto o domínio já define
`capturedomain.MaxFileSizeBytes`. A duplicação remanescente pode permitir que a
configuração do Storage, a validação do domínio e a constraint `captures_size`
do banco fiquem divergentes.

Usar `capturedomain.MaxFileSizeBytes` ao configurar o bucket de capturas em
`cmd/cleanup-captures/main.go`, como já ocorre na composição da API. Manter o
limite genérico do adaptador de Storage independente do limite específico da
feature e adicionar uma verificação automatizada que detecte divergência entre
a constante do domínio e a constraint do banco.

**Critério de aceite:** a composição da aplicação não deve conter o valor
numérico do limite de capturas; API, comando de limpeza e validações da feature
devem usar a mesma constante. Um teste de migration ou integração deve confirmar
que `captures_size` aceita o limite exato e rejeita valores acima dele.
