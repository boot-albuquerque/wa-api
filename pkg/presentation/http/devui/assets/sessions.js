// Painel de sessões.
//
// MUDANÇAS DE 2026-08-20, e a razão de cada uma:
//
//   - A LISTAGEM VEM DA API. Antes, a lista era o que estivesse no
//     localStorage, e por isso existia um botão "adicionar existente": uma
//     sessão criada noutro sítio simplesmente não aparecia. Agora
//     `GET /admin/users` é a fonte, e o armazenamento local só acrescenta o
//     token — que a API redige e não há como recuperar depois.
//
//   - O TOKEN DE ADMIN saiu do cabeçalho. Estava lá como um campo de texto
//     permanente, o que fazia a página parecer um ecrã de login. Passa a ser
//     pedido só onde é preciso (criar sessão) e a ficar guardado.
//
//   - AS OPERAÇÕES DE ENVIO E CHAT vivem em modais no card, e não numa página
//     à parte: o que se quer testar é "esta sessão envia?", e ter de copiar um
//     token para outro sítio quebra essa pergunta ao meio.
//
//   - OS EVENTOS saíram para `eventos.html`. Eram um painel global colado à
//     lista, e ocupavam metade do ecrã mesmo quando não se estava a olhar
//     para eles.

import { API, Tokens, listarSessoes, carregarConfig, novoToken, $ } from "./devui.js";
import { ENVIO, CHAT } from "./operacoes.js";

const POLL_MS = 3000;

// Rotas de sessão usadas por este painel. Nomeadas em vez de literais soltos
// porque o mesmo caminho aparece em mais de um sítio e literal repetido é o
// mesmo bug à espera de divergir (ADR-0004).
const ROTA_CONNECT = "/session/connect";
const ROTA_DISCONNECT = "/session/disconnect";
const ROTA_LOGOUT = "/session/logout";
// F269 fez corte limpo de /session/qr para /session/pair/qr — o caminho
// antigo não responde mais (ver pkg/bootstrap/caminhos.tsv).
const ROTA_QR = "/session/pair/qr";

// comEngine anexa `?engine=` a uma rota do surface de pareamento
// (/session/connect, /session/qr). As duas exigem o parâmetro — sem ele,
// pairing.Registry.Resolve recusa com invalid_engine ANTES de tocar
// qualquer provider (pkg/pairing/registry.go). `s.engine` vem de
// GET /admin/users (devui.js:listarSessoes), sempre "noise" ou
// "headless" — os únicos valores domain.ParseEngine aceita.
function comEngine(rota, s) {
  return `${rota}?engine=${encodeURIComponent(s.engine)}`;
}

// Janela em que o painel espera o QR depois de pedir `connect`, e o intervalo
// entre sondagens de ROTA_QR dentro dela.
//
// 12s cobre com folga o pior caso medido em 2026-08-20 contra o servidor
// real PARA noise: entre o `connect` e o primeiro QR passaram-se 640ms a
// 1,4s em todas as corridas. headless não tem esse luxo — mede um boot de
// Chrome real, não um handshake de socket. Medido ao vivo em 2026-08-29
// (Claude in Chrome, connect→primeiro GET /session/pair/qr): 13039ms só
// nesse pedido, com o QR ainda vazio na resposta. 12s bastava para derrubar
// esse cartão em "falhou" ANTES do Chrome sequer montar a página — não é
// folga, é o timeout raso vencendo a corrida contra o boot. QR_ESPERA_MS
// passa a ser por engine; ver esperaQRMs.
const QR_ESPERA_MS_PADRAO = 12000;
const QR_ESPERA_MS_HEADLESS = 30000;
const QR_SONDA_MS = 1000;

// esperaQRMs devolve o teto de "nunca mostrou QR nenhum" para a sessão `s`.
// headless mede boot de Chrome real (variável, sujeito a carga da
// máquina); noise mede handshake de socket. Confundir os dois teto foi o
// defeito medido acima.
function esperaQRMs(s) {
  return s.engine === "headless" ? QR_ESPERA_MS_HEADLESS : QR_ESPERA_MS_PADRAO;
}

// Teto para o handshake do WebSocket. Não é o tempo até o QR: é só até o
// socket ficar OPEN, que na mesma medição levou 5ms a 13ms.
const WS_ABERTURA_MS = 5000;

// QR_PRESO_MS é o limiar de "código preso" DEPOIS de já ter mostrado um QR
// — diferente de QR_ESPERA_MS, que cobre "nunca mostrou nenhum". O backend
// (internal/wa-headless/capabilities/qr, HOUSEKEEP H145) já tenta se
// recuperar sozinho de um código vazio ou expirado — medido em ~1-4s no
// pior caso — então 15s de sondagem vazia SEGUIDA é folga generosa antes de
// admitir que a recuperação automática não está a resolver e mostrar o
// botão manual.
const QR_PRESO_MS = 15000;

let sessoes = [];
let ultimaAtualizacaoBoa = 0;

// ---- listagem ---------------------------------------------------------------

async function atualizar() {
  const r = await listarSessoes();
  if (!r.ok) {
    // 401 aqui é quase sempre token de admin em falta ou errado, e é a única
    // coisa que impede a página inteira de funcionar. Dizer isso é mais útil
    // do que uma grelha vazia sem explicação (HOUSEKEEP F94: um vazio mudo é
    // indistinguível de "não há nada" e de "não consigo ver").
    mostrarAviso(r.status === 401 || r.status === 0
      ? "Sem acesso à listagem. O token de admin vem do servidor em /devui/config — " +
        "se isto persistir, o painel foi servido por uma instância sem WA_API_DEV_UI."
      : `Não consegui listar as sessões (HTTP ${r.status}).`);
    // F208: when the poll fails, the cards stay frozen showing old state — and
    // that state contradicts the warning. Mark every card as stale so the
    // operator sees the data is old, not current.
    marcarCartoesSobreviventes();
    return;
  }
  esconderAviso();
  ultimaAtualizacaoBoa = Date.now();
  desmarcarCartoesSobreviventes();
  sessoes = r.sessoes;
  desenhar();
}

