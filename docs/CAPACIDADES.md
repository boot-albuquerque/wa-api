# Capacidades do wa-api — o que temos, o que falta, e o que o protocolo não dá

**Data do levantamento**: 2026-08-23, contra `feature/wa-noise` em `4ea73be`.

## Como ler este documento

As colunas **wa-api** e **protocolo** são MEDIDAS: vêm de contar rotas
registadas em `pkg/bootstrap/wiring_routes.go` e de procurar símbolos em
`internal/wa-noise/`. Os comandos estão em cada secção para poderem ser
repetidos.

As colunas dos CONCORRENTES são **conhecimento geral, não medição**. Não corri
Cloud API, Evolution nem Baileys para produzir esta tabela. Onde a diferença
decide alguma coisa, verifique antes de agir — este projeto já se enganou uma
vez por acreditar numa issue de outro projeto sem medir (ARMADILHAS #26, e a
F222, em que uma afirmação vinda de `mautrix/whatsapp#904` foi refutada em
campo).

## O que somos, tecnicamente

`internal/wa-noise` é um fork Go do protocolo **WhatsApp Web multi-dispositivo**
— a mesma família técnica do Baileys, não da API oficial. Isso define o teto e
o chão de tudo o que segue:

- **Teto**: podemos fazer o que um cliente WhatsApp faz, incluindo o que a API
  oficial nunca expôs.
- **Chão**: não temos garantias, SLA nem estatuto oficial da Meta.

A comparação certa é com **Baileys** (biblioteca) e **Evolution API** (HTTP
sobre Baileys). A **Cloud API** é outra categoria: menos superfície, mais
garantias.

## Superfície atual: 107 rotas

| família | rotas |
|---|---|
| chat | 28 |
| group | 18 |
| user | 16 |
| session | 14 |
| newsletter | 12 |
| webhook | 6 |
| hmac | 3 |
| labels | 2 |
| status | **1** |
| call | 1 |
| proxy | 1 |

Reproduzir:

```
grep -oE 'registry.Register\("(/[a-zA-Z0-9/{}._-]+)"' \
  pkg/bootstrap/wiring_routes.go | sed 's/registry.Register("//;s/"//' | sort
```

## Comparação

| capacidade | wa-api | Cloud API | Evolution | Baileys |
|---|---|---|---|---|
| Envio: texto, imagem, vídeo, áudio, documento, sticker | ✅ | ✅ | ✅ | ✅ |
| Envio: localização, contacto, enquete, template | ✅ | ⚠️ template aprovado | ✅ | ✅ |
| Botões e lista | ✅ | ⚠️ só via template | ✅ | ✅ |
| **Carrossel (HSCROLL_CARDS)** | ✅ provado em campo (F216) | ❌ | ⚠️ | ⚠️ raw proto |
| **Reply-to (citar)** | ✅ 14 rotas (CAP-46) | ✅ | ✅ | ✅ |
| **Menções (@)** | ✅ 8 rotas com texto (CAP-47) | ✅ | ✅ | ✅ |
| Editar / apagar / reagir | ✅ | ⚠️ parcial | ✅ | ✅ |
| **Grupos** | ✅ 18 rotas | ❌ **não existe** | ✅ | ✅ |
| **Canais (newsletter)** | ✅ 12 rotas | ❌ | ⚠️ parcial | ✅ |
| Contactos, bloqueio, privacidade | ✅ | ❌ | ✅ | ✅ |
| Presença, recibos de leitura | ✅ | ⚠️ limitado | ✅ | ✅ |
| Download de média (5 tipos) | ✅ | ✅ | ✅ | ✅ |
| Webhook, S3, HMAC, proxy | ✅ | ✅ só webhook | ✅ | ➖ é lib |
| Etiquetas (labels) | ✅ 2 rotas | ❌ | ⚠️ | ✅ |
| Rejeitar chamada | ✅ | ❌ | ⚠️ | ✅ |

**A vantagem estrutural sobre a Cloud API são grupos e canais**: não existem na
API oficial. Quem precisa de bot em grupo não tem alternativa oficial.

## O que nos falta — a rota, com o protocolo já pronto

Estes são baratos: a capacidade está no fork e não tem consumidor.

### 1. Votar em enquete

Criamos enquetes (`/chat/send/poll`) e **ninguém pode responder pela API**.

```
$ grep -c "BuildPollVote" <simbolos do fork>      -> 1   (existe)
$ grep -rn "BuildPollVote" pkg/ --include=*.go    -> 0   (sem consumidor)
```

### 2. Status/stories com média

Só existe `/status/set/text`. O fork já conhece o destino:

```
internal/wa-noise/protocol/types/jid.go:36:  StatusBroadcastJID = NewJID("status", BroadcastServer)
internal/wa-noise/core/broadcast.go:15:      if jid == types.StatusBroadcastJID {
```

**Mas NÃO é uma rota de envio normal com outro destino.** Um status vai para
TODOS os contactos, não para um JID, e o fork resolve essa lista antes de
enviar (`getBroadcastListParticipants` -> `getStatusBroadcastRecipients`,
`broadcast.go:12-20`). Isso muda o custo: há uma resolução de destinatários no
caminho, e um modo de falha novo (lista vazia, ou o próprio remetente na
lista).

### 3. Encaminhar mensagem

**CORREÇÃO 2026-08-23**: eu tinha classificado isto como "falta no protocolo".
**Estava errado.** O `ContextInfo` já tem os dois campos que o encaminhamento
usa:

```
waE2E/...pb.go:9022:  ForwardingScore *uint32
waE2E/...pb.go:9023:  IsForwarded     *bool
```

