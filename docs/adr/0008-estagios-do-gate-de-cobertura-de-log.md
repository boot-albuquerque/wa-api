# ADR-0008 — Estágios do gate de cobertura de log: advisory, ratchet, floor

**Data**: 2026-08-20
**Estado**: aceito (documenta mecanismo já em produção desde as fases F9–F16)

## Contexto

Este ADR é escrito **depois** do mecanismo que descreve. Ele não decide nada
novo: registra uma decisão que já estava tomada, implementada e sendo citada —
mas nunca escrita.

O número `0008` estava vago porque `Makefile:243`, `Makefile:255` e
`cmd/logcov/main_test.go:96` já citavam "ADR-008" como justificativa do estágio
do gate, e o documento nunca existiu. A citação era impressa em **toda**
execução de `make check`:

```
log-coverage: estagio do gate = ratchet (ADR-008)
```

Registrado como F162 no `HOUSEKEEP.md`, com o `0008` deliberadamente reservado
enquanto o ADR do S3 saía como `0009`.

## Decisão

O alvo `log-coverage-gate` lê a chave `stage` de `.log-coverage-baseline` e
opera em um de três estágios declarados:

| estágio | comportamento |
|---|---|
| `advisory` | mede, IMPRIME, e **nunca** falha |
| `ratchet` | falha se qualquer uma das quatro métricas regredir |
| `floor` | idem `ratchet` — ver "O que a implementação REALMENTE faz" |

Quatro métricas são guardadas, cada uma com sua chave no baseline:

| métrica | chave | direção que REPROVA |
|---|---|---|
| cobertura de função | `min_func_coverage` | caiu abaixo do piso |
| cobertura de caminho de erro | `min_errpath_coverage` | caiu abaixo do piso |
| funções elegíveis | `min_eligible` | **encolheu** (denominador diminuiu) |
| anotações de isenção | `max_exempt_annotations` | **subiu** acima do teto |

A terceira é a menos óbvia e a mais importante: o gate reprova quando o
DENOMINADOR encolhe. Sem ela, remover funções elegíveis inflaria a percentagem
de cobertura sem que nada tivesse sido testado.

### Fail-closed, em três pontos

1. **`stage` ausente ou inválido não reverte para `advisory`** — o gate falha.
   Reverter em silêncio desarmaria as quatro travas de uma vez, e uma linha
   apagada por acidente não pode ter esse poder.
2. **`logcov` falhando ao medir não vira aprovação.** A mensagem do próprio
   Makefile é a justificativa: "a metrica esta' cega, o que nao e' o mesmo que
   100%: ausencia de medicao nao vira aprovacao."
3. **Valor ausente no JSON ou no baseline** reprova em vez de comparar com
   vazio.

### O baseline é impresso em toda execução, por desenho

O alvo faz `grep -E '^(stage|min_|max_)'` do baseline e imprime, mesmo quando
passa. É deliberado: quem lê o log de CI vê contra o que está sendo comparado,
sem abrir o arquivo. Um gate cujo critério é invisível é um gate em que não se
confia.

### Ratchet-UP é responsabilidade do PR que sobe a métrica

Quando a cobertura SOBE, o alvo imprime `ATENCAO: ... Suba min_* para N neste
PR`. Isso é advisory por natureza — não há regressão a reprovar. Mas ignorá-lo
acumula folga, e folga é catraca desligada na faixa que importa. Foi o que a
[F171] mediu: piso em 840 contra medição de 859 permitia REMOVER cobertura até
84,0% sem o gate reclamar, por duas rodadas.

**Regra prática**: subir piso é mudança de gate e vai em commit PRÓPRIO,
separado da correção que o gate julga. E o valor se MEDE na hora de aplicar —
copiar o número de um relatório anterior commita a catraca já com folga.

## O que a implementação REALMENTE faz, e onde diverge deste modelo

Dois pontos medidos ao escrever este ADR, registrados aqui porque um ADR que
descreve a intenção e omite a implementação é pior que nenhum:

### 1. `floor` é hoje indistinguível de `ratchet`

O `case` de validação aceita `advisory|ratchet|floor`, mas a lógica de falha é
uma só condição:

```make
if [ "$$stage" != "advisory" ]; then ... fi
```

Não existe nenhuma linha em `Makefile`, `cmd/logcov/` ou `scripts/` que trate
`floor` diferente de `ratchet`. O modelo tem três estágios; a implementação tem
**dois**: "imprime" e "reprova".

Isso não é defeito hoje — `stage=ratchet` é o valor em uso, e `floor` nunca foi
selecionado. É dívida de desenho: o dia em que alguém declarar `floor`
esperando comportamento mais estrito, receberá exatamente o de `ratchet`, sem
aviso. Quem for implementar a distinção deve escrevê-la aqui antes de escrevê-la
no Makefile.

### 2. O rótulo `piso exato` de `eligible` está errado

A linha impressa diz:

```
eligible         = N (piso exato M)
```

mas a checagem é `[ "$$eligible" -lt "$$min_eligible" ]` — ou seja, CRESCER é
permitido, e deve ser mesmo: a CAP-38 e a CAP-40 cresceram o denominador
legitimamente ao acrescentar funções. É um piso, não um valor exato. O rótulo
promete uma rigidez que o gate não tem, e um rótulo mais estrito que o
comportamento treina quem lê a duvidar da mensagem em vez do código.

## Consequências

- As citações em `Makefile:243`, `Makefile:255` e `cmd/logcov/main_test.go:96`
  passam a apontar para um documento que existe.
- O slot `0008` deixa de ser um buraco na numeração dos ADRs.
- Os dois desvios acima ficam registrados como dívida CONHECIDA em vez de
  surpresa: `floor` sem comportamento próprio, e o rótulo `piso exato` que
  descreve mal a checagem.

## Referências

- `Makefile`, alvo `log-coverage-gate`
- `.log-coverage-baseline` — as quatro chaves e o `stage`
- `cmd/logcov/` — o medidor; `cmd/logcov/testdata/eligible.golden` — o conjunto
  elegível versionado, cujo diagnóstico de divergência foi consertado na [F154]
- `HOUSEKEEP.md`: [F162] (este ADR faltando), [F171] (catraca com folga),
  [F154] (golden ancorado em número de linha)
