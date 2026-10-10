# Refatoração incremental do repositório de exames laboratoriais

## Resumo

Refatorar somente `internal/features/patient/exam/laboratory/postgres`, preservando a interface da feature, as rotas, o banco e o comportamento atual. Alterações mecânicas em `bootstrap` e testes serão feitas apenas para acompanhar a renomeação do construtor; nenhum outro adaptador de repositório será refatorado.

## Etapas

1. **Caracterização**

   Criar testes do adaptador para:

   - reconstrução de `lab_report → lab_panels → observations`;
   - persistência transacional do agregado;
   - rollback quando a criação de painel ou observação falhar;
   - `FindByID` retornando `nil` para `pgx.ErrNoRows`;
   - listagem de laudos e timeline;
   - exclusão do laudo com cascata.

   Nenhuma alteração de comportamento será feita nesta etapa.

2. **Padronização de nomes**

   Dentro do adaptador de laboratório:

   - renomear `LabsRepository` para `Repository`;
   - renomear `NewLabsRepository` para `NewRepository`;
   - manter a interface `laboratory.Repository` sem mudanças;
   - atualizar somente os pontos de composição e testes que instanciam o adaptador.

3. **Separação dos arquivos**

   Organizar o pacote assim:

   ```text
   internal/features/patient/exam/laboratory/postgres/
   ├── repository.go
   ├── read.go
   ├── write.go
   ├── mapper.go
   └── errors.go
   ```

   - `repository.go`: tipo, construtor e verificação da interface;
   - `read.go`: `FindByID`, `ListLabs` e timeline;
   - `write.go`: `Create`, `CreateInTx` e `Delete`;
   - `mapper.go`: conversões SQLC/domínio;
   - `errors.go`: classificadores específicos do adaptador.

4. **Padronização da transação**

   Preservar as duas operações:

   ```go
   Create(ctx, report)
   CreateInTx(ctx, tx, report)
   ```

   `Create` continuará abrindo e confirmando sua própria transação. `CreateInTx` continuará disponível para o fluxo de confirmação, sem alterar `DraftRepository` ou qualquer outro repositório.

   O rollback deverá usar o mesmo contexto de limpeza já adotado no código atual.

5. **Limpeza interna**

   - remover helpers sem uso;
   - usar os conversores compartilhados de `database/postgres/pgtypes.go`;
   - concentrar conversões de `LabReport`, `LabPanel` e `Observation` em `mapper.go`;
   - tornar privado qualquer helper usado apenas pelo adaptador;
   - preservar os nomes e formatos públicos da API.

## Validação

Executar os testes unitários e de compilação após as etapas relevantes:

```powershell
go test ./internal/features/patient/exam/laboratory/... ./internal/application/bootstrap ./internal/application/usecase/labdocumentconfirmation
```

Os testes de integração requerem `LABS_TEST_DATABASE_URL` apontando para um
PostgreSQL local isolado. Eles criam e removem um schema próprio:

```powershell
$env:LABS_TEST_DATABASE_URL = "postgres://postgres:postgres@localhost:5432/sonnda_test?sslmode=disable"
go test -tags=integration ./internal/features/patient/exam/laboratory/postgres ./internal/features/documentprocessing/postgres -count=1
```
