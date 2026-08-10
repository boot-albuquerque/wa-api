# Brief de UX — tempo de ocupação na tela de ritmo de campanha

**Para:** design de produto (UX/UI)
**Origem:** ADR-0007 (custo por sessão) + medições em
`scripts/chromium-study/RELATORIO-FASE-{4B,4C,5}.md`
**Status:** proposta de mudança, não implementada

---

## 1. O contexto técnico, sem jargão

Cada número de WhatsApp conectado ocupa **um navegador inteiro** enquanto está
ativo — não é uma conexão leve, é um Chrome rodando com a conta aberta. Medido:
**cerca de 1 GB de memória por número ativo**, e **um número por navegador** (dois
não compartilham).

A consequência que interessa ao design: **o que custa não é a quantidade de
mensagens, é quanto tempo o número fica acordado.** Quando a campanha termina, o
navegador é desligado e aquele número para de consumir.

Por isso a tela de ritmo é, sem ter sido desenhada para isso, **o painel onde se
decide o consumo de infraestrutura da plataforma inteira**.

## 2. O que a tela comunica hoje, e o que falta

Hoje a escolha é apresentada em dois eixos: **velocidade** e **risco de bloqueio**.
Ambos corretos e bem resolvidos — o card "Equilibrado" como recomendado, a barra
de risco, o texto explicando a aleatoriedade do intervalo.

Falta um terceiro eixo, que hoje está invisível. Para **as mesmas 936 mensagens**:

| ritmo | tempo de envio | ocupação relativa |
|---|--:|--:|
| Rápido | 5 h 44 min | 1,0× |
| Equilibrado | 21 h 23 min | **3,7×** |
| Cuidadoso | 50 h 26 min | **8,8×** |

O mesmo trabalho, entregue igual, com **quase 9× de diferença** em tempo de
número ocupado entre as pontas.

O detalhe que torna isso urgente: **"Cuidadoso" é o modo indicado para números
novos ou recém-recuperados** — ou seja, exatamente os clientes que acabaram de
entrar. O caminho mais recomendado no onboarding é o mais caro por um fator de
quase nove, e ninguém na tela consegue ver isso.

## 3. Peça 1 — "Tempo de ocupação" no painel Previsão

O painel Previsão já tem início, término, janelas, ritmo real e volume. Falta uma
linha, e ela deve ter o mesmo peso visual das outras:

```
⧗  OCUPAÇÃO DO NÚMERO
   21 h 23 min acordado  ·  em 3 blocos
```

Três decisões de conteúdo que importam:

- **"Ocupação" ou "tempo acordado", nunca "custo".** Ver §6.
- **Distinguir tempo decorrido de tempo ocupado.** A campanha termina em 3 dias
  corridos, mas o número só fica acordado ~21 h — ele dorme entre as janelas.
  Hoje "conclui em 21 h 23 min" e "término Ter 21/07" convivem na tela sem
  explicar que são coisas diferentes. Essa é uma confusão que já existe e que
  esta linha pode resolver de passagem.
- **Mostrar o número de blocos.** Ver §5, tem consequência de confiabilidade.

## 4. Peça 2 — o terceiro eixo nos cards de ritmo

Cada card já tem intervalo, pausa, conclusão e barra de risco. Acrescentar uma
linha de ocupação, no mesmo formato das existentes:

```
⧗  1 a cada 45–90 s
▥  pausa de 10 min a cada 40
⌛  conclui em 21 h 23 min
◐  21 h de número ocupado          ← nova
```

**Comparação relativa ajuda mais que o valor absoluto.** No card "Cuidadoso",
algo como *"50 h — 2,4× o Equilibrado"* comunica a escolha melhor do que "50 h"
sozinho, porque a decisão do usuário é comparativa por natureza.

O ponto de design a resolver: **isso não pode virar um quarto medidor competindo
com a barra de risco.** Risco continua sendo o eixo principal — é o que protege o
número do cliente. Ocupação é informação de contexto, e a hierarquia visual
precisa dizer isso.

## 5. Nota de confiabilidade — quantos blocos, não só quanto tempo

Cada bloco (janela) implica **dormir e acordar** o navegador. Medimos que o
ciclo de sono/despertar, quando feito pelo caminho errado, degrada a sessão a
partir da **4ª repetição** — o número desconecta e pede QR de novo.

O caminho correto já está identificado e corrigido do nosso lado, mas a
implicação de produto permanece: **mais janelas = mais ciclos = mais superfície
de falha**. Uma campanha em 3 janelas gasta 3 ciclos.

Para o design isso significa que **"3 janelas de 09:00 às 18:00" é informação de
confiabilidade, não só de agenda**, e justifica estar visível. Não sugiro alarme
— sugiro que o número de blocos não fique escondido num texto secundário.

## 6. O que NÃO fazer

**Não exibir custo em reais para o cliente.** O plano é flat (R$89/número). O
cliente **não paga mais** por escolher "Cuidadoso" — quem paga somos nós. Mostrar
"esta campanha custa R$X" seria falso, e transformaria uma escolha de segurança
numa escolha financeira que não existe para ele.

**Não usar a ocupação para empurrar o modo mais rápido.** "Rápido" tem risco
alto de bloqueio; empurrar o cliente para lá para economizar infraestrutura
troca um custo nosso por um dano dele. A hierarquia correta continua sendo
**risco primeiro, ocupação como contexto**.

**Não transformar em paywall nesta etapa.** Se um dia houver limite de
franquia, aí sim a ocupação vira unidade de cobrança — e o desenho muda. Hoje
não é o caso, e antecipar isso cria fricção sem contrapartida.

## 7. Contrato de dados — o que pedir à engenharia

A tela precisa de dois campos que hoje provavelmente não existem no cálculo de
previsão:

| campo | descrição |
|---|---|
| `occupancy_seconds` | soma do tempo em que o número ficará acordado, **sem** contar as pausas entre janelas |
| `wake_cycles` | quantas vezes o número dormirá e acordará (= número de janelas) |

Ambos são deriváveis do que já alimenta a previsão — não exigem medição nova,
só serem expostos.

E um pedido de instrumentação, que é pré-requisito para qualquer evolução deste
desenho: **session-hours realizadas por tenant**. Hoje só temos estimativa de
plano; o consumo real nunca foi medido. Enquanto ele não existir, qualquer
franquia, alerta ou tier baseado em ocupação seria construído sobre suposição.

Sugestão de sequência: **primeiro um painel interno** (admin/operação) mostrando
ocupação realizada por tenant, depois — só se os dados justificarem — algo
voltado ao cliente.

## 8. Critério de aceite

- [ ] Um usuário consegue responder "quanto tempo meu número fica ocupado?" sem
      abrir documentação
- [ ] A diferença de ocupação entre "Rápido" e "Cuidadoso" é percebida na
      comparação dos cards, sem precisar somar nada mentalmente
- [ ] Tempo decorrido e tempo ocupado não são mais confundíveis
- [ ] A barra de risco continua sendo o elemento dominante da decisão
- [ ] Nenhum valor monetário aparece para o cliente
