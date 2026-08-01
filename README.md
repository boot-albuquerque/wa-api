# DisparaZap

DisparaZap é uma implementação da biblioteca [@tulir/whatsmeow](https://github.com/tulir/whatsmeow) como um serviço
RESTful com suporte a múltiplos dispositivos e sessões concorrentes.

Whatsmeow não usa Puppeteer no Chrome headless, nem emulador Android. Comunica-se diretamente com os servidores
WebSocket do WhatsApp, tornando-o significativamente mais rápido e muito menos exigente em memória e CPU do que
essas soluções. A desvantagem é que mudanças no protocolo do WhatsApp podem quebrar conexões, exigindo atualização
da biblioteca.

## :warning: Aviso

**Usar este software violando os Termos de Serviço do WhatsApp pode resultar no banimento do seu número.**
Tenha muito cuidado — não use para enviar SPAM ou algo similar. Use por sua conta e risco. Se você precisa
desenvolver algo para fins comerciais, entre em contato com um provedor global de soluções WhatsApp e
registre-se no serviço WhatsApp Business API.

## Endpoints disponíveis

* **Session:** Conecte, desconecte e faça logout do WhatsApp. Consulte status da conexão e QR codes para escaneamento.
* **Messages:** Envie mensagens de texto, imagem, áudio, documento, template, vídeo, sticker, localização, contato e enquetes.
* **Users:** Verifique se números de telefone possuem WhatsApp, obtenha informações de usuários e avatares, e recupere a lista completa de contatos.
* **Chat:** Configure presença (digitando/pausado, gravando mídia), marque mensagens como lidas, baixe imagens de mensagens, envie reações.
* **Groups:** Crie, exclua e liste grupos, obtenha informações, links de convite, configure participantes, altere fotos e nomes de grupos.
* **Webhooks:** Configure e obtenha webhooks que serão chamados sempre que eventos ou mensagens forem recebidos.
* **HMAC Configuration:** Configure chaves HMAC para segurança de webhooks e verificação de assinaturas.

### Assinatura HMAC para Webhooks

Quando HMAC está configurado, todos os webhooks incluem o header `x-hmac-signature` com assinatura SHA-256 HMAC.

#### Geração de Assinatura por Content-Type:

**`application/json`**
* Dados assinados: corpo bruto da requisição JSON
* Verificação: use exatamente o JSON recebido

**`application/x-www-form-urlencoded`**
* Dados assinados: string form-encoded (`key=value&key2=value2`)
* Verificação: reconstrua a string form a partir dos parâmetros recebidos

**`multipart/form-data`** (uploads de arquivo)
* Dados assinados: representação JSON dos campos do formulário (excluindo arquivos)
* Verificação: crie JSON a partir dos campos não-arquivo do formulário

*Sempre verifique assinaturas antes de processar webhooks*

## Pré-requisitos

**Obrigatório:**
* Go (Go Programming Language)

**Opcional:**
* Docker (para containerização)

## Atualizando dependências

Este projeto usa a biblioteca whatsmeow para comunicar-se com o WhatsApp. Para atualizar para a versão mais recente:

```bash
go get -u go.mau.fi/whatsmeow@latest
go mod tidy
```

## Build

O `main` do serviço fica em `cmd/core`, não na raiz do módulo — `go build .`
na raiz falha porque lá não existe pacote Go algum.

```bash
make build              # produz ./wa-api a partir de ./cmd/core
```

Sem o Makefile, o equivalente direto:

```bash
go build -o wa-api ./cmd/core
```

Para compilar tudo (inclusive os binários auxiliares `cmd/listroutes` e
`cmd/wss`) sem gerar artefato:

```bash
go build ./...
```

## Execução

Por padrão, inicia um serviço REST na porta 8080. Parâmetros disponíveis:

* `-admintoken`  : define o token de autenticação para endpoints admin. Se não especificado, será lido do .env
* `-address`  : define o endereço IP para bind do servidor (padrão 0.0.0.0)
* `-port`  : define a porta (padrão 8080)
* `-logtype` : formato dos logs, `console` (padrão) ou `json`
* `-color` : habilita saída colorida para logs em console
* `-osname` : nome do SO na conexão do WhatsApp
* `-skipmedia` : pula download de mídia das mensagens
* `-wadebug` : habilita debug do whatsmeow, níveis INFO ou DEBUG

* `-sslcertificate` : arquivo de certificado SSL
* `-sslprivatekey` : arquivo de chave privada SSL

Exemplo — logs coloridos:

```
./disparazapi -logtype=console -color=true
```

Logs em JSON:

```
./disparazapi -logtype json
```

Com timezone:

Configure `TZ=America/Sao_Paulo ./disparazapi ...` no shell ou no arquivo `.env` ou Docker Compose: `TZ=America/Sao_Paulo`.

## Configuração

DisparaZap usa um arquivo `.env` para configuração. Use o `.env.sample` como template:

```bash
cp .env.sample .env
```

### Variáveis de Ambiente

#### Configurações Obrigatórias
```
WA_API_ADMIN_TOKEN=seu_admin_token_aqui
```

#### Configurações de Segurança

```
WA_API_GLOBAL_ENCRYPTION_KEY=sua_chave_32_bytes_aqui
WA_API_GLOBAL_HMAC_KEY=sua_chave_hmac_global_aqui
```

#### Configurações Opcionais

```
TZ=America/Sao_Paulo
WEBHOOK_FORMAT=json
SESSION_DEVICE_NAME=DisparaZap
WA_API_PORT=8080
WA_API_GLOBAL_WEBHOOK=https://sua-url-global-webhook.url
WEBHOOK_RETRY_ENABLED=true
WEBHOOK_RETRY_COUNT=2
WEBHOOK_RETRY_DELAY_SECONDS=30
WEBHOOK_ERROR_QUEUE_NAME=disparazapi_dead_letter_webhooks
```

### Notas Importantes

#### Credenciais Auto-Geradas
Se as seguintes configurações não forem fornecidas, serão auto-geradas:
* `WA_API_ADMIN_TOKEN`: Token aleatório de 32 caracteres
* `WA_API_GLOBAL_ENCRYPTION_KEY`: Chave aleatória de 32 bytes para criptografia AES-256

**Importante**: Salve as credenciais auto-geradas no seu arquivo `.env` ou você perderá acesso aos dados criptografados e funções de admin ao reiniciar!

#### Segurança de Webhooks
* `WA_API_GLOBAL_HMAC_KEY`: Chave HMAC global para assinatura de webhooks (mínimo 32 caracteres)

**Breaking change:** o envio de webhooks agora verifica o certificado TLS do destino por padrão (antes a verificação era sempre desabilitada). Se o seu receptor de webhook usa certificado self-signed, defina `WA_API_WEBHOOK_TLS_SKIP_VERIFY=true` para restaurar o comportamento antigo — um aviso é registrado no log de boot quando habilitado. Padrão: `false`.

#### Configuração de Banco de Dados

**Para PostgreSQL:**
```
DB_USER=disparazapi
DB_PASSWORD=disparazapi
DB_NAME=disparazapi
DB_HOST=db  # Use 'db' com Docker Compose, ou 'localhost' para execução nativa
DB_PORT=5432
DB_SSLMODE=false
```

**Para SQLite (padrão):**
Nenhuma configuração de banco necessária — SQLite é usado por padrão se nenhuma configuração PostgreSQL for fornecida.

### Integração com RabbitMQ
DisparaZap suporta envio de eventos do WhatsApp para uma fila RabbitMQ para distribuição global de eventos. Quando habilitado, todos os eventos serão publicados na fila configurada independentemente das configurações de webhook individuais.

Configure estas variáveis de ambiente para habilitar:

```
RABBITMQ_URL=amqp://guest:guest@localhost:5672
RABBITMQ_QUEUE=whatsapp  # Opcional (padrão: whatsapp_events)
```

Quando habilitado:

* Todos os eventos do WhatsApp (mensagens, atualizações de presença, etc.) serão publicados na fila configurada independentemente das assinaturas de eventos para webhooks regulares
* Eventos incluirão userId e instanceName
* Funciona em conjunto com webhooks — eventos são enviados tanto para RabbitMQ quanto para webhooks configurados
* A integração é global e afeta todas as instâncias

### Segurança de Webhooks com HMAC

DisparaZap suporta assinaturas HMAC para verificação de webhooks:

* **HMAC por instância**: Configure chaves HMAC únicas para cada instância de usuário
* **HMAC global**: Defina uma chave HMAC global via `WA_API_GLOBAL_HMAC_KEY`
* **Header de assinatura**: Todos os webhooks assinados incluem o header `x-hmac-signature`
* **Segurança da chave**: Chaves HMAC nunca são expostas após a configuração

**Prioridade**: HMAC da instância > HMAC global > Sem assinatura

Configure chaves HMAC via Dashboard ou usando os endpoints da API `/session/hmac/config`.

#### Opções principais de configuração:

* WA_API_ADMIN_TOKEN: Obrigatório — Token de autenticação para endpoints admin
* TZ: Opcional — Timezone para operações do servidor (padrão: UTC)
* PostgreSQL: Opções específicas, apenas necessárias ao usar backend PostgreSQL
* RabbitMQ: Opcional, necessário apenas para publicar eventos no RabbitMQ

### Configuração Docker

Ao usar Docker Compose, `deploy/docker-compose.yml` carrega automaticamente variáveis do arquivo `.env` quando disponível. No entanto, `deploy/docker-compose-swarm.yaml` usa `docker stack deploy`, que não carrega automaticamente do `.env`. Variáveis no arquivo swarm só serão substituídas se exportadas no shell onde o comando deploy é executado. Para gerenciar segredos no Swarm, considere usar Docker secrets.

A configuração Docker irá:
1. Carregar variáveis do arquivo `.env` primeiro (se presente e suportado)
2. Usar valores padrão como fallback se variáveis não estiverem definidas
3. Sobrescrever com quaisquer variáveis definidas explicitamente na seção `environment` do compose

**Diferenças principais para deploy Docker:**
- Defina `DB_HOST=db` em vez de `localhost` para conectar ao container PostgreSQL
- A variável `WA_API_PORT` controla o mapeamento de porta externa no `deploy/docker-compose.yml`
- Em modo swarm, `WA_API_PORT` configura a porta do load balancer Traefik

**Nota:** O arquivo `.env` já está incluído no `.gitignore` para evitar commit de informações sensíveis.

## Uso

Para interagir com a API, inclua o header `Authorization` nas requisições HTTP, contendo o token de autenticação do usuário. Você pode ter múltiplos usuários (números diferentes de WhatsApp) no mesmo servidor.

* Referência Swagger em [/api](/api)
* Página web para conectar e escanear QR codes em [/login](/login)
* Dashboard completo para criar, gerenciar e testar instâncias em [/dashboard](dashboard)

## Ações de ADMIN

Você pode listar, adicionar e remover usuários usando os endpoints admin. Para isso, use o WA_API_ADMIN_TOKEN no header Authorization.

Use o endpoint `/admin/users` com o header Authorization contendo o token para:

- `GET /admin/users` - Listar todos os usuários
- `POST /admin/users` - Criar um novo usuário
- `DELETE /admin/users/{id}` - Remover um usuário

O corpo JSON para criar um novo usuário deve conter:

- `name` [string] : Nome do usuário
- `token` [string] : Token de segurança para autorizar/autenticar este usuário
- `webhook` [string] : URL para enviar eventos via POST (opcional)
- `events` [string] : Lista separada por vírgulas de eventos a receber (obrigatório) — eventos válidos: "Message", "ReadReceipt", "Presence", "HistorySync", "ChatPresence", "All"
- `expiration` [int] : Timestamp de expiração (opcional, não aplicado pelo sistema)

## Criação de Usuário com Proxy e S3

Você pode criar um usuário com configuração opcional de proxy e armazenamento S3. Todos os campos são opcionais e backward-compatible. Se não fornecidos, o usuário será criado com configurações padrão.

### Exemplo de Payload

```json
{
  "name": "test_user",
  "token": "user_token",
  "proxyConfig": {
    "enabled": true,
    "proxyURL": "socks5://user:pass@host:port"
  },
  "s3Config": {
    "enabled": true,
    "endpoint": "https://s3.amazonaws.com",
    "region": "us-east-1",
    "bucket": "my-bucket",
    "accessKey": "AKIAIOSFODNN7EXAMPLE",
    "secretKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
    "pathStyle": false,
    "publicURL": "https://cdn.yoursite.com",
    "mediaDelivery": "both",
    "retentionDays": 30
  }
}
```

- `proxyConfig` (object, opcional):
  - `enabled` (boolean): Habilita proxy para este usuário.
  - `proxyURL` (string): URL do proxy (ex.: `socks5://user:pass@host:port`).
- `s3Config` (object, opcional):
  - `enabled` (boolean): Habilita armazenamento S3 para este usuário.
  - `endpoint` (string): URL do endpoint S3.
  - `region` (string): Região S3.
  - `bucket` (string): Nome do bucket S3.
  - `accessKey` (string): Chave de acesso S3.
  - `secretKey` (string): Chave secreta S3.
  - `pathStyle` (boolean): Usar endereçamento path-style.
  - `publicURL` (string): URL pública para acesso aos arquivos.
  - `mediaDelivery` (string): Tipo de entrega de mídia (`base64`, `s3`, ou `both`).
  - `retentionDays` (integer): Dias para retenção dos arquivos.

Se você omitir `proxyConfig` ou `s3Config`, o usuário será criado sem integração de proxy ou S3, mantendo total backward compatibility.

## Referência da API

As chamadas da API devem ser feitas com content-type JSON, e parâmetros enviados no corpo da requisição, sempre passando o header Token para autenticação.

Consulte a [Referência da API](docs/api/README.md)

## Arquitetura

Este projeto segue os princípios de Clean Architecture com as seguintes camadas:

- **Domain**: Entidades e regras de negócio centrais
- **Application**: Casos de uso e ports (interfaces)
- **Infrastructure**: Adaptadores externos (banco de dados, WhatsApp, S3, RabbitMQ)
- **Interfaces**: HTTP handlers e rotas

## License

Copyright &copy; 2025 DisparaZap contributors

[MIT](https://choosealicense.com/licenses/mit/)

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Atribuição do Ícone

[Communication icons created by Vectors Market - Flaticon](https://www.flaticon.com/free-icons/communication)

## Legal

Este código não é de forma alguma afiliado, autorizado, mantido, patrocinado ou
endossado pelo WhatsApp ou qualquer de suas afiliadas ou subsidiárias. Este é um
software independente e não-oficial. Use por sua conta e risco.

## Aviso de Criptografia

Esta distribuição inclui software criptográfico. O país onde você reside atualmente
pode ter restrições quanto à importação, posse, uso e/ou reexportação de software
de criptografia. ANTES de usar qualquer software de criptografia, verifique as leis,
regulamentos e políticas do seu país sobre importação, posse, uso e reexportação de
software de criptografia, para verificar se isso é permitido. Veja
[http://www.wassenaar.org/](http://www.wassenaar.org/) para mais informações.

O U.S. Government Department of Commerce, Bureau of Industry and Security (BIS),
classificou este software como Export Commodity Control Number (ECCN) 5D002.C.1,
que inclui software de segurança da informação usando ou executando funções
criptográficas com algoritmos assimétricos. A forma e maneira desta distribuição
a torna elegível para exportação sob a exceção License Exception ENC Technology
Software Unrestricted (TSU) (veja o BIS Export Administration Regulations, Seção
740.13) tanto para código objeto quanto código fonte.
