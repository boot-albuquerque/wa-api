// Fluxo de eventos das sessões, na SUA PRÓPRIA rota.
//
// Antes era um painel colado à lista de sessões, ocupando metade do ecrã
// mesmo quando ninguém olhava para ele — e, pior, o WebSocket ficava aberto
// enquanto a página de sessões estivesse aberta, misturando "quero ver o QR"
// com "quero observar o tráfego".
//
// Aqui a ligação existe porque a página existe: fechar o separador fecha os
// sockets, que é o que se espera de um observador.

import { Tokens, listarSessoes, $ } from "./devui.js";

const MAX_LINHAS = 2000; // acima disto o navegador engasga e o painel deixa de servir
const sockets = new Map(); // id -> WebSocket
let filtro = "";

// ---- registo ----------------------------------------------------------------

function registar(quem, tipo, carga, classe) {
  $("vazio")?.remove();
  const fluxo = $("fluxo");

  const row = document.createElement("div");
  row.className = "ev" + (classe ? " " + classe : "");
  row.innerHTML = `<time></time><span class="quem"></span><span class="tipo"></span><span class="carga"></span>`;
  row.querySelector("time").textContent = new Date().toLocaleTimeString("pt-BR", { hour12: false });
  // textContent e não innerHTML: a carga vem do servidor e pode conter
  // qualquer coisa. Interpolá-la como HTML seria injeção num painel que
  // mostra exatamente aquilo que não se controla.
  row.querySelector(".quem").textContent = quem;
  row.querySelector(".tipo").textContent = tipo;
  row.querySelector(".carga").textContent = carga ?? "";
  row.dataset.busca = `${quem} ${tipo} ${carga ?? ""}`.toLowerCase();
  aplicarFiltro(row);

  const noFim = fluxo.scrollHeight - fluxo.scrollTop - fluxo.clientHeight < 40;
  fluxo.appendChild(row);
  while (fluxo.childElementCount > MAX_LINHAS) fluxo.firstElementChild.remove();
  if ($("seguir").checked && noFim) fluxo.scrollTop = fluxo.scrollHeight;
}

const aplicarFiltro = (row) => { row.hidden = !!filtro && !row.dataset.busca.includes(filtro); };

$("filtro").oninput = (e) => {
  filtro = e.target.value.trim().toLowerCase();
  for (const r of $("fluxo").querySelectorAll(".ev")) aplicarFiltro(r);
};
$("btn-limpar").onclick = () => { $("fluxo").innerHTML = ""; };

// ---- sockets ----------------------------------------------------------------

function abrir(s) {
  if (sockets.has(s.id)) return;
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  // O token vai na query porque a API WebSocket do navegador não permite
  // header no handshake. É a única exceção à regra de header — HOUSEKEEP F75.
  const ws = new WebSocket(`${proto}//${location.host}/session/ws?token=${encodeURIComponent(s.token)}`);
  sockets.set(s.id, ws);

  ws.onopen = () => { registar(s.nome, "ws", "ligação aberta", "sys"); contar(); };
  ws.onerror = () => registar(s.nome, "ws", "erro de socket", "err");
  ws.onclose = (e) => {
    // Sem religação automática: num painel de diagnóstico, um socket que
    // reabre sozinho esconde o sintoma que se quer observar.
    registar(s.nome, "ws", `fechado (código ${e.code})`, "sys");
    sockets.delete(s.id); contar();
  };
  ws.onmessage = (m) => {
    let d;
    try { d = JSON.parse(m.data); } catch { registar(s.nome, "raw", m.data); return; }
    const tipo = d.type || d.event || "evento";
    const resto = { ...d };
    delete resto.type;
    // O QR chega como PNG em base64 e enche o painel com dezenas de milhares
    // de caracteres, empurrando para fora tudo o resto. O tamanho é a
    // informação útil; os bytes não são.
    if (resto.qrCodeBase64) resto.qrCodeBase64 = `<png ${resto.qrCodeBase64.length} bytes>`;
    registar(s.nome, String(tipo), JSON.stringify(resto));
  };
}

const contar = () => { $("resumo").textContent = `${sockets.size} ligado${sockets.size === 1 ? "" : "s"}`; };

async function sincronizar() {
  const r = await listarSessoes();
  if (!r.ok) return;
  const comToken = r.sessoes.filter((s) => s.temToken);

  if (!comToken.length && !sockets.size) {
    const v = $("vazio");
    if (v) {
      v.textContent = Object.keys(Tokens.todos()).length
        ? "As sessões com token neste navegador já não existem na API."
        : "Nenhuma sessão com token neste navegador. Crie uma em Sessões para ver os eventos dela.";
    }
    return;
  }

  for (const s of comToken) abrir(s);
  // Fecha o que já não está na API: uma sessão apagada deixa um socket a
  // apontar para nada, e o `fechado (código …)` que ele produz parece defeito.
  const vivos = new Set(comToken.map((s) => s.id));
  for (const id of [...sockets.keys()]) {
    if (!vivos.has(id)) { sockets.get(id)?.close(); sockets.delete(id); }
  }
}

sincronizar();
setInterval(sincronizar, 5000);
window.addEventListener("beforeunload", () => {
  for (const ws of sockets.values()) { ws.onclose = null; ws.close(); }
});
