// Package wanoise é apenas a âncora de documentação do diretório: todo o
// código vive nos subpacotes abaixo, um por responsabilidade. Nada importa
// este pacote — a raiz existir sem arquivos .go soltos é o ponto.
//
// A camada é o adapter que este projeto mantém sobre o cliente vendorizado em
// internal/wa-noise/, implementando as portas de pkg/application/contracts.
//
// Seam e apoio:
//
//	waclient/             a superfície de *whatsmeow.Client que os adapters
//	                      exercitam (waclient.Client), o getter por userID e a
//	                      ponte para o tipo concreto do SDK
//	waclient/waclienttest/ fakes exportados de waclient.Client e do ContactStore,
//	                      compartilhados pelos testes de todos os subpacotes
//	jid/                  parse de JID, conversão domain.JID <-> types.JID e o
//	                      JIDResolverAdapter
//	walog/                ponte waLog.Logger -> zerolog usada pelo SDK
//	applog/               ZerologAdapter, a porta appport.Logger
//	safego/               goroutine com recover para efeitos fire-and-forget
//	platform/             tradução do nome de plataforma para o enum do SDK
//
// Ciclo de vida e registro:
//
//	session/              guard, provider, a Session concreta, pareamento por
//	                      QR e tradução dos eventos de sessão
//	registry/             ClientManager: os registros por userID (cliente do
//	                      SDK, cliente HTTP de webhook, MyClient, opções de
//	                      enquete), o fan-out WebSocket e a contagem de sessões
//
// Adapters de capacidade, um subpacote por família de portas:
//
//	group/                GroupDirectory, GroupLifecycle, GroupSettings,
//	                      GroupRequests
//	user/                 ContactDirectory, BlocklistManager, PrivacyManager
//	chat/                 ChatMessenger e MessageComposer
//	presence/             PresenceController
//	misc/                 ChatOperations, ProfileAccessProvider,
//	                      NewsletterReader, AppStateSyncer
//	profile/              ProfileDataAccess sobre o Store do cliente
package wanoise