function mostrarAviso(txt) { const a = $("aviso-admin"); a.textContent = txt; a.hidden = false; }
function esconderAviso() { $("aviso-admin").hidden = true; }

// F208: mark/unmark cards as stale when the REST poll fails. Without this, the
// cards keep showing "pareada e conectada" right next to the warning that says
// the server is unreachable — a contradiction on the same screen.
function marcarCartoesSobreviventes() {
  const g = $("grid");
  if (!g) return;
  for (const card of g.querySelectorAll(".card")) {
    card.classList.add("stale");
    let badge = card.querySelector(".stale-badge");
    if (!badge) {
      badge = document.createElement("div");
      badge.className = "stale-badge";
      card.appendChild(badge);
    }
    atualizarIdadeBadge(badge);
  }
}

function desmarcarCartoesSobreviventes() {
  const g = $("grid");
  if (!g) return;
  for (const card of g.querySelectorAll(".card.stale")) {
    card.classList.remove("stale");
    card.querySelector(".stale-badge")?.remove();
  }
}

function atualizarIdadeBadge(badge) {
  if (!ultimaAtualizacaoBoa) {
    badge.textContent = "dados possivelmente desatualizados";
    return;
  }
  const segs = Math.round((Date.now() - ultimaAtualizacaoBoa) / 1000);
  badge.textContent = segs < 60
    ? `dados de há ${segs}s`
    : `dados de há ${Math.round(segs / 60)}min`;
}

function desenhar() {
  const g = $("grid");

  if (!sessoes.length) {
    g.innerHTML = `<div class="vazio"><b>Nenhuma sessão.</b><br>
      Use <b>+ Nova sessão</b> para criar um utilizador e parear um número.</div>`;
    return;
  }

  // Redesenha só o que mudou: recriar a grelha inteira a cada 3 segundos
  // apagaria o QR a meio do pareamento e faria o utilizador perder o código.
  const vistos = new Set();
  for (const s of sessoes) {
    vistos.add(s.id);
    let card = g.querySelector(`.card[data-id="${CSS.escape(s.id)}"]`);
    if (!card) {
      card = criarCard(s);
      g.appendChild(card);
      // F77: o socket abre aqui, na CRIAÇÃO do card, e não dentro de
      // `acao(s,"conectar")`. Antes, observar e conectar eram a MESMA ação:
      // depois da primeira queda do socket o painel ficava cego a eventos até
      // alguém clicar em Conectar — e clicar dispara `/session/connect`, com
      // efeito colateral de sessão.
      //
      // NA CRIAÇÃO, e não a cada repintura, porque `desenhar()` corre de 3 em
      // 3 segundos: reabrir a cada passagem seria RECONEXÃO AUTOMÁTICA, que é
      // deliberadamente proibida neste painel — "um socket que reabre sozinho
      // esconde o sintoma". Um socket que morre continua morto e visível.
      if (s.autenticado && s.temToken) abrirWS(s);
    }
    pintar(card, s);
  }
  for (const c of [...g.querySelectorAll(".card")]) {
    if (!vistos.has(c.dataset.id)) c.remove();
  }

  // O botão em lote só existe quando há o que limpar: um botão permanentemente
  // inerte ensina a ignorá-lo.
  const orfas = sessoes.filter((s) => !s.temToken).length;
  const btn = $("btn-limpar-orfas");
  btn.hidden = orfas === 0;
  btn.textContent = `Remover ${orfas} sem token`;
}

function criarCard(s) {
  const card = document.createElement("div");
  card.className = "card";
  card.dataset.id = s.id;
  card.innerHTML = `
    <div class="card-top">
      <span class="dot"></span>
      <span class="nome"></span>
      <span class="tok"></span>
      <span class="ws" title="estado do WebSocket de eventos"></span>
    </div>
    <div class="estado">—</div>
    <div class="jid"></div>
    <div class="sem-token aviso" hidden>Sem token neste navegador. A API não devolve tokens
      já criados, então esta sessão só pode ser observada — para operar, crie uma sessão nova.</div>
    <div class="qr" hidden><div class="msg">…</div></div>
    <div class="ttl" hidden><i></i></div>
    <div class="acoes">
      <button data-a="conectar" class="primary">Conectar</button>
      <button data-a="desconectar">Desconectar</button>
      <button data-a="mensagens">Mensagens</button>
      <button data-a="chat">Chat</button>
      <button data-a="logout" class="danger">Logout</button>
      <button data-a="remover" class="danger">Remover</button>
    </div>`;
  for (const b of card.querySelectorAll("[data-a]")) {
    b.onclick = () => acao(cardSessao(card.dataset.id), b.dataset.a);
  }
  return card;
}

const cardSessao = (id) => sessoes.find((s) => s.id === id);
const cardDe = (id) => $("grid").querySelector(`.card[data-id="${CSS.escape(id)}"]`);

