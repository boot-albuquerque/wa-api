# Matriz de capacidades do WhatsApp — só o que foi investigado

**Esta tabela NÃO é uma matriz completa.** Contém exclusivamente as capacidades
investigadas na sessão de 2026-08-26 (F264 e F265) e as que foram medidas em
campo na mesma bateria e usadas como controlo. Acrescentar linha aqui exige a
mesma disciplina: evidência com fonte e nível, ou não entra.

## Níveis de evidência

| nível | significado |
|---|---|
| **A** | documentação oficial da Meta/WhatsApp |
| **B** | comportamento medido por nós, em campo |
| **C** | implementação de referência madura (código no `main`) |
| **D** | issue/PR com repro ou medição declarada |
| **E** | opinião sem fonte — **não usada neste documento** |

## Etiquetas de conta

`PERSONAL_SUPPORTED` · `BUSINESS_SUPPORTED` · `BOTH_SUPPORTED` ·
`PERSONAL_ONLY` · `BUSINESS_ONLY` · `ACCOUNT_STATE_DEPENDENT` · `UNKNOWN`

---

## Tabela

| Capacidade | Personal | Business | Pré-condição | Evidência | Fonte + nível |
|---|---|---|---|---|---|
| **Ler a blocklist** (`GET /users/blocklist`) | sim | sim | sessão ligada | `200 {"Blocklist":[],"DHash":"…"}` nas duas contas | bateria 2026-08-26 — **B** |
| **Bloquear contacto** (`POST /users/block`) | sim | sim | `<item>` com `jid`=**LID** e `pn_jid`=PN; a SPA exige ainda uma conversa existente ao bloquear um contacto PN | `422` em 4/4 com a forma antiga (PN); a via SPA executa a operação com sucesso (LEDGER H59) | medição **B**; forma do stanza **C** (whatsmeow `8d023aa973`, Baileys `8ca9316a10`, wwebjs `Contact.js`); pré-condição da SPA **C** (string do bundle vivo, `internal/wa-headless/capabilities/block/block.go:15`) |
| **Desbloquear contacto** (`POST /users/unblock`) | sim | sim | `<item>` com `jid`=**LID**, **sem** `pn_jid` | idem — `422` em 4/4 | idem. A regra "unblock usa LID e dispensa `pn_jid`" é explícita nas duas referências — **C** |
| **Resolver PN → LID** (pré-requisito das duas acima) | sim | sim | mapa local (`LIDs` store) ou `GetUserInfo`/`queryWidExists` contra o servidor | não medido isoladamente nesta sessão | whatsmeow `8d023aa973` usa store + fallback `GetUserInfo`; Baileys usa `signalRepository.lidMapping` e falha `400` sem ele — **C** |
| **Ler mensagens de um canal** (`POST /newsletters/messages`) | sim | sim | `<messages type='jid' jid=… count=…>` para `s.whatsapp.net` | `200 []` no canal de teste (vazio) | bateria 2026-08-26 — **B** |
| **Ler contadores de um canal** (`POST /newsletters/updates`) | sim | sim | **a forma `<message_updates>` para o JID do canal está morta**; os contadores vêm dentro da resposta de `<messages type='jid'>` | `500` ao fim de 30 s, sem resposta do servidor, 2026-08-26 | medição **B**; causa **D** (Baileys #2555 aberta 2026-05-13 com o mesmo timeout; PR #2620 com o stanza capturado do WA Web) + **C** (port mergeado em rsalcara/InfiniteAPI #503, 2026-06-06) |
| **Assinar updates ao vivo de um canal** (`POST /newsletters/subscribe`) | sim | sim | IQ `set` para o JID do canal, filho `<live_updates>` | `200` — **é o controlo que prova que o servidor roteia IQs para `@newsletter`** | bateria 2026-08-26 — **B** |

---

## Etiquetas de conta, por capacidade

| Capacidade | Etiqueta | Porquê |
|---|---|---|
| Ler a blocklist | `BOTH_SUPPORTED` | medido nas duas contas, `200` nas duas |
| Bloquear / desbloquear | `BOTH_SUPPORTED` | medido nas duas contas com resultado IDÊNTICO; a falha é anterior a qualquer regra de conta, logo o tipo de conta não é o discriminador |
| Resolver PN → LID | `UNKNOWN` | não medido isoladamente; nenhuma referência distingue conta |
| Ler mensagens de canal | `BOTH_SUPPORTED` | rota irmã medida a `200`; nada nas referências liga a conta |
| Ler contadores de canal | `BOTH_SUPPORTED` | a capacidade existe pela forma nova; a falha medida é de forma de stanza, não de permissão |
| Assinar updates ao vivo | `BOTH_SUPPORTED` | medido a `200` |

**Nenhuma capacidade investigada ficou `BUSINESS_ONLY`, `PERSONAL_ONLY` ou
`ACCOUNT_STATE_DEPENDENT`.** Foi verificado explicitamente e a resposta foi
negativa nas duas rotas — o que é informação, não ausência dela.

---

## Ausência de documentação oficial — registada, não inferida

Não existe documentação pública da Meta para nenhuma das capacidades acima:

- o protocolo binário do WhatsApp Web/multi-device não é documentado;
- a Cloud API (a única API oficial documentada) **não expõe** bloquear ou
  desbloquear contactos;
- a Cloud API **não expõe** Canais (newsletters) de todo — nem leitura de
  mensagens, nem contadores.

Consequência: **nenhuma linha desta tabela pode atingir Nível A**, e o teto
prático é C/D. Isto é uma propriedade do domínio, não uma falha da pesquisa, e
está aqui escrito para que ninguém volte a procurar o que não existe.

### A ausência na Cloud API não tem valor probatório — em nenhuma direcção

Os dois pontos sobre a Cloud API, acima, dizem **onde não se encontrou**, não o
que existe. `docs/REFERENCIA-META-OFICIAL.md` estabelece que a Cloud API e este
projecto são **superfícies diferentes** — Graph API com conta registada,
templates e custo por conversa, contra o protocolo do WhatsApp Web falado pelo
fork em `internal/wa-noise` — e que cerca de **60 das 141 rotas** daqui (grupos,
comunidades, canais, status) não têm equivalente lá **por desenho**.

Portanto:

- a ausência **não é** evidência de `UNSUPPORTED_CONFIRMED`;
- a ausência **não é** evidência de Nível B, apesar de "verificada": verificar
  que algo não está num sítio onde nunca estaria não mede a capacidade;
- `UNSUPPORTED_CONFIRMED` continua a exigir o que sempre exigiu — doc oficial
  **da superfície certa** mais comportamento medido, ou várias implementações
  de referência maduras a concordar.

As classificações desta tabela não dependem da Cloud API em nenhum ponto; esta
nota existe para que uma leitura futura não lhes empreste um apoio que elas não
têm nem precisam.
