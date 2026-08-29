# Contrato de autoria da especificação OpenAPI

Este ficheiro é o que cada autor de um grupo de rotas tem de seguir. Não é
estilo: é o que faz doze ficheiros escritos por pessoas diferentes lerem-se
como um documento só.

## Onde se escreve

```
api/openapi/base.yaml            info, servers, tags, securitySchemes, componentes PARTILHADOS
api/openapi/paths/<grupo>.yaml   os caminhos do seu grupo — e SÓ do seu grupo
api/openapi/schemas/<grupo>.yaml os esquemas próprios do seu grupo
api/openapi/evidencias.tsv       a marca de evidência de cada rota
```

**Não cole ✅ 🟡 ❌ ⬜ no `summary`.** O marcador é aplicado pelo
`cmd/openapidoc` a partir de `evidencias.tsv`. Colado no título, ele duplica
assim que alguém reescrever o texto, e desatualiza-se sem ninguém dar por isso.

Rota nova em `paths/` **exige** linha em `evidencias.tsv` — o gerador falha sem
ela. E uma linha para rota que já não existe também o faz falhar: entrada
obsoleta é o que mantém uma rota removida com ar de medida.

**Nunca edite `pkg/presentation/http/apidocs/openapi.yaml`.** É gerado por
`go run ./cmd/openapidoc`, e há teste que o recusa se estiver desactualizado.

**Nunca edite `base.yaml`** se o seu trabalho é um grupo de rotas. Precisa de
um componente partilhado novo? Peça — dois autores a mexer no mesmo ficheiro é
exactamente o que esta divisão existe para evitar.

Colisão de caminho entre dois ficheiros é ERRO do gerador, não último-a-escrever
ganha.

## A forma de uma operação

```yaml
/chat/send/text:
  post:
    tags: [Envio de mensagens]          # obrigatório, e tem de existir em base.yaml
    summary: Enviar uma mensagem de texto   # orientado à AÇÃO, não "POST /chat/send/text"
    description: |                      # obrigatório, mínimo 80 caracteres
      O que faz.

      **Quando usar**: ...

      **Regras**: as que o consumidor tem de respeitar — validações reais,
      lidas no use case, não imaginadas.

      **Autenticação**: token de sessão. (ou: token de administração / nenhuma)
    security:
      - TokenSessao: []                 # explícito SEMPRE, mesmo quando é o padrão
    requestBody:
      required: true
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/PedidoEnvioTexto'
          example:                      # exemplo COMPLETO e realista
            Phone: 554192421234@s.whatsapp.net
            Body: Bom dia! O seu pedido saiu para entrega.
    responses:
      '200':
        description: Mensagem aceite pelo WhatsApp.
        content:
          application/json:
            schema:
              allOf:
                - $ref: '#/components/schemas/Envelope'
                - type: object
                  properties:
                    data:
                      $ref: '#/components/schemas/ResultadoEnvio'
            example:
              code: 200
              data: {message_id: 3EB0A4B2AFD45E625C0917, timestamp: 1787747900, status: sent}
              success: true
      '400':
        $ref: '#/components/responses/CorpoInvalido'
      '401':
        $ref: '#/components/responses/NaoAutorizado'
```

## As sete regras

1. **Idioma**: tudo o que a pessoa lê em pt-BR — summary, description,
   descrições de campo e de resposta. Nomes técnicos do contrato (`Phone`,
   `groupJID`, `message_id`, cabeçalhos) ficam **como estão**, porque são o que
   viaja no fio.

2. **Nunca invente.** Estado, campo, erro, regra e exemplo saem da
   implementação. Se não conseguir determinar algo com segurança, escreva na
   descrição: *"Não determinado com segurança pela implementação atual."*
   É melhor do que uma frase plausível e errada.

3. **A struct de domínio NÃO é o contrato da rota.** Onze manipuladores
   declaram structs anónimas próprias; onde o nome diverge, é a do manipulador
   que manda (HOUSEKEEP F267). Leia o manipulador, e depois o use case, e
   confirme com uma chamada real quando o ambiente permitir.

4. **Erros: só os que acontecem.** Leia o use case e o manipulador e documente
   as recusas que existem no código. Nada de `403` por convenção. `error` é
   SEMPRE o objecto `Erro`, em qualquer estado: o que não passa pela taxonomia
   `apperr` recebe o código genérico do estado (`invalid_request`,
   `unauthorized`, `not_found`, `internal_error`, ...). A forma antiga, com
   `error` em texto, era a F266 e foi removida.

5. **Exemplos coerentes entre si.** Use sempre os mesmos identificadores:
   - contacto `554192421234@s.whatsapp.net` / LID `90937376170214@lid`
   - grupo `120363411669320145@g.us`
   - comunidade `120363430034334401@g.us`
   - canal `120363411025775186@newsletter`
   - mensagem `3EB0A4B2AFD45E625C0917`
   Nada de `string`, `foo` ou `123`.

6. **Reutilize.** As respostas `NaoAutorizado`, `SemSessao`, `CorpoInvalido`,
   `RecusadoPeloWhatsApp` e `ErroInterno`, e os esquemas `Envelope`, `Erro`,
   `ResultadoEnvio`, `Detalhes`, `SucessoMensagem` e `ContextoResposta` já
   existem em `base.yaml`. Só crie esquema novo para o que é seu.

7. **Meça.** Antes de escrever a resposta de uma rota, chame-a. O que o
   servidor devolve venceu a struct em pelo menos dez sítios já medidos.

## Antes de commitar

```
go run ./cmd/openapidoc                                   # regenera
go test ./pkg/bootstrap/ -run TestOpenAPI                 # os quatro gates
go test ./pkg/presentation/http/apidocs/                  # o servidor da página
```

Commite o `openapi.yaml` regenerado junto com as suas fontes.