// marcarWS escreve o estado do socket no cartão de `id`.
//
// Vive à parte de `pintar` porque tem de correr SEM ele. `atualizar()` desiste
// cedo quando a listagem falha (`sessions.js` acima, no ramo do aviso), e nesse
// caso `desenhar()` — logo `pintar()` — nunca corre. Medido em campo a
// 2026-08-21: com o servidor abaixo, os cartões congelavam a dizer "eventos
// ligados" e "pareada e conectada", sem aviso visível, enquanto nada estava
// ligado. A falha que mata os sockets é a MESMA que paralisa a repintura, por
// isso um indicador que só se atualiza ao repintar mente exatamente quando
// mais importa.
//
// Chamado também pelos eventos do próprio socket (`onopen`/`onclose`/
// `onerror`), que chegam independentemente do poll REST.
function marcarWS(id, vivo) {
  const el = $("grid")?.querySelector(`.card[data-id="${CSS.escape(id)}"] .ws`);
  if (!el) return;
  el.textContent = vivo ? "eventos ligados" : "sem eventos";
  el.className = "ws " + (vivo ? "on" : "off");
}

function pintar(card, s) {
  const cls = s.autenticado ? "on" : s.conectado ? "wait" : "off";
  const estado = s.autenticado ? "pareada e conectada"
               : s.conectado ? "conectada, não autenticada"
               : "desconectada";
  card.querySelector(".dot").className = "dot " + cls;
  card.querySelector(".nome").textContent = s.nome;
  card.querySelector(".tok").textContent = s.temToken ? "token local" : "sem token";

  // F77/F85: o estado do socket fica VISÍVEL. Abrir sozinho e não mostrar
  // seria pior que não abrir — o operador acharia que está a observar quando
  // a ligação já caiu, e é precisamente durante a rajada de HistorySync que
  // ela cai (F85).
  const ws = sockets.get(s.id);
  marcarWS(s.id, !!ws && ws.readyState === WebSocket.OPEN);
  card.querySelector(".estado").textContent = estado;
  card.querySelector(".jid").textContent = s.jid;
  card.querySelector(".sem-token").hidden = s.temToken;
  card.classList.toggle("ativa", s.autenticado);
  for (const b of card.querySelectorAll("[data-a]")) {
    // "Remover" continua ATIVO sem token local, e é o ponto: remover é
    // operação de ADMIN, e o admin vem do servidor. É assim que se apaga uma
    // sessão criada noutro navegador — o caso que antes não tinha saída
    // nenhuma a não ser ir à API à mão.
    b.disabled = !s.temToken && b.dataset.a !== "remover";
  }
  if (s.autenticado) esconderQR(card);
}

// ---- QR ---------------------------------------------------------------------

const ttlTimers = new Map();

// QR_DATA_URI_PREFIXO é o cabeçalho que `GET /session/pair/qr` promete no
// schema (`api/openapi/schemas/sessao.yaml:79` — "Imagem do QR code em data
// URI"), e o painel VERIFICA em vez de assumir. Ver mostrarQR.
const QR_DATA_URI_PREFIXO = "data:image/png;base64,";

// mostrarQR desenha a IMAGEM que GET /session/pair/qr devolve — o mesmo
// contrato para noise e headless desde a F373, agora que os dois
// passam pelo mesmo `pkg/qrimage`.
//
// Este painel já desenhou o valor client-side, tratando-o como a string crua
// de pareamento. Renderizava lindamente e o QR era INVÁLIDO: para noise o
// valor sempre foi o data URI, então o código desenhado codificava os 1858
// caracteres "data:image/png;base64,iVBOR…" — um QR perfeito com o conteúdo
// errado, que o telefone lê e o WhatsApp recusa. Nada no console acusava
// nada, porque 1858 cabe folgado no limite de 2953 bytes do nível L.
//
// Daí a GUARDA: se o valor não começar pelo prefixo documentado, ele não é
// uma imagem, e desenhá-lo num <img> falharia silenciosamente do mesmo jeito
// que desenhá-lo como payload falhou. Melhor dizer o que se recebeu.
function mostrarQR(id, dataURI) {
  const card = cardDe(id); if (!card) return;
  const qr = card.querySelector(".qr");
  if (!dataURI.startsWith(QR_DATA_URI_PREFIXO)) return contratoQRQuebrado(id, dataURI);
  qr.hidden = false;
  qr.classList.remove("expirado");
  qr.querySelector(".over")?.remove();
  if (qr.firstElementChild?.tagName !== "IMG") qr.innerHTML = '<img alt="QR code de pareamento">';
  const img = qr.querySelector("img");
  if (img.src === dataURI) return;
  img.src = dataURI;
  // Aproximado, não autoritativo: a API não devolve expiresAt nesta rota
  // (nunca devolveu — a barra só existia enquanto o QR chegava só por
  // WebSocket, cujo evento carrega expiresAt à parte). 20s é a cadência de
  // rotação MEDIDA (HOUSEKEEP H145, TestProbeQRRetryPattern, ~20s entre
  // rotações automáticas), reiniciada a cada imagem nova — é um indicador
  // visual de "está vivo", não uma promessa de prazo.
  iniciarTTL(id, new Date(Date.now() + 20000).toISOString());
}

// contratoQRQuebrado é o que o painel mostra quando a rota responde algo que
// não é a imagem documentada — um engine novo que devolva a string crua, por
// exemplo, que foi exactamente o estado de headless antes da F373.
// Aparecer aqui é melhor do que um <img> quebrado ou, pior, um QR bonito com
// o conteúdo errado.
function contratoQRQuebrado(id, valor) {
  const card = cardDe(id); if (!card) return;
  const qr = card.querySelector(".qr");
  qr.hidden = false;
  qr.innerHTML = '<div class="msg"></div>';
  qr.querySelector(".msg").textContent =
    "GET /session/pair/qr devolveu algo que não é a imagem documentada " +
    `(esperado "${QR_DATA_URI_PREFIXO}…", veio "${valor.slice(0, 24)}…"). ` +
    "O engine desta sessão não está a honrar o contrato da rota.";
  pararTTL(id);
}

