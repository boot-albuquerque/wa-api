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

import { API, Tokens, listarSessoes, $ } from "./devui.js";
import { ENVIO, CHAT } from "./operacoes.js";

const POLL_MS = 3000;
let sessoes = [];

// ---- listagem ---------------------------------------------------------------

async function atualizar() {
  const r = await listarSessoes();
  if (!r.ok) {
    // 401 aqui é quase sempre token de admin em falta ou errado, e é a única
    // coisa que impede a página inteira de funcionar. Dizer isso é mais útil
    // do que uma grelha vazia sem explicação (HOUSEKEEP F94: um vazio mudo é
    // indistinguível de "não há nada" e de "não consigo ver").
    mostrarAviso(r.status === 401 || r.status === 0
      ? "Sem acesso à listagem. O token de admin é pedido ao criar uma sessão e fica guardado neste navegador."
      : `Não consegui listar as sessões (HTTP ${r.status}).`);
    return;
  }
  esconderAviso();
  sessoes = r.sessoes;
  desenhar();
}

function mostrarAviso(txt) { const a = $("aviso-admin"); a.textContent = txt; a.hidden = false; }
function esconderAviso() { $("aviso-admin").hidden = true; }

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
    if (!card) { card = criarCard(s); g.appendChild(card); }
    pintar(card, s);
  }
  for (const c of [...g.querySelectorAll(".card")]) {
    if (!vistos.has(c.dataset.id)) c.remove();
  }
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
    </div>`;
  for (const b of card.querySelectorAll("[data-a]")) {
    b.onclick = () => acao(cardSessao(card.dataset.id), b.dataset.a);
  }
  return card;
}

const cardSessao = (id) => sessoes.find((s) => s.id === id);
const cardDe = (id) => $("grid").querySelector(`.card[data-id="${CSS.escape(id)}"]`);

function pintar(card, s) {
  const cls = s.autenticado ? "on" : s.conectado ? "wait" : "off";
  const estado = s.autenticado ? "pareada e conectada"
               : s.conectado ? "conectada, não autenticada"
               : "desconectada";
  card.querySelector(".dot").className = "dot " + cls;
  card.querySelector(".nome").textContent = s.nome;
  card.querySelector(".tok").textContent = s.temToken ? "token local" : "sem token";
  card.querySelector(".estado").textContent = estado;
  card.querySelector(".jid").textContent = s.jid;
  card.querySelector(".sem-token").hidden = s.temToken;
  card.classList.toggle("ativa", s.autenticado);
  for (const b of card.querySelectorAll("[data-a]")) b.disabled = !s.temToken;
  if (s.autenticado) esconderQR(card);
}

// ---- QR ---------------------------------------------------------------------

const ttlTimers = new Map();

function mostrarQR(id, dataURI, expiraEm) {
  const card = cardDe(id); if (!card) return;
  const qr = card.querySelector(".qr");
  qr.hidden = false;
  qr.classList.remove("expirado");
  qr.querySelector(".over")?.remove();
  if (qr.firstElementChild?.tagName !== "IMG") qr.innerHTML = '<img alt="QR code de pareamento">';
  const img = qr.querySelector("img");
  if (img.src !== dataURI) img.src = dataURI;
  if (expiraEm) iniciarTTL(id, expiraEm);
}

function esconderQR(card) {
  card.querySelector(".qr").hidden = true;
  card.querySelector(".ttl").hidden = true;
}

// A barra usa o `expiresAt` que a PRÓPRIA API manda. Não há contagem fixa de
// 20s: o primeiro código vale 60s, e assumir 20 mostraria expirado o que ainda
// é válido.
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

function abrirWS(s) {
  fecharWS(s.id);
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/session/ws?token=${encodeURIComponent(s.token)}`);
  sockets.set(s.id, ws);
  ws.onmessage = (m) => {
    let d; try { d = JSON.parse(m.data); } catch { return; }
    const tipo = String(d.type || d.event || "").toLowerCase();
    // `qrCodeBase64` é OPCIONAL por contrato: se a codificação da imagem
    // falhar, o evento ainda chega com `code`. Melhor não desenhar nada do que
    // desenhar um <img> quebrado.
    if (tipo === "qr" && d.qrCodeBase64) mostrarQR(s.id, d.qrCodeBase64, d.expiresAt);
    if (tipo === "qrtimeout") marcarExpirado(s);
    if (tipo.includes("pairsuccess") || tipo === "connected" || tipo === "loggedout") atualizar();
  };
  ws.onclose = () => sockets.delete(s.id);
}

function fecharWS(id) {
  const ws = sockets.get(id);
  if (ws) { ws.onclose = null; ws.close(); sockets.delete(id); }
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
  if (!s || !s.temToken) return;
  const card = cardDe(s.id);

  if (qual === "mensagens") return abrirMenu(s, "Enviar mensagem", ENVIO,
    "Cada operação é uma rota de POST /chat/send/*. O telefone vai só com dígitos.");
  if (qual === "chat") return abrirMenu(s, "Operações de chat", CHAT,
    "Leitura e manipulação de conversas. Apagar mensagem é IRREVERSÍVEL do lado de quem recebeu.");

  if (qual === "conectar") {
    abrirWS(s);
    const qr = card.querySelector(".qr");
    qr.hidden = false; qr.classList.remove("expirado");
    qr.innerHTML = '<div class="msg">Pedindo QR à API…</div>';
    await API.sessao(s.token, "GET", "/session/connect");
    setTimeout(atualizar, 800);
    return;
  }

  if (qual === "desconectar") {
    // Derruba o TRANSPORTE e mantém o pareamento: reconecta depois sem QR
    // novo. É diferente de logout.
    await API.sessao(s.token, "GET", "/session/disconnect");
    fecharWS(s.id); esconderQR(card);
    setTimeout(atualizar, 800);
    return;
  }

  if (qual === "logout") {
    // DESVINCULA o aparelho: some de "Aparelhos conectados" no telemóvel e a
    // próxima conexão exige QR novo. Irreversível sem o telefone à mão.
    const ok = confirm(
      `Logout de "${s.nome}"\n\n` +
      `Isto DESVINCULA o aparelho: a sessão some de "Aparelhos conectados" no ` +
      `telemóvel e passa a ser preciso parear por QR outra vez.\n\n` +
      `Para apenas derrubar a conexão mantendo o pareamento, use Desconectar.`);
    if (!ok) return;
    await API.sessao(s.token, "POST", "/session/logout");
    fecharWS(s.id); esconderQR(card);
    setTimeout(atualizar, 800);
  }
}

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

  if (op.perigo && !confirm(`${op.rotulo}\n\nEsta operação é IRREVERSÍVEL. Confirmar?`)) return;

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
  $("nova-token").value = "";
  $("nova-admin").value = Tokens.admin();
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
  const adm = $("nova-admin").value.trim();

  if (!nome || !token || !adm) {
    return responder(resp, "err", "Preencha nome, token da sessão e token de admin.");
  }

  Tokens.guardarAdmin(adm);
  const r = await API.admin("POST", "/admin/users", { name: nome, token, events: "All" });
  if (!r.ok) {
    return responder(resp, "err", r.status === 401
      ? "HTTP 401 — o token de admin não foi aceite."
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

atualizar();
setInterval(atualizar, POLL_MS);
window.addEventListener("beforeunload", () => {
  for (const id of [...sockets.keys()]) fecharWS(id);
});
