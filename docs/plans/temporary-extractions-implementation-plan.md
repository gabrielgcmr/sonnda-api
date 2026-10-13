<!-- docs/plans/temporary-extractions-implementation-plan.md -->
# Persistência e revisão de extrações temporárias

## Objetivo

O médico poderá sair de `/home/exams`, fechar o navegador e voltar para encontrar os arquivos, o andamento e o resultado durante 24 horas. Fotos laboratoriais poderão formar um trabalho de até 20 páginas. A interface mostrará o documento e os dados extraídos lado a lado.

## Etapa 1 — Persistência e recuperação

**Status:** implementada nos repositórios. A migration deve ser aplicada antes da publicação da API.

- Criar no backend trabalhos temporários associados à conta, com tipo, páginas ordenadas, resultado estruturado e prazo de expiração.
- Referenciar capturas existentes; enviar arquivos escolhidos no computador ao bucket privado temporário.
- Adaptar `/home/exams` para criar, listar e reabrir trabalhos após navegação ou reinício do navegador. A extração continua síncrona nesta etapa.
- Manter os endpoints atuais durante a transição.

## Etapa 2 — Processamento durável

**Status:** implementada nos repositórios. Aplicar as migrations das etapas 1 e 2 antes de publicar a API.

- Enfileirar trabalhos no PostgreSQL e processá-los em worker Go com reserva temporária e retomada após reinício.
- Persistir estado e progresso; permitir nova tentativa enquanto as fontes estiverem disponíveis.
- Confirmar disponibilidade e prazo das páginas antes de gravar o resultado.

O worker roda junto com a API, assume um trabalho por vez com reserva de dois minutos e renova a reserva durante a extração. Outras instâncias podem assumir trabalhos sem reserva válida. Ao desligar, a instância devolve o trabalho à fila sem gastar tentativa; após três reservas interrompidas, o trabalho fica em falha até nova tentativa manual, que reinicia a contagem. Pânicos no extrator e fontes vencidas antes da gravação viram falha registrada. A interface consulta o estado do trabalho enquanto a página estiver aberta; o processamento no servidor independe dessa consulta.

Testes de integração do SQL e da retomada do worker: `TEMPORARY_EXTRACTIONS_TEST_DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:54322/postgres go test -tags integration -run Temporary ./internal/features/documentprocessing/postgres/`.

## Etapa 3 — Documento e resultado lado a lado

**Status:** implementada nos repositórios. `GET /temporary-extractions/{id}/pages/{position}/file` emite URL assinada de até cinco minutos, limitada ao vencimento da página, depois de confirmar a conta. A listagem traz `patient_name` (laudos laboratoriais e de imagem) e `exam_name`; a extração de imagem passou a identificar o nome do paciente. Em `/home/exams`, o envio e as extrações recentes ficam numa coluna estreita; documento e resultado ficam lado a lado quando o painel tem pelo menos 52rem e alternam por abas abaixo disso. O resultado laboratorial mostra os dados do laudo no topo e a referência em coluna própria.

- Mostrar imagem ou PDF com navegação por páginas à esquerda e resultado estruturado à direita; em telas estreitas, alternar entre as áreas.
- Emitir URLs temporárias somente após verificar a conta. Mostrar progresso, falha, prazo e aviso de revisão clínica.
- Limpar a seleção e recarregar os trabalhos ao mudar de conta.

## Etapa 4 — Otimização e múltiplas páginas laboratoriais

**Status:** implementada nos repositórios. A otimização usa o navegador antes do upload e conserva o original quando a conversão não reduz o tamanho em pelo menos 10%, não é suportada ou há transparência. A legibilidade clínica deve ser conferida no documento exibido. O worker avança em grupos de até quatro páginas e 10 MiB; extrai cada página em uma chamada própria para atribuir a origem sem ambiguidade. Isso pode aumentar o número de chamadas ao Gemini em trabalhos longos.

- Comprimir fotos no celular e no computador antes do envio, corrigindo orientação e testando inicialmente 2.400 px no lado maior e JPEG de qualidade 0,85. Conservar o original quando a conversão piorar a leitura ou não reduzir tamanho. Preservar PDFs.
- Aceitar de 1 a 20 imagens por trabalho laboratorial. Processar em grupos de até quatro páginas e 10 MiB por chamada, salvar progresso e consolidar resultados com origem por página.
- Preservar resultados de datas ou valores distintos e sinalizar divergências de identificação para revisão. Manter o limite atual de laudos de imagem.

## Contratos e retenção

- Endpoints autenticados: upload temporário; criação, listagem, consulta, nova tentativa e exclusão de trabalhos; acesso temporário às páginas.
- A consulta detalhada retorna tipo, estado, páginas, `expires_at` e resultado no formato já usado pela interface.
- Cada captura pertence a um trabalho ativo. O prazo do trabalho é o vencimento mais próximo entre suas páginas. Capturas mantêm a regra atual de expiração; uploads diretos vencem 24 horas após envio bem-sucedido.
- A exclusão manual do trabalho exclui todas as capturas e uploads diretos associados. Excluir captura pela caixa global invalida seu trabalho. Falhas de remoção do Storage permanecem indisponíveis e são retomadas na limpeza agendada existente.
- Arquivos e resultados não ficam no `localStorage`. Todas as operações e URLs exigem verificação da conta no backend. A inclusão no prontuário continua separada.

## Verificação

- Recuperar resultado após fechar e reabrir navegador; na etapa 2, recuperar também processamento em andamento.
- Mostrar JPEG, PNG e PDF com o resultado e respeitar a ordem e o vencimento das páginas.
- Testar 20 fotos laboratoriais, consolidação com origem, divergências e precisão antes/depois da compressão.
- Testar exclusão, expiração, falha de Storage, retomada do worker e isolamento entre contas.
- Aplicar migrations antes da API e publicar o frontend depois. Executar `go test ./...` e `bun run validate`.
