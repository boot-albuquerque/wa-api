# Checklist de pré-release (E2E manual)

Executado manualmente contra staging, com um dispositivo WhatsApp real pareado,
antes de qualquer release. Não substitui os testes automatizados (golden files,
`go test ./...`) — cobre o caminho que eles não alcançam: um dispositivo de
verdade, uma sessão de verdade, entrega de webhook de verdade.

## Pré-requisitos

- [ ] Ambiente de staging no ar, apontando para um banco vazio ou descartável
- [ ] Um número de WhatsApp de teste disponível para parear
- [ ] Um endpoint de webhook de teste capaz de mostrar headers recebidos (ex: `webhook.site`, ou um listener local com log de headers)
- [ ] Acesso ao bucket S3 (ou equivalente) configurado para o ambiente de staging
- [ ] Acesso ao RabbitMQ (management UI ou `rabbitmqadmin`) do ambiente de staging

## Passos

1. **Parear**
   - [ ] `POST /session/connect` para o usuário de teste
   - [ ] QR code exibido/retornado; escanear com o dispositivo de teste
   - [ ] Sessão reporta conectada (`GET /session/status` ou equivalente)

2. **Enviar texto**
   - [ ] `POST /chat/send/text` para um contato de teste
   - [ ] Mensagem chega no dispositivo de teste

3. **Enviar mídia**
   - [ ] `POST /chat/send/image` (ou áudio/vídeo/documento) com um arquivo de teste
   - [ ] Mídia chega no dispositivo de teste

4. **Confirmar objeto no S3 com ACL privada**
   - [ ] O objeto da mídia enviada aparece no bucket configurado
   - [ ] O objeto **não** é publicamente acessível via URL direta (sem presign) — confirma que a ACL pública foi removida (Fase 5c)
   - [ ] A URL retornada pela API (presigned) funciona e expira conforme esperado

5. **Confirmar webhook entregue com header `x-hmac-signature`**
   - [ ] Um evento de mensagem recebida dispara o webhook configurado
   - [ ] O corpo da entrega recebida no endpoint de teste contém o header `x-hmac-signature`
   - [ ] A assinatura é válida para o `globalhmackey`/segredo configurado do usuário

6. **Confirmar publicação no RabbitMQ**
   - [ ] O evento de mensagem recebida (ou de erro de webhook, se aplicável) aparece na fila configurada
   - [ ] A mensagem na fila tem o payload esperado (JSON válido, campos mínimos presentes)

7. **Desconectar**
   - [ ] `POST /session/disconnect` (ou `/session/logout`) para o usuário de teste
   - [ ] Sessão reporta desconectada
   - [ ] Nenhum log de pânico ou erro inesperado durante a desconexão

## Registro

Anexar ao PR de release (ou ao ticket de deploy): data/hora da execução, quem
executou, resultado de cada item (✅/❌), e link para os logs relevantes de
staging caso algum item falhe.
