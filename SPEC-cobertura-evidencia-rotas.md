# SPEC — metodologia para fechar F239/F282

**Segue o RFC** (`RFC-cobertura-evidencia-rotas.md`), com as quatro decisões
já tomadas pelo usuário:

1. Evidência migra para **campo estruturado** em `api/openapi/evidencias.tsv`.
2. Lotes por **família de rota**.
3. Parada por **número fixo de rotas por sessão/turno**.
4. Rota reprovada **para a campanha** até a correção ser decidida/aplicada.

## 1. A tabela `evidencias.tsv` ganha três colunas

Hoje: `método	caminho	marca` (3 colunas, comentário no topo documenta os
quatro valores de `marca`). Passa a:

```
método	caminho	marca	data	observador	evidência
```

- **`data`** — `AAAA-MM-DD` da medição. Vazio nas linhas ainda não
  remedidas nesta campanha (não se preenche em massa "hoje" para linhas
  não tocadas — isso seria fingir medição).
- **`observador`** — um destes literais, curto e enumerável (para uma
  auditoria futura poder contar por tipo): `segunda-rota` (GET irmão do
  POST/PUT/DELETE testado), `websocket` (evento observado em `/session/ws`
  de uma sessão diferente da que agiu), `sqlite` (leitura direta de
  tabela — só quando aplicável, como na `CAMPANHA-DESCARTAVEL.md`),
  `cliente` (confirmação visual no WhatsApp Web/app — rara, exige
  Claude in Chrome e é mais cara), `protocolo` (o próprio erro/sucesso do
  WhatsApp já é a prova, ex.: `406 not-acceptable` genuíno em vez de `400`
  de rota mal-formada — válido só quando NÃO havia efeito nenhum a
  confirmar, como puro roteamento), `motivo` (não confirma efeito — é a
  razão de uma rota ⬜/❌: por que não foi executada, ou a causa medida da
  falha), `misto` (mais de um observador na mesma medição).
- **`evidência`** — uma frase curta e ESPECÍFICA: o que foi lido, que
  valor mudou. Formato livre, mas banido por definição o texto genérico
  atual ("chamada real com resposta e efeito confirmado por segunda
  leitura ou pelo cliente") — se a frase pudesse ser copiada para
  qualquer outra rota sem editar nada, não é específica o suficiente.

Linhas com `marca` diferente de ✅ (🟡/❌/⬜) só ganham as três colunas
quando também forem tocadas por esta campanha — não é obrigatório
preenchê-las todas de uma vez; `#` vazio é aceitável até lá.

## 2. `docs/OPENAPI-EVIDENCIAS.md` passa a ser DERIVADO, não a fonte

Hoje é prosa mantida à mão, divergente da tabela (é por isso que F282 e
este RFC precisaram de comparar as duas). Depois desta campanha, o
documento markdown deixa de ser editado directamente: um pequeno gerador
(`cmd/openapidoc`, ou um `cmd/` novo e pequeno se acoplar mal) lê
`evidencias.tsv` e escreve a tabela completa do relatório — mesma regra
que `caminhos.tsv` já segue para rotas canónicas ("não há terceira cópia a
desactualizar-se").

**Escopo mínimo aceitável** desta parte, para não virar um projeto à
parte: o gerador só precisa produzir a TABELA (as linhas), não o resto da
prosa introdutória do relatório — essa continua editada à mão, como hoje.

## 3. O procedimento por rota (o que "medir" quer dizer aqui)

Para cada rota da família do lote:

1. **Ler a marca e evidência atuais.** Se já é 🟡/❌/⬜ com motivo
   específico (não a frase-modelo), pular — já está correto pela própria
   definição da marca (fora do escopo deste RFC).
2. **Montar o pedido real** contra `envia` (ou `envia`+`recebe` quando a
   rota precisar de duas pontas — ex.: enviar e confirmar recebimento).
   `recebe` só observa; se a rota exigir `recebe` AGIR, registrar como
   limite em vez de contornar (mesma regra do RFC).
3. **Chamar a rota.** Registrar status e corpo.
4. **Confirmar o efeito com um observador independente** — nunca o `200`
   sozinho. Ordem de preferência: `sqlite` > `segunda-rota` >
   `websocket` > `cliente` > `protocolo` (só quando não havia efeito a
   confirmar, como nas três rotas 🟡/⚠️ que a F286 já fechou desta forma).
5. **Classificar**:
   - Efeito confirmado como esperado → `✅`, evidência específica.
   - Resposta de sucesso sem como confirmar o efeito → `🟡` (regressão
     documentada, não reprovação — a rota pode estar certa e só faltar
     observador).
   - **O `200` mentiu** (efeito não aconteceu, ou aconteceu errado) →
     **PARAR A CAMPANHA**. Ver §4.
6. **Desfazer o fixture** (sair de grupo, apagar canal, etc.) antes de
   passar para a próxima rota — mesma disciplina desta sessão.
7. **Escrever a linha** em `evidencias.tsv` (as três colunas novas) e,
   quando o achado for incidental (bug fora do escopo da rota sendo
   testada), uma entrada no `HOUSEKEEP.md` — mesmo padrão de F256/F265.

## 4. Protocolo de "PARAR" (decisão 4 do RFC)

Quando uma rota reprova (o `200`/sucesso aparente não corresponde ao
efeito real):

1. A campanha de medição **para imediatamente** — não passa para a
   próxima rota da família.
2. Registro IMEDIATO no `HOUSEKEEP.md`, com o `F<n>` seguinte, seguindo a
   estrutura de sempre (data/contexto, onde, problema, evidência,
   correção sugerida, status).
3. Reporto ao usuário: qual rota, o que devia acontecer, o que aconteceu.
4. **Aguardo decisão explícita** antes de retomar: corrigir agora (como
   F278/F280/F261 nesta sessão), registrar como `aberto` e continuar a
   campanha nas OUTRAS famílias, ou outra instrução.
5. Só depois de decidido isso — corrigido, ou explicitamente adiado — a
   campanha retoma, na mesma família ou na próxima, conforme o PLAN.

Isto é mais lento que "registra e segue", mas é a decisão do usuário, e
tem uma vantagem real: uma reprovação pode ser sintoma de uma causa que
afeta MAIS de uma rota da mesma família (como F227/F228/F229 no
histórico) — parar evita medir 10 rotas em cima de uma premissa já
quebrada.

## 5. Definição de "família", para o PLAN batizar os lotes

Agrupamento pelo prefixo de caminho canónico, que já reflete a
organização real do código (`pkg/presentation/http/dto/<família>`):

`chats` (mensagens e ações de conversa) · `groups` · `communities` ·
`newsletters` · `users` (contatos, bloqueio, perfil) · `status` · `labels`
· `webhook`/`s3`/`hmac`/`proxy` (configuração — geralmente já ✅ com
evidência boa, checar antes de incluir) · `admin` (idem) · avulsas
(`/call/reject`, etc.).

## 6. Critério de "pronto"

A campanha termina quando **as 93 linhas com a frase-modelo** (contagem
inicial, `docs/OPENAPI-EVIDENCIAS.md`) tiverem, cada uma: `marca`
possivelmente revista, `data`, `observador`, `evidência` específica
preenchidos em `evidencias.tsv`; o gerador de `OPENAPI-EVIDENCIAS.md`
existir e produzir a tabela sem prosa genérica remanescente;
`go run ./cmd/openapidoc` e o gate correspondente verdes; `HOUSEKEEP.md`
com uma entrada fechando F239 e F282 (referenciando o novo total e
citando os achados incidentais abertos durante a campanha, se houver).
