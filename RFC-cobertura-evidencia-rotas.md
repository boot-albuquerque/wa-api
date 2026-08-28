# RFC — fechar F239/F282: evidência específica para as rotas que só têm a frase-modelo

**Data**: 2026-08-28. **Autor**: sessão atual, a pedido explícito do usuário.
**Relacionado**: `HOUSEKEEP.md` F239, F282, F286; `CAMPANHA-DESCARTAVEL.md`
(precedente direto — mesma ideia, escopo menor); `AUDITORIA-EVIDENCIAS.md`.

**Status**: proposto — precisa das decisões da seção "O que preciso que você
decida" antes de eu escrever o SPEC e o PLAN.

## O problema, sem inflar o número

F239 mediu "~85 rotas com estado funcional desconhecido" e F282 mediu "98 ✅
com a mesma frase de evidência genérica, zero específicas". **Os dois números
não somam 183** — são duas medições do MESMO buraco por ângulos diferentes
(F239 pergunta "sabemos que funciona?", F282 pergunta "a evidência escrita
prova alguma coisa?"), e a interseção é grande. Contagem atual, direto da
fonte de verdade (`docs/OPENAPI-EVIDENCIAS.md`, 2026-08-28):

```
$ grep -c "chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente" \
    docs/OPENAPI-EVIDENCIAS.md
93
```

**93 rotas**, hoje, carregam só a frase-modelo. É esse o número real a
resolver — não 183. A `CAMPANHA-DESCARTAVEL.md` já fechou a fatia que não
precisava de conta real (32 ⬜ → 7); o que sobra é exatamente a fatia que
aquela campanha não podia tocar: rotas cujo efeito só se confirma contra o
protocolo do WhatsApp de verdade.

## Por que agora, e o que mudou

Duas sessões noise reais e pareadas ficaram disponíveis nesta sessão
(pedido explícito do usuário): **"envia"** (`5516981818244`, pode agir à
vontade — enviar, criar, apagar, o que a rota pedir) e **"recebe"**
(`554192421234`, só observa — não deve ser usada para iniciar ações que
não sejam as estritamente necessárias para confirmar o efeito do lado de
quem recebe). É a mesma dupla já usada nesta sessão para fechar F264/F278,
F280, F261 (correções) e F256/F260/F265/F272/F286 (investigações) — cada
uma com evidência de campo específica, não a frase-modelo.

## O que a `CAMPANHA-DESCARTAVEL.md` já ensina, e que este RFC herda

- **Fixture descartável não é desculpa para pular a medição do EFEITO.**
  Criar/apagar um recurso de teste é barato; o que prova a rota é o
  observador (segunda leitura, WebSocket, log), não o `200`.
- **Hierarquia de observadores, da mais forte para a mais fraca**: leitura
  direta do dado (segunda rota GET, ou SQLite quando aplicável) > quadro de
  WebSocket > log do servidor (só para diagnosticar, nunca para promover
  sozinho).
- **O `200` já mentiu três vezes nesta árvore** (F227 mensagem não
  guardada, F228 voto não contado, F229 perfil alterado por engano). A
  regra que sai disso: **nenhuma rota é promovida por status HTTP sozinho.**

## Escopo

**Dentro**: as 93 rotas com a frase-modelo em `docs/OPENAPI-EVIDENCIAS.md`,
reclassificadas com evidência específica (o que foi lido, que valor
mudou) — promovendo, rebaixando para 🟡, ou confirmando ✅ com prova real,
conforme o que a medição mostrar. Rotas que já são 🟡/❌/⬜ com motivo
específico ficam de fora (já estão corretas pela própria definição da
marca).

**Fora**: qualquer correção de bug encontrado durante a medição não entra
no mesmo lote — vira achado no `HOUSEKEEP.md`, igual às sessões anteriores
(F256, F265, etc. nasceram assim). Rotas que só existem no wa-headless
(fora do escopo desta dupla noise) ficam para quando houver sessão
headless pareada.

## Riscos e limites conhecidos, ditos antes de começar

1. **93 rotas é grande demais para uma sessão.** Vai exigir várias sessões
   /turnos — o PLAN tem de dizer em que ordem e com que critério de parada
   por lote.
2. **Ações irreversíveis ou vistosas** (mudar nome/foto da conta,
   bloquear/desbloquear, sair de grupo, mudar privacidade) precisam de
   confirmação explícita antes de cada lote que as contenha — mesma regra
   que já segui nesta sessão (perguntei antes de "recebe" pedir entrada em
   grupo).
3. **"recebe" só observa** continua valendo — uma rota que só possa ser
   medida fazendo "recebe" AGIR (não reagir/observar) fica registrada como
   limite, não contornada.
4. **Consumo de tokens/turnos**: 93 rotas com medição real, uma a uma,
   documentadas no padrão HOUSEKEEP (como as 5 investigações desta sessão)
   é um volume de trabalho da ordem de dezenas de milhares de tokens por
   lote. O PLAN vai propor lotes por família de rota (mensagens, grupos,
   newsletter, contatos, mídia…), não uma corrida única.

## O que preciso que você decida antes do SPEC/PLAN

1. **Formato do registro de evidência**: continuar escrevendo em prosa no
   `docs/OPENAPI-EVIDENCIAS.md` (como as promoções da `CAMPANHA-DESCARTAVEL.md`
   já fizeram), ou migrar para campo estruturado na própria `evidencias.tsv`
   (a "correção sugerida" que F282 registrou e nunca foi aplicada)?
2. **Ordem dos lotes**: por família de rota (ex: todas as de mensagem, depois
   todas as de grupo, …) ou por marca atual (as 93 ✅ primeiro, versus
   misturar com as 🟡/⬜ que também têm motivo fraco)?
3. **Critério de parada por lote**: número fixo de rotas por sessão, ou
   "até a próxima ação vistosa/irreversível, aí eu paro e confirmo"?
4. **O que fazer quando uma rota reprovar** (o `200` mentir de novo): vira
   HOUSEKEEP na hora e a campanha continua, ou eu paro tudo para corrigir
   primeiro? (a prática desta sessão foi: registro o achado, sigo
   investigando/medindo, só corrijo à parte se for pedido — proponho manter
   isso, mas é decisão sua.)
