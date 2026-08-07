// Package wanoise é apenas a âncora de documentação do diretório: todo o
// código vive nos subpacotes abaixo. Nada importa este pacote — a raiz
// existir sem arquivos .go soltos é o ponto.
//
// Esta é a camada de INTEGRAÇÃO entre a aplicação e o módulo de protocolo em
// internal/wa-noise/. A distinção com aquela árvore é deliberada e a
// repetição de nomes entre as duas é informativa, não redundante:
//
//	internal/wa-noise/capabilities/group   implementa a capability Group do protocolo
//	pkg/infra/wa-noise/adapters/group      faz a aplicação usar Groups via WhatsApp
//
// # Direção das dependências
//
// A regra é que adapters ficam no topo e nada abaixo deles os importa de
// volta:
//
//	adapters/  ──► mapping/, client/, registry/
//	runtime/   ──► client/, registry/, observability/
//	registry/  ──► client/
//	client/    ──► internal/wa-noise
//
// Proibido, e a razão de a árvore ter esta forma: client, registry, mapping
// ou observability importarem adapters.
//
// # adapters/ — traduzem porta da aplicação em operação WhatsApp
//
//	chat/          ChatMessenger e MessageComposer
//	group/         GroupDirectory, GroupLifecycle, GroupSettings, GroupRequests
//	presence/      PresenceController
//	profile/       ProfileDataAccess sobre o Store do cliente
//	user/          ContactDirectory, BlocklistManager, PrivacyManager
//	misc/          ChatOperations, ProfileAccessProvider, NewsletterReader,
//	               AppStateSyncer — dívida explícita, ver abaixo
//	sessioncount/  SessionCounterAdapter (appport.SessionCounter)
//
// misc/ é um pacote a ser eliminado, não um destino. "misc" significa que o
// boundary ainda não foi encontrado; cada método dele pertence, no fim, a um
// dos outros adapters ou a uma capability de nome concreto.
//
// # client/ — a fronteira anticorrupção
//
// O pacote se chama waclient, embora o diretório se chame client. Não é
// descuido: "waclient" no CAMINHO é redundante (já estamos sob wa-noise/),
// mas o identificador aparece em ~38 arquivos onde esse caminho não está à
// vista, e lá `waclient.Client` carrega o que `client.Client` — um stutter —
// não carrega. O nome `client` como pacote também colidiria com as muitas
// variáveis locais chamadas `client` que já existem nestes adapters. Os
// imports trazem o alias explícito para que a diferença nunca surpreenda.
//
//	client/          waclient.Client (a superfície de *wanoise.Client que os
//	                 adapters exercitam), o getter por userID, a ponte para o
//	                 tipo concreto do SDK e os timeouts de fronteira
//	client/testkit/  fakes exportados de waclient.Client e do ContactStore,
//	                 compartilhados pelos testes de todos os subpacotes
//
// # runtime/ — ciclo de vida, não operação
//
//	session/  guard, provider, a Session concreta, pareamento por QR e a
//	          tradução dos eventos de sessão
//	safego/   goroutine com recover para efeitos fire-and-forget
//
// A distinção com adapters/: adapters/user EXECUTA operações de usuário;
// runtime/session MANTÉM a integração de pé.
//
// # registry/ — instâncias e estado compartilhado por userID
//
// Só pertence a registry/ o que registra, localiza, armazena ou indexa
// instâncias. Ver o doc do próprio pacote para a divisão interna em
// sub-registries e o invariante de lock que a sustenta.
//
// # mapping/ — tradução entre representações
//
//	jid/       parse de JID, conversão domain.JID <-> types.JID, JIDResolverAdapter
//	platform/  nome de plataforma do app -> enum DeviceProps_PlatformType do SDK
//
// # observability/
//
//	applog/  ZerologAdapter, a porta appport.Logger (logging DO APP)
//	walog/   ponte waLog.Logger -> zerolog usada pelo SDK (logging DO PROTOCOLO)
//
// Os nomes de pacote continuam applog/walog em vez de app/whatsapp: sob os
// diretórios `observability/app` e `observability/whatsapp`, os call sites
// virariam `app.NewZerologAdapter()`, que perde a informação de que aquilo é
// logging.
package wanoise
