# ADR-0002: Vendorizar o núcleo do `whatsmeow` em vez de reimplementar protocolo do zero

- **Status**: accepted (amenda o ADR-0001; aprovado 2026-08-06)
- **Data**: 2026-08-06
- **Amenda**: [ADR-0001](0001-native-whatsapp-protocol-roadmap.md), especificamente a alternativa
  descartada "Fork completo do `whatsmeow` agora" e o critério de partida
  "não é razoável reimplementar Signal/Noise do zero por capricho"

## Contexto

O ADR-0001 tratou "protocolo nativo" como sinônimo de "escrever `waBinary`,
Noise e Signal do zero", e descartou fork completo do `whatsmeow` como
alternativa por causa disso — custo de manutenção de toda a superfície de
criptografia sem evidência de que a dor justificasse.

Essa premissa está incompleta. `waBinary` (encode/decode de nós) e o
handshake Noise fazem parte do próprio módulo `go.mau.fi/whatsmeow`
(pacotes `binary/` e `socket/`), sob licença **MPL-2.0** — que permite
cópia e modificação, exigindo só que arquivos `.go` efetivamente
modificados continuem sob MPL-2.0 com aviso de licença preservado; arquivos
não tocados podem ser copiados como estão. O protocolo Signal (double
ratchet) nem é código do `whatsmeow` — é o módulo separado
`go.mau.fi/libsignal`, já uma implementação Go independente, pronta pra
vendorizar.

Medição feita nesta sessão (`go.mau.fi/whatsmeow v0.0.0-20260516102357-8d3700152a69`,
`go.mau.fi/libsignal v0.2.1`):

| Peça | Linhas | Papel |
|---|---:|---|
| `whatsmeow/binary` | 1.223 | encode/decode `waBinary` |
| `whatsmeow/socket` | 548 | handshake Noise + framing |
| `libsignal` (módulo separado) | 10.106 | criptografia de sessão (Signal) |
| `whatsmeow/store` | 1.156 | persistência de device/chaves |
| `whatsmeow/types` | 1.342 | tipos de domínio (JID, eventos) |
| `whatsmeow/appstate` | 1.220 | sync de contatos/config |
| `whatsmeow` (raiz — `client.go`, `message.go`, `group.go`, `user.go`...) | 14.798 (39 arquivos) | orquestração de alto nível — hoje é o que `SessionProviderAdapter` chama via `whatsmeow.Client` |
| `whatsmeow/proto` (protobuf gerado — `waE2E`, `waHistorySync` etc.) | 99.205 | serialização dos payloads |
| **Total do módulo** | **~124.000** | |

Ou seja: "reimplementar protocolo nativo do zero" nunca foi a única
leitura possível do objetivo do ADR-0001. **Vendorizar o núcleo
(`binary`+`socket`+`libsignal`+`store`+`types`+`proto`) e reescrever só a
camada de orquestração (o equivalente a `client.go` e afins, ~14,8k
linhas) diretamente no nosso repo** entrega o mesmo resultado que motivou
o ADR-0001 — não depender do ritmo de release/PR upstream — sem exigir
escrever criptografia do zero. É trabalho real (reescrever ~15k linhas de
orquestração é grande), mas de escala completamente diferente da
"reimplementação big-bang do protocolo" que o ADR-0001 rejeitou.

## Decisão

Substituir a estratégia "superfície-por-superfície, nativização como
último recurso" do ADR-0001 por: **vendorizar os pacotes de baixo nível do
`whatsmeow` (`binary`, `socket`, `store`, `types`, `proto`) e o módulo
`libsignal`** para dentro do repo `wa-api` (ou um módulo Go interno
próprio), preservando a licença MPL-2.0 nos arquivos copiados, e
**reescrever a camada de orquestração de alto nível** (hoje concentrada em
`whatsmeow.Client` e chamada via `SessionProviderAdapter`) usando essas
peças diretamente — customizando o que for necessário, arquivo por
arquivo, só onde houver motivo real.