function esconderQR(card) {
  card.querySelector(".qr").hidden = true;
  card.querySelector(".ttl").hidden = true;
}

// A barra é uma APROXIMAÇÃO da cadência medida (ver mostrarQR) — não um
// prazo que a API garanta.
function iniciarTTL(id, iso) {
  pararTTL(id);
  const fim = Date.parse(iso);
  const card = cardDe(id);
  if (!card || !Number.isFinite(fim)) return;
  const barra = card.querySelector(".ttl"), i = barra.querySelector("i");
  barra.hidden = false;
  const total = Math.max(1, fim - Date.now());
  const tick = () => {
    const resta = fim - Date.now();
    if (resta <= 0) { pararTTL(id); i.style.width = "0"; return; }
    i.style.width = Math.max(0, Math.min(100, (resta / total) * 100)) + "%";
    barra.classList.toggle("low", resta < 7000);
  };
  tick();
  ttlTimers.set(id, setInterval(tick, 250));
}
function pararTTL(id) {
  const t = ttlTimers.get(id);
  if (t) { clearInterval(t); ttlTimers.delete(id); }
}

// O QR chega por WebSocket, e por isso a página de sessões mantém um socket
// por sessão ENQUANTO ela estiver a parear — e só então. Um socket permanente
// aqui duplicaria o que `eventos.html` faz.
const sockets = new Map();