Não é preciso implementar nada no fork: é montar campos existentes, exatamente
como se fez no reply-to (CAP-46). O que falta é a rota e o caso de uso.

### 4. Desaparecimento por conversa

Temos `/group/ephemeral`. Falta o equivalente para conversa individual;
`SetDisappearingTimer` está no fork e é usado em 34 sítios internos, sem rota.

## O que falta no PRÓPRIO protocolo

Aqui não basta expor: é preciso implementar no fork.

| capacidade | estado no fork | nota |
|---|---|---|
| **Fixar mensagem** | `PinInChat` não existe | Baileys tem; teríamos de o construir |
| **Favoritar (star)** | `StarMessage` não existe | idem |
| **Silenciar conversa** | `MuteChat` não existe | só existe `/newsletter/mute` |
| **Comunidades** | uma única menção | essencialmente não suportado |

Reproduzir:

```
for k in PinInChat StarMessage MuteChat Community; do
  echo -n "$k: "; grep -rl "$k" internal/wa-noise/core/*.go | wc -l
done
```

## O que NÃO se faz, e porquê — decisões já medidas

Não voltar a abrir sem dado novo. Cada uma foi medida em campo e registada.

| item | veredito | evidência |
|---|---|---|
| **`ALBUM_IMAGE`** | não se faz (F221) | três variantes, todas recusadas pelo cliente; o álbum é `MessageAssociation`/`MEDIA_ALBUM`, outro mecanismo |
| **PIX / pagamento** | não se faz (F213) | `payment_info` + `pix_static_code` recusado pelo cliente; e mesmo a renderizar, **não processa pagamento** |
| **Catálogo, produtos, Flows** | fora de alcance | superfície exclusiva da Cloud API com WABA |

## Nota de método

O padrão que se repete neste projeto: **o `200` do servidor não prova nada**. O
carrossel, o álbum e o PIX devolveram todos `200` com `message_id` — dois
renderizaram, um não. Toda capability nova de envio precisa de fotografia do
cliente antes de ser dada por entregue.

## Estimativas

### Em que unidade estas estimativas estão

Uma **sessão de worker** = um worker do orca, com packet, a levar a capability
de ponta a ponta: domínio, porta, caso de uso, handler, rota, devui, testes com
controlo negativo executado, e baselines atualizadas.

A base é o que ESTE projeto realmente produziu, não uma média da indústria:

| entrega | ficheiros | sessões |
|---|---|---|
| CAP-46A reply-to: fundação + 1 rota | 14 | 1 |
| CAP-46B reply-to: 12 rotas restantes | 57 | 1 |
| CAP-47 menções: 8 rotas | 47 | 1 |
| CAP-23 rota do carrossel | 18 | 1 |
| F212 trim em duas bases | 10 | 1 |

O padrão é estável: **uma capability nova cabe numa sessão**, mesmo quando toca
dezenas de ficheiros, porque o trabalho é repetitivo depois de a forma estar
decidida. O que NÃO cabe é a decisão de forma — foi por isso que o CAP-46 se
partiu em duas.

Some-se a cada uma:
- **`make check`**: 15 a 60 min, conforme a carga da máquina.
- **Verificação em campo**: uma ronda de fotografia, dependente de humano.
  Não é tempo de máquina; é latência.

### Prioridade 1 — temos protocolo, falta rota

| # | item | sessões | notas de risco |
|---|---|---|---|
| 1 | **Votar em enquete** | 1 | `BuildPollVote` já existe e não tem consumidor. A rota é simétrica ao `/chat/send/poll`. Risco baixo. |
| 2 | **Encaminhar mensagem** | 1 | Campos `IsForwarded`/`ForwardingScore` já no protobuf. Decisão a tomar: encaminhamos por id de mensagem (exige tê-la) ou por conteúdo repetido? |
| 3 | **Desaparecimento por conversa** | 1 | `SetDisappearingTimer` já usado internamente. Espelha `/group/ephemeral`. Risco baixo. |
| 4 | **Status/stories com média** | 1–2 | O único desta lista com custo escondido: resolução da lista de destinatários e modos de falha próprios. |

### Prioridade 2 — falta no protocolo

| # | item | sessões | notas de risco |
|---|---|---|---|
| 5 | **Fixar mensagem** | 2–3 | Não existe no fork. Exige protobuf + envio + teste, e a fronteira do ADR-001 (não alargar a fachada) entra em jogo. |
| 6 | **Favoritar (star)** | 2–3 | Idem. É estado local do cliente; confirmar se sequer viaja no wire. |
| 7 | **Silenciar conversa** | 2–3 | Idem. Provavelmente app-state, não mensagem — outro mecanismo. |
| 8 | **Comunidades** | ≥4 | Praticamente ausente do fork. Não estimar a sério sem levantamento próprio. |

### O que NÃO entra em estimativa

`ALBUM_IMAGE` (F221), PIX (F213), catálogo/produtos/Flows. Os dois primeiros
foram **refutados por medição em campo**; o terceiro exige WABA. Reabrir só com
dado novo.

### Aviso sobre estas estimativas

São de ESFORÇO, não de calendário. Três coisas que este projeto mostrou que
alargam o prazo, e nenhuma aparece na tabela:

1. **A decisão de forma**, quando não é óbvia. O CAP-46 precisou de uma sessão
   só para decidir se reutilizava `EditContextInfo`.
2. **A ronda de fotografia**, que depende de humano e de o telemóvel estar à
   mão.
3. **O que a medição refuta.** A F222 refutou uma premissa DEPOIS de integrada;
   o álbum consumiu três variantes antes de se saber que era outro mecanismo.
   Estimar como se a primeira hipótese fosse a certa é o erro clássico aqui.