Isso é possível **hoje sem retrabalho de arquitetura**, porque a sessão já
está atrás do port `port.SessionProvider`/`port.Session`
(`pkg/application/contracts/session_provider.go`) — a implementação atual
em `pkg/infra/whatsmeow/session_provider_adapter.go` é exatamente o ponto
de substituição. Uma nova implementação (`SessionProviderVendored` ou
nome equivalente) passa a satisfazer o mesmo port, sem tocar em
`SessionOrchestrator`, handlers, ou qualquer usecase.

## Racional

- **Resolve a dor original do ADR-0001** (correções não esperam merge
  alheio) sem o custo que o ADR-0001 temia (reimplementar cripto do zero)
  — a criptografia vem pronta e testada em produção via `libsignal` e
  `whatsmeow/socket`, só passa a viver no nosso repo em vez de importada
  como dependência externa pinada em pseudo-versão.
- **Legal**: MPL-2.0 permite isso — copiar e modificar por arquivo,
  mantendo aviso de licença nos arquivos tocados. Não é GPL (que exigiria
  relicenciar o projeto inteiro).
- **A camada que de fato precisa ser reescrita é pequena relativamente ao
  todo**: ~14,8k linhas de orquestração contra ~124k linhas do módulo
  inteiro (o resto — `proto/`, `binary/`, `socket/`, `libsignal`, `store`,
  `types` — é copiado quase sem alteração, só onde a dor concreta pedir
  customização).
- **Consistente com o port já existente**: não é preciso reabrir a Fase 2
  do plano de arquitetura multi-sessão (`.omc/plans/native-multisession-architecture.md`)
  — o ponto de encaixe (`SessionProvider`) já foi desenhado justamente
  pra isso.

## Alternativas reconsideradas

- **Manter o critério do ADR-0001 (nativizar só com dor real medida,
  reimplementando protocolo do zero por superfície)**: mais conservador,
  adia indefinidamente qualquer ganho de independência do upstream — o
  ADR-0001 já registrava esse critério como "nenhuma ação imediata".
- **Fork completo ingênuo (clonar o repo `whatsmeow` inteiro, sem
  seletividade)**: descartado — vendorizar seletivamente
  (`binary`/`socket`/`store`/`types`/`proto`/`libsignal`) e reescrever só
  a orquestração é mais barato de manter do que carregar as 124k linhas
  inteiras, incluindo partes (ex: `appstate`, funcionalidades não usadas)
  que talvez nunca precisem de customização.

## Consequências

- Perde-se atualização automática de segurança/correção do `whatsmeow`
  upstream nos pacotes vendorizados — merges futuros do upstream precisam
  ser portados manualmente (mitigação possível: rastrear os commits
  upstream dos pacotes vendorizados e revisar periodicamente).
- Licença MPL-2.0 deve ser respeitada arquivo a arquivo — aviso de
  licença preservado nos arquivos copiados/modificados; documentar a
  origem (commit/tag do `whatsmeow` no momento da cópia) para rastreio.
- A primeira implementação real (reescrever a camada de orquestração
  contra os pacotes vendorizados) precisa de planejamento próprio — este
  ADR só formaliza a estratégia, não é o plano de execução.
- ADR-0001 permanece válido como filosofia geral (migração incremental,
  ports antes de troca de implementação, zero regressão observável) —
  esta ADR amenda especificamente a rejeição de fork e o critério de
  "não vale reimplementar cripto".

## Próximos passos

1. ~~Confirmar este ADR como `accepted`.~~ Feito (2026-08-06).
2. `ralplan` para o plano de execução: qual pacote vendorizar primeiro,
   como estruturar o vendoring (módulo Go interno vs `vendor/` vs
   submódulo), e o desenho da nova implementação de `SessionProvider`
   contra os pacotes vendorizados.