// abrirWS resolve SÓ quando o socket está OPEN (ou quando desistiu dele).
//
// Devolver antes disso foi o defeito de 2026-08-20. `acao(s,"conectar")`
// abria o socket e disparava `GET /session/connect` no MESMO tick; o registo
// do socket do lado do servidor (AddWSConn, em handler_session_ws.go) só
// acontece depois do upgrade concluir, e o QR despachado nessa janela não tem
// para onde ir — o fan-out entrega a zero conexões e o evento desaparece.
// Medido, com o socket a entrar 3s depois do `connect`:
//
//	0.433s HTTP connect(sem-ws) -> 200 {"status":"connecting"}
//	3.461s HTTP qr(depois-de-3s) -> len=1870   <- o QR EXISTE
//	3.481s WS(tardio) OPEN
//	21.366s WS(tardio) MSG type=QR             <- 17,9s de silêncio
//
// O QR emitido a ~1,4s nunca chegou ao socket. Esperar o OPEN fecha a
// corrida; a sondagem de ROTA_QR em aguardarQR cobre o que ela não fechar.
function abrirWS(s) {
  fecharWS(s.id);
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/session/ws?token=${encodeURIComponent(s.token)}`);
  sockets.set(s.id, ws);
  ws.onmessage = (m) => {
    let d; try { d = JSON.parse(m.data); } catch { return; }
    const tipo = String(d.type || d.event || "").toLowerCase();
    // O QR em si NÃO vem mais daqui — ver sondarQR. Os eventos `qr`/
    // `qrtimeout` só existem para noise (pkg/application/session/
    // orchestrator.go), e nunca para headless: a mesma sondagem de
    // GET /session/qr precisa cobrir os dois de qualquer forma, então usá-la
    // como ÚNICA fonte — em vez de WS para um engine e sondagem para o outro
    // — é a lógica igual que os dois merecem, e o painel para de depender de
    // um canal que o próprio código já documentava como "com perda".
    if (tipo.includes("pairsuccess") || tipo === "connected" || tipo === "loggedout") atualizar();
  };
  ws.onopen = () => marcarWS(s.id, true);
  // Não religa (ver o comentário em `desenhar`), mas DIZ que caiu — e diz sem
  // depender do poll, que pode estar morto pela mesma razão.
  ws.onclose = () => { sockets.delete(s.id); marcarWS(s.id, false); };
  ws.onerror = () => marcarWS(s.id, false);

  // Resolve em vez de rejeitar no erro e no tempo esgotado: o `connect` tem
  // de sair de qualquer maneira. Um socket que não abriu degrada para a
  // sondagem, não cancela o pareamento.
  return new Promise((resolve) => {
    if (ws.readyState === WebSocket.OPEN) return resolve(true);
    const pronto = (v) => resolve(v);
    ws.addEventListener("open", () => pronto(true), { once: true });
    ws.addEventListener("error", () => pronto(false), { once: true });
    setTimeout(() => pronto(false), WS_ABERTURA_MS);
  });
}

// sondadores: id -> objeto de controlo da sondagem em curso, para que uma
// segunda chamada a sondarQR (reconectar / "Gerar novo QR") CANCELE a
// anterior em vez de as duas correrem em paralelo escrevendo no mesmo
// cartão.
const sondadores = new Map();

function pararSondaQR(id) {
  const c = sondadores.get(id);
  if (c) { c.parado = true; sondadores.delete(id); }
}

// sondarQR sonda `GET /session/qr` CONTINUAMENTE — não só até o primeiro QR
// aparecer — e é a MESMA lógica para noise e headless: os dois
// respondem o mesmo contrato (imagem em data URI — `pkg/qrimage`, o único
// codificador dos dois desde a F373; a divergência que existia aqui é o que
// tornava o QR de noise inválido), e nenhum dos dois tem um canal de push que o outro não
// tenha (ver o comentário em abrirWS: `qr`/`qrtimeout` só existiam para
// noise, e o WebSocket é "canal com perda" mesmo nesse). Sondar sempre é
// o que os serve os dois igualmente bem.
//
// Dois estados de falha, distintos de propósito:
//
//   - NUNCA mostrou QR nenhum dentro de esperaQRMs(s) (12s noise, 30s
//     headless — boot de Chrome real, ver QR_ESPERA_MS_HEADLESS): o pedido
//     falhou de facto — sessão já pareada, engine mal configurado — e
//     falhouQR() diz isso com a mensagem certa.
//   - JÁ mostrou um QR, mas a sondagem volta vazia por QR_PRESO_MS (15s)
//     seguidos: o código travou apesar do backend tentar recuperar-se
//     sozinho (HOUSEKEEP H145) — marcarExpirado() mostra o botão manual,
//     que é o mesmo "Gerar novo QR" que existia só para o evento
//     `qrtimeout` de noise, agora acionado pela MESMA sondagem para
//     qualquer engine.
//
// É o que a Evolution API faz em connectToWhatsapp: depois de conectar, ela
// espera e LÊ o QR guardado (`await delay(2000); return instance.qrCode`) em
// vez de confiar só no evento.
function sondarQR(s) {
  pararSondaQR(s.id);
  const ctrl = { parado: false, teveQR: false, vazioDesde: Date.now(), inicio: Date.now() };
  sondadores.set(s.id, ctrl);

  const passo = async () => {
    if (ctrl.parado) return;
    const card = cardDe(s.id);
    if (!card) { pararSondaQR(s.id); return; }
    // A sessão pareou (por este ou por outro cartão/telefone) — nada mais a
    // sondar. `sessoes` é repintado por `atualizar()`, que o handler de
    // `connected`/`pairsuccess` do WS já dispara.
    const atual = cardSessao(s.id);
    if (atual?.autenticado) { pararSondaQR(s.id); esconderQR(card); return; }
    if (card.querySelector(".qr .over")) { pararSondaQR(s.id); return; } // já falhou/travou

    const r = await API.sessao(s.token, "GET", comEngine(ROTA_QR, s));
    if (ctrl.parado) return; // parado enquanto o pedido estava em voo
    // `qr_code`: a chave era `QRCode` — o nome do campo Go, PascalCase no fio
    // — até a migração para DTO (docs/HTTP-DTO-CONVENTIONS.md). Este painel é
    // o ÚNICO consumidor de /session/qr dentro do repositório, e o corte a
    // seco parte-o se ele não for corrigido junto.
    const imagem = r.body?.data?.qr_code || "";
    if (imagem) {
      ctrl.teveQR = true;
      ctrl.vazioDesde = 0;
      mostrarQR(s.id, imagem);
    } else {
      if (!ctrl.teveQR && Date.now() - ctrl.inicio >= esperaQRMs(s)) {
        pararSondaQR(s.id); falhouQR(s); return;
      }
      if (ctrl.teveQR) {
        if (!ctrl.vazioDesde) ctrl.vazioDesde = Date.now();
        if (Date.now() - ctrl.vazioDesde >= QR_PRESO_MS) {
          pararSondaQR(s.id); marcarExpirado(s); return;
        }
      }
    }
    setTimeout(passo, QR_SONDA_MS);
  };
  passo();
}

// falhouQR troca o "Pedindo QR à API…" perpétuo por um erro accionável.
//
// Um pedido que não produz QR nenhum dentro da janela falhou, e o painel tem
// de o dizer: um estado de espera sem fim é indistinguível de um painel
// partido, e foi assim que este defeito chegou até aqui.
function falhouQR(s) {
  const card = cardDe(s.id); if (!card) return;
  const qr = card.querySelector(".qr");
  if (qr.querySelector("img") || qr.querySelector(".over")) return;
  qr.hidden = false;
  qr.innerHTML = '<div class="msg">A API aceitou o pedido mas não emitiu QR nenhum. ' +
    "Uma sessão já pareada não gera QR — use Logout para desvincular antes de parear outra vez.</div>";
  const o = document.createElement("div");
  o.className = "over";
  o.innerHTML = '<button type="button">Tentar de novo</button>';
  o.querySelector("button").onclick = () => acao(s, "conectar");
  qr.appendChild(o);
}

function fecharWS(id) {
  const ws = sockets.get(id);
  if (ws) { ws.onclose = null; ws.close(); sockets.delete(id); }
  // Marca aqui porque o `onclose` foi anulado acima de propósito: um fecho
  // NOSSO não deve disparar a mesma cadeia de um fecho do outro lado. Sem esta
  // linha, `desconectar` e `logout` deixariam o cartão a dizer "eventos
  // ligados" até à próxima repintura — e, se a listagem também falhar, para
  // sempre.
  marcarWS(id, false);
}

function marcarExpirado(s) {
  const card = cardDe(s.id); if (!card) return;
  pararTTL(s.id);
  const qr = card.querySelector(".qr");
  qr.classList.add("expirado");
  if (!qr.querySelector(".over")) {
    const o = document.createElement("div");
    o.className = "over";
    o.innerHTML = '<button type="button">Gerar novo QR</button>';
    o.querySelector("button").onclick = () => acao(s, "conectar");
    qr.appendChild(o);
  }
}

// ---- ações ------------------------------------------------------------------

async function acao(s, qual) {
  if (!s) return;
  if (!s.temToken && qual !== "remover") return;
  const card = cardDe(s.id);

  if (qual === "remover") return abrirRemover(s);
  if (qual === "mensagens") return abrirMenu(s, "Enviar mensagem", ENVIO,
    "Cada operação é uma rota de POST /chat/send/*. O telefone vai só com dígitos.");
  if (qual === "chat") return abrirMenu(s, "Operações de chat", CHAT,
    "Leitura e manipulação de conversas. Apagar mensagem é IRREVERSÍVEL do lado de quem recebeu.");

  if (qual === "conectar") {
    const qr = card.querySelector(".qr");
    qr.hidden = false; qr.classList.remove("expirado");
    qr.innerHTML = '<div class="msg">Pedindo QR à API…</div>';
    // ORDEM: o socket ABERTO antes de o `connect` sair. Ver abrirWS para a
    // medição do que acontece quando esta ordem se inverte — inverter as duas
    // linhas continua a passar em qualquer teste que só olhe para o fim.
    await abrirWS(s);
    await API.sessao(s.token, "GET", comEngine(ROTA_CONNECT, s));
    setTimeout(atualizar, 800);
    // Sem await: a sondagem corre ao lado do painel, e prender `acao` aqui
    // deixaria o botão em espera durante toda a janela.
    sondarQR(s);
    return;
  }

  if (qual === "desconectar") {
    // Derruba o TRANSPORTE e mantém o pareamento: reconecta depois sem QR
    // novo. É diferente de logout.
    pararSondaQR(s.id);
    await API.sessao(s.token, "GET", ROTA_DISCONNECT);
    fecharWS(s.id); esconderQR(card);
    setTimeout(atualizar, 800);
    return;
  }

  if (qual === "logout") {
    // DESVINCULA o aparelho: some de "Aparelhos conectados" no telemóvel e a
    // próxima conexão exige QR novo. Irreversível sem o telefone à mão.
    const ok = await confirmar({
      titulo: `Logout de "${s.nome}"`,
      texto: "Isto DESVINCULA o aparelho: a sessão sai de \"Aparelhos conectados\" no " +
        "telemóvel e passa a ser preciso parear por QR outra vez, o que exige o telemóvel " +
        "à mão. Para apenas derrubar a conexão mantendo o pareamento, use Desconectar.",
      okTexto: "Desvincular",
    });
    if (!ok) return;
    pararSondaQR(s.id);
    await API.sessao(s.token, "POST", ROTA_LOGOUT);
    fecharWS(s.id); esconderQR(card);
    setTimeout(atualizar, 800);
  }
}

// confirmar substitui o confirm() nativo em TODO o painel.
//
// O nativo devolve `false` EM SILÊNCIO quando o navegador suprime diálogos — o
// Chrome oferece "impedir que esta página crie mais diálogos" depois de alguns
// seguidos, e a partir daí toda ação protegida por confirm() deixa de acontecer
// sem dizer porquê. Num painel de diagnóstico isso é pior que noutro sítio
// qualquer: o operador conclui que a API está partida.
//
// Foi o defeito reportado ("tentei remover todos e não deu certo") e existia em
// três sítios — o lote, o logout e o apagar mensagem. Corrigir só o reportado
// deixaria os outros dois à espera.
function confirmar({ titulo, texto, okTexto = "Confirmar" }) {
  return new Promise((resolve) => {
    $("conf-titulo").textContent = titulo;
    $("conf-texto").textContent = texto;
    $("conf-ok").textContent = okTexto;
    let decidido = false;
    $("conf-ok").onclick = () => { decidido = true; $("dlg-confirma").close(); };
    $("dlg-confirma").onclose = () => resolve(decidido);
    $("dlg-confirma").showModal();
  });
}

// ---- remover ----------------------------------------------------------------

// São DUAS rotas, e significam coisas diferentes:
//
//   DELETE /admin/users/{id}       apaga o utilizador da API. O TELEMÓVEL FICA
//                                  PAREADO a uma sessão que já não existe — o
//                                  aparelho continua em "Aparelhos conectados"
//                                  e ninguém o tira de lá a não ser à mão.
//   DELETE /admin/users/{id}/full  faz logout e desconecta ANTES de apagar, e
//                                  por isso o telemóvel liberta o aparelho.
//
// O painel oferece as duas em vez de escolher por quem usa. Esconder a
// diferença seria deixar dispositivos-fantasma no telemóvel de alguém sem que
// essa pessoa soubesse porquê — e "remover" é a palavra que faz esperar o
// contrário.
//
// Nenhuma delas precisa do token da sessão: são operações de ADMIN. É assim
// que se apaga uma sessão criada noutro navegador.
function abrirRemover(s) {
  $("rem-titulo").textContent = `Remover "${s.nome}"`;
  $("rem-ajuda").textContent = s.autenticado
    ? "Esta sessão está pareada a um telemóvel."
    : "Esta sessão não está pareada.";

  const aviso = $("rem-aviso");
  aviso.hidden = !s.autenticado;
  aviso.textContent = "Remover SÓ DA API deixa o aparelho em \"Aparelhos conectados\" no " +
    "telemóvel, ligado a uma sessão que deixou de existir. Desvincular e remover faz o " +
    "logout primeiro.";

  const resp = $("rem-resposta");
  resp.hidden = true; resp.className = "resposta";

  $("rem-simples").onclick = () => remover(s, false);
  $("rem-completo").onclick = () => remover(s, true);
  $("dlg-remover").showModal();
}

async function remover(s, completo) {
  const resp = $("rem-resposta");
  const caminho = `/admin/users/${encodeURIComponent(s.id)}${completo ? "/full" : ""}`;

  const r = await API.admin("DELETE", caminho);
  if (!r.ok) {
    return responder(resp, "err", `HTTP ${r.status}\n${JSON.stringify(r.body, null, 2)}`);
  }

  // O token local só é esquecido DEPOIS de a API confirmar. Esquecer antes
  // deixaria a sessão na lista sem forma de a operar se a remoção falhasse.
  fecharWS(s.id);
  pararTTL(s.id);
  Tokens.esquecer(s.id);

  $("dlg-remover").close();
  await atualizar();
}

// ---- remover as órfãs -------------------------------------------------------

// "Sem token" quer dizer: existe na API, e este navegador não tem a credencial
// dela. Na prática são as sessões criadas noutro navegador, ou as que ficaram
// para trás quando alguém limpou o armazenamento.
//
// É um botão em lote porque limpá-las uma a uma é o tipo de tarefa que ninguém
// faz — e sessões que não se conseguem operar acumulam-se até a lista deixar de
// ser útil.
$("btn-limpar-orfas").onclick = () => {
  const orfas = sessoes.filter((s) => !s.temToken);
  if (!orfas.length) return;

  const pareadas = orfas.filter((s) => s.autenticado);
  const aviso = $("lote-aviso");
  aviso.hidden = pareadas.length === 0;
  aviso.textContent =
    `${pareadas.length} destas está(ão) PAREADA(S) a um telemóvel. Removê-las desvincula o ` +
    `aparelho, e voltar a usá-las exige ler um QR novo — o que precisa do telemóvel à mão. ` +
    `Vêm DESMARCADAS por isso; marque só se for mesmo o que quer.`;

  const lista = $("lote-lista");
  lista.innerHTML = "";
  for (const s of orfas) {
    const l = document.createElement("label");
    l.className = "campo";
    l.style.flexDirection = "row";
    l.style.alignItems = "center";
    l.style.gap = "8px";
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.style.width = "auto";
    cb.dataset.id = s.id;
    // PAREADA vem desmarcada. Foi assim que eu própria apaguei duas sessões
    // vivas ao diagnosticar isto: a caixa vinha marcada por omissão e "remover
    // as sem token" não avisava que "sem token" inclui sessões a funcionar.
    cb.checked = !s.autenticado;
    const span = document.createElement("span");
    span.textContent = s.nome + (s.autenticado ? "  — PAREADA" : "");
    span.style.color = s.autenticado ? "var(--warn)" : "";
    l.append(cb, span);
    lista.appendChild(l);
  }

  const resp = $("lote-resposta");
  resp.hidden = true; resp.className = "resposta";
  $("dlg-lote").showModal();
};

// A execução vive no BOTÃO, e não atrás de um confirm().
//
// O confirm() nativo devolve `false` EM SILÊNCIO quando o navegador suprime
// diálogos — o Chrome oferece "impedir que esta página crie mais diálogos"
// depois de alguns seguidos, e a partir daí o botão parece não fazer nada.
// Foi exatamente o sintoma reportado: "tentei remover todos e não deu certo".
//
// Um modal próprio não pode ser suprimido, mostra O QUE vai ser removido, e
// deixa a resposta à vista quando algo falha.
$("lote-executar").onclick = async () => {
  const resp = $("lote-resposta");
  const marcados = [...$("lote-lista").querySelectorAll("input:checked")].map((c) => c.dataset.id);

  if (!marcados.length) {
    return responder(resp, "err", "Nenhuma sessão marcada.");
  }

  const falhas = [];
  for (const id of marcados) {
    const s = sessoes.find((x) => x.id === id);
    if (!s) continue;
    // Cada uma pela rota CERTA: as pareadas por /full, para não deixarem
    // aparelho-fantasma; as outras pela simples, que não gasta ida ao
    // protocolo.
    const r = await API.admin("DELETE", `/admin/users/${encodeURIComponent(id)}${s.autenticado ? "/full" : ""}`);
    if (r.ok) {
      fecharWS(id); pararTTL(id); Tokens.esquecer(id);
    } else {
      falhas.push(`${s.nome}: HTTP ${r.status}`);
    }
  }

  await atualizar();

  // Uma falha no meio não pode passar despercebida só porque as outras
  // correram bem: o diálogo fica aberto a dizer quais falharam.
  if (falhas.length) {
    return responder(resp, "err", `Removidas ${marcados.length - falhas.length} de ${marcados.length}.\n` + falhas.join("\n"));
  }
  $("dlg-lote").close();
};

// ---- modais de operação -----------------------------------------------------

function abrirMenu(s, titulo, catalogo, ajuda) {
  $("menu-titulo").textContent = `${titulo} — ${s.nome}`;
  $("menu-ajuda").textContent = ajuda;
  const ops = $("menu-ops");
  ops.innerHTML = "";
  for (const op of catalogo) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = op.rotulo;
    if (op.perigo) b.className = "danger";
    b.onclick = () => { $("dlg-menu").close(); abrirOperacao(s, op); };
    ops.appendChild(b);
  }
  $("dlg-menu").showModal();
}

function abrirOperacao(s, op) {
  const metodo = op.metodo || "POST";
  $("op-titulo").textContent = `${op.rotulo} — ${s.nome}`;
  $("op-ajuda").textContent = `${metodo} ${op.rota}`;
  const campos = op.campos || op.query || [];
  const alvo = $("op-campos");
  alvo.innerHTML = "";

  for (const c of campos) {
    const l = document.createElement("label");
    l.className = "campo";
    const span = document.createElement("span");
    span.textContent = c.rotulo + (c.req ? " *" : "");
    l.appendChild(span);

    let ctrl;
    if (c.tipo === "textarea") { ctrl = document.createElement("textarea"); }
    else if (c.tipo === "select") {
      ctrl = document.createElement("select");
      for (const o of c.opcoes) {
        const opt = document.createElement("option");
        opt.value = o; opt.textContent = o;
        ctrl.appendChild(opt);
      }
    } else {
      ctrl = document.createElement("input");
      ctrl.type = c.tipo === "file" ? "file" : c.tipo === "number" ? "number" : "text";
      if (c.tipo === "number") ctrl.step = "any";
    }
    ctrl.dataset.campo = c.nome;
    if (c.valor !== undefined && c.tipo !== "file") ctrl.value = c.valor;
    l.appendChild(ctrl);
    alvo.appendChild(l);
  }

  const resp = $("op-resposta");
  resp.hidden = true; resp.className = "resposta";
  $("op-enviar").onclick = () => executar(s, op, campos);
  $("dlg-op").showModal();
}

async function executar(s, op, campos) {
  const metodo = op.metodo || "POST";
  const resp = $("op-resposta");

  if (op.perigo) {
    const ok = await confirmar({
      titulo: op.rotulo,
      texto: "Esta operação é IRREVERSÍVEL do lado de quem recebeu a mensagem.",
      okTexto: op.rotulo,
    });
    if (!ok) return;
  }

  let dados;
  try { dados = await recolher(campos); }
  catch (e) { return responder(resp, "err", String(e.message || e)); }

  const em = campos.some((c) => c.req && (dados[c.nome] === "" || dados[c.nome] === undefined));
  if (em) return responder(resp, "err", "Preencha os campos obrigatórios (marcados com *).");

  let r;
  if (metodo === "GET") {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(dados)) if (v !== "" && v !== undefined) qs.set(k, v);
    r = await API.sessao(s.token, "GET", `${op.rota}?${qs}`);
  } else {
    r = await API.sessao(s.token, "POST", op.rota, dados);
  }
  responder(resp, r.ok ? "ok" : "err", `HTTP ${r.status}\n${JSON.stringify(r.body, null, 2)}`);
}

// recolher lê os controlos e aplica `pre` de cada campo.
//
// O `pre` corre DENTRO do try do chamador porque JSON.parse é um `pre` — um
// JSON malformado tem de virar mensagem no painel, e não exceção silenciosa
// que deixa o botão sem reação.
async function recolher(campos) {
  const dados = {};
  for (const c of campos) {
    const ctrl = $("op-campos").querySelector(`[data-campo="${CSS.escape(c.nome)}"]`);
    if (!ctrl) continue;
    let v;
    if (c.tipo === "file") {
      const f = ctrl.files?.[0];
      if (!f) { dados[c.nome] = ""; continue; }
      v = await lerComoDataURI(f);
    } else {
      v = ctrl.value.trim();
    }
    if (v === "" && !c.req) continue;
    dados[c.nome] = c.pre ? c.pre(v) : v;
  }
  return dados;
}

const lerComoDataURI = (f) => new Promise((res, rej) => {
  const r = new FileReader();
  r.onload = () => res(String(r.result));
  r.onerror = () => rej(new Error(`não consegui ler ${f.name}`));
  r.readAsDataURL(f);
});

function responder(el, classe, txt) {
  el.hidden = false;
  el.className = "resposta " + classe;
  el.textContent = txt;
}

// ---- criar sessão -----------------------------------------------------------

$("btn-nova").onclick = () => {
  $("nova-nome").value = "";
  // noise por padrão a cada abertura: é o engine canónico do projeto, e
  // reabrir o diálogo não deve carregar a última escolha de uma tentativa
  // anterior.
  $("nova-engine").value = "noise";
  // Gerado a cada abertura, e não reaproveitado: se alguém abrir o diálogo,
  // desistir e voltar, o token de arranque tem de ser outro. Reusar faria dois
  // "cancelar" seguidos proporem a mesma credencial.
  $("nova-token").value = novoToken();
  const resp = $("nova-resposta");
  resp.hidden = true; resp.className = "resposta";
  $("dlg-nova").showModal();
  $("nova-nome").focus();
};

// O botão CRIA, e não fecha-e-depois-cria.
//
// A primeira versão pendurava o trabalho em `dialog.onclose` e lia os campos
// depois de o diálogo fechar. Parecia elegante e era frágil: o evento de fecho
// não é um sítio fiável para executar uma ação, e testar isso exige simular o
// fecho em vez de carregar no botão. Descoberto ao exercitar a página no
// navegador — o diálogo fechava com `returnValue = "ok"` e nada acontecia.
//
// Aqui o botão faz o trabalho, mostra a resposta DENTRO do diálogo, e só o
// fecha quando correu bem. Um erro fica à vista em vez de desaparecer com o
// modal.
$("nova-criar").onclick = async () => {
  const resp = $("nova-resposta");
  const nome = $("nova-nome").value.trim();
  const token = $("nova-token").value.trim();
  const engine = $("nova-engine").value;

  if (!nome) return responder(resp, "err", "Dê um nome à sessão.");
  if (!token) return responder(resp, "err", "O token não foi gerado. Feche e reabra o diálogo.");

  const r = await API.admin("POST", "/admin/users", { name: nome, token, events: "All", engine });
  if (!r.ok) {
    return responder(resp, "err", r.status === 401
      ? "HTTP 401 — o servidor recusou o token de admin que ele próprio entregou. " +
        "Recarregue a página: o servidor pode ter reiniciado com outro."
      : `HTTP ${r.status}\n${JSON.stringify(r.body, null, 2)}`);
  }

  // O token só vem AQUI, na resposta da criação — `GET /admin/users` redige-o.
  // Guardá-lo agora é a única oportunidade que existe.
  const id = r.body?.data?.id;
  if (id) Tokens.guardar(id, r.body?.data?.token || token);

  $("dlg-nova").close();
  esconderAviso();
  await atualizar();
};

// ---- ciclo ------------------------------------------------------------------

// A configuração vem PRIMEIRO: sem o token de admin, a listagem devolve 401 e
// o painel mostraria "sem acesso" no arranque antes de ter tentado obtê-lo.
await carregarConfig();
atualizar();
setInterval(atualizar, POLL_MS);
window.addEventListener("beforeunload", () => {
  for (const id of [...sockets.keys()]) fecharWS(id);
});
