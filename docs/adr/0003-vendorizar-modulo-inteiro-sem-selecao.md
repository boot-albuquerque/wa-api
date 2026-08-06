# ADR-0003: Vendorizar o módulo whatsmeow inteiro (sem seleção de pacotes), `libsignal` externo

- **Status**: accepted (amenda o ADR-0002, aprovado 2026-08-06)
- **Data**: 2026-08-06
- **Amenda**: [ADR-0002](0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md),
  especificamente a alternativa reconsiderada "Fork completo ingênuo" (rejeitada)
  e a decisão original de vendorizar `libsignal`

## Contexto

Durante o planejamento de execução do ADR-0002
(`.omc/plans/vendor-whatsmeow-native-fork.md`), o levantamento técnico
mostrou dois pontos em que a execução real diverge do que o ADR-0002
decidiu:

1. **O ADR-0002 rejeitou "fork completo ingênuo"** com a justificativa de
   que vendorizar seletivamente (`binary`/`socket`/`store`/`types`/`proto`/
   `libsignal`) é mais barato de manter do que carregar as ~124k linhas
   inteiras do módulo, "incluindo partes (ex: `appstate`, funcionalidades
   não usadas) que talvez nunca precisem de customização".

   Na prática, essa seletividade não é possível sem redesenhar a
   fronteira que o ADR-0002 queria evitar tocar: `store/` importa
   diretamente `whatsmeow/proto/waAdv` e `whatsmeow/proto/waCompanionReg`;
   a raiz (`client.go` e afins) importa `appstate` para sync de
   contatos/config — capacidade já em uso pelo `wa-api`
   (`whatsmeow/appstate` é importado em 6 arquivos do wa-api hoje — 2 em
   `pkg/bootstrap` e 4 em `pkg/infra/whatsmeow`). Ou
   seja, "vendorizar seletivamente, deixando `appstate` de fora" não é
   uma opção real sem quebrar consumidores existentes — teria que
   recriar exatamente a costura entre pacotes que copiar o módulo
   inteiro evita.

   A economia que o ADR-0002 esperava ("mais barato de manter") também
   não se sustenta: separar pacotes que se importam mutuamente dentro do
   mesmo módulo upstream não reduz linhas de manutenção — só adiciona
   trabalho de manter a costura entre "o que vendorizamos" e "o que
   ainda é externo", exatamente o tipo de fronteira frágil que motivou
   preservar a API pública do `Client` no primeiro lugar (ver ADR-0002,
   seção Decisão).

2. **O ADR-0002 decidiu vendorizar `libsignal` junto** ("vendorizar os
   pacotes de baixo nível do whatsmeow ... e o módulo `libsignal`").
   Mas `libsignal` é um **módulo Go publicado separadamente**
   (`go.mau.fi/libsignal`), sem nenhum import reverso para `whatsmeow` —
   não há razão técnica pra copiá-lo pra dentro do fork; ele continua
   funcionando como dependência normal do `go.mod`, do mesmo jeito que
   qualquer outra lib de terceiros que o `wa-api` já usa. Vendorizá-lo
   sem necessidade só adicionaria ~10k linhas de manutenção sem nenhum
   ganho (não resolve nenhuma dor do ADR-0001 original — o motivador
   nunca foi sobre `libsignal`, e sim sobre correções em `client.go` e
   afins não esperarem PR/release upstream).

## Decisão

Amendar o ADR-0002 em dois pontos:

1. **Vendorizar o módulo whatsmeow inteiro** (raiz + `appstate` + `argo` +
   `binary` + `proto` + `socket` + `store` + `types` + `util`), sem
   seleção de pacotes — a seletividade que o ADR-0002 propunha não é
   viável sem recriar a fronteira que preservar a API pública do `Client`
   já resolve de graça.
2. **`libsignal` permanece dependência externa normal**, não vendorizado
   — é módulo separado sem import reverso, e vendorizá-lo não teria
   nenhum efeito sobre o motivador do ADR-0001/0002 (não depender de
   release upstream do `whatsmeow`).

## Racional

- **A alternativa "fork completo ingênuo" do ADR-0002 foi rejeitada por
  um custo que não se concretiza**: a suposta economia de vendorizar
  seletivamente exige recriar a costura entre pacotes que hoje o próprio
  módulo whatsmeow já resolve internamente (imports entre `store`,
  `appstate`, `proto`, raiz) — não copiar essa costura não a elimina, só
  transfere o trabalho de mantê-la pro `wa-api`.
- **`appstate` já está em uso** (`pkg/bootstrap`, 6 arquivos) — não é
  "funcionalidade não usada" como o ADR-0002 supôs; excluí-lo do
  vendoring quebraria consumidores existentes.
- **`libsignal` nunca foi parte do problema**: o motivador de todo o
  roadmap (ADR-0001) é não depender do ritmo de release/PR do
  `whatsmeow`. `libsignal` é outro projeto, com seu próprio ciclo de
  release, e vendorizá-lo não muda em nada a dependência do `whatsmeow`.

## Alternativas reconsideradas

- **Manter a seletividade do ADR-0002** (vendorizar só `binary`/`socket`/
  `store`/`types`/`proto`, deixar `appstate`/raiz de fora ou como
  dependência externa parcial): descartada — tecnicamente inviável sem
  quebrar `appstate`, e não reduz custo de manutenção como o ADR-0002
  supunha.
- **Vendorizar `libsignal` como o ADR-0002 original propunha**:
  descartada — sem benefício mensurável, custo puro (10k linhas a mais
  pra manter sincronizadas sem necessidade).

## Consequências

- O plano de execução (`.omc/plans/vendor-whatsmeow-native-fork.md`)
  reflete esta decisão diretamente — módulo inteiro, `libsignal` externo.
- ADR-0002 permanece válido em sua decisão central (vendorizar em vez de
  reimplementar do zero, preservando a API pública do `Client`) — esta
  ADR só corrige o escopo da seleção de pacotes e a decisão sobre
  `libsignal`.
- Nenhuma mudança na filosofia geral do ADR-0001 (migração incremental,
  ports antes de troca de implementação, zero regressão observável).
