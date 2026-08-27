// Estado e chamadas partilhados pelas páginas do devui.
//
// POR QUE UM MÓDULO E NÃO UMA CÓPIA EM CADA PÁGINA: `sessions.html` e
// `eventos.html` precisam da MESMA regra de tokens e do MESMO cliente HTTP.
// Duas cópias divergem — é a HOUSEKEEP F187 outra vez, e ali custou um dia.
export const API = {
  // O token das chamadas REST vai no HEADER, nunca na query string: a query
  // ainda é aceite nesta release mas emite WARN a cada request e sai na
  // próxima (middleware/auth.go). Uma página de teste que exercite o caminho
  // deprecado ensina o caminho errado a quem a ler.
  //
  // O WebSocket é a EXCEÇÃO, e não por descuido: a API WebSocket do navegador
  // não permite header no handshake. Não há alternativa do lado do cliente —
  // HOUSEKEEP F75.
  async sessao(token, metodo, caminho, corpo) {
    const opt = { method: metodo, cache: "no-store", headers: { token } };
    if (corpo !== undefined) {
      opt.headers["Content-Type"] = "application/json";
      opt.body = JSON.stringify(corpo);
    }
    const r = await fetch(caminho, opt);
    let b = null;
    try { b = await r.json(); } catch { /* resposta sem corpo JSON */ }
    return { ok: r.ok, status: r.status, body: b };
  },

  async admin(metodo, caminho, corpo) {
    const opt = { method: metodo, cache: "no-store", headers: { Authorization: Tokens.admin() } };
    if (corpo !== undefined) {
      opt.headers["Content-Type"] = "application/json";
      opt.body = JSON.stringify(corpo);
    }
    const r = await fetch(caminho, opt);
    let b = null;
    try { b = await r.json(); } catch { /* idem */ }
    return { ok: r.ok, status: r.status, body: b };
  },
};

// Tokens guarda o que a API NÃO devolve.
//
// `GET /admin/users` redige o token (`"token":""`) e faz bem: um listador que
// devolvesse credenciais transformaria o token de admin numa chave mestra.
// Mas `/session/*` autentica POR token, então a página precisa dele.
//
// A saída é: `POST /admin/users` DEVOLVE o token na criação — medido, não
// suposto — e é aí, e só aí, que ele entra aqui.
//
// A CHAVE É O `id` DO UTILIZADOR, e não o token. Assim a listagem vem sempre
// da API e o armazenamento local só acrescenta a credencial. Chavear pelo
// token faria a lista ser o que está no navegador, que foi o desenho anterior
// e a razão de existir um botão "adicionar existente".
export const Tokens = {
  CHAVE: "wa-devui-tokens",

  todos() {
    try { return JSON.parse(localStorage.getItem(this.CHAVE) || "{}"); }
    catch { return {}; }
  },
  de(id) { return this.todos()[id] || ""; },
  guardar(id, token) {
    const m = this.todos();
    m[id] = token;
    localStorage.setItem(this.CHAVE, JSON.stringify(m));
  },
  esquecer(id) {
    const m = this.todos();
    delete m[id];
    localStorage.setItem(this.CHAVE, JSON.stringify(m));
  },

  admin() { return adminEmMemoria; },
};

// O token de admin vem DO SERVIDOR, não do utilizador.
//
// A API exige-o em `/admin/*` — medido, `401` sem ele — mas fazer alguém
// escrevê-lo num painel que só existe com WA_API_DEV_UI ligado é pedir uma
// senha para entrar numa sala que já está aberta.
//
// A consequência de segurança está escrita em devui.go, no comentário de
// Handler, e não é pequena: quem alcança o painel passa a ter acesso
// administrativo. A proteção real é, e sempre foi, a variável de ambiente.
let adminEmMemoria = "";

export async function carregarConfig() {
  try {
    const r = await fetch("config", { cache: "no-store" });
    if (!r.ok) return false;
    const c = await r.json();
    adminEmMemoria = c.adminToken || "";
    // EM MEMÓRIA, e não no localStorage: o token vem do servidor a cada
    // carregamento, então guardá-lo só criaria uma cópia que sobrevive ao
    // servidor que a emitiu — e que fica desatualizada em silêncio quando ele
    // é reiniciado com outro.
    return adminEmMemoria !== "";
  } catch {
    return false;
  }
}

// listarSessoes devolve a lista COMO A API A VÊ, com o token local anexado
// quando existir.
//
// `temToken` é explícito em vez de se inferir de `token === ""`: quem lê a
// lista tem de poder distinguir "sessão sem credencial neste navegador" de
// "sessão sem credencial nenhuma", e essas são coisas diferentes.
export async function listarSessoes() {
  const r = await API.admin("GET", "/admin/users");
  if (!r.ok) return { ok: false, status: r.status, sessoes: [] };
  const sessoes = (r.body?.data || []).map((u) => ({
    id: u.id,
    nome: u.name,
    jid: u.jid || "",
    conectado: !!u.connected,
    autenticado: !!u.logged_in,
    token: Tokens.de(u.id),
    temToken: !!Tokens.de(u.id),
  }));
  return { ok: true, status: r.status, sessoes };
}

// soDigitos normaliza um telefone para o que as rotas de envio esperam.
//
// As rotas aceitam o número nu e resolvem o servidor sozinhas; mandar
// `+55 (11) 99999-9999` faz o resolvedor recusar. Normalizar aqui evita que
// cada modal reinvente a regra — e que a reinvente de forma diferente.
export const soDigitos = (v) => String(v || "").replace(/\D+/g, "");

export const $ = (id) => document.getElementById(id);

// novoToken gera o token de uma sessão nova.
//
// `wa_noise_` + 32 hex de crypto.getRandomValues, que são 128 bits de
// entropia. Gerado no cliente porque é o cliente que o guarda: pedi-lo ao
// servidor obrigaria a uma rota nova para produzir um valor que ninguém
// precisa de reproduzir.
//
// getRandomValues e NÃO Math.random: este valor é a credencial da sessão, e
// Math.random é previsível por desenho — o gerador do V8 não é
// criptográfico, e um token adivinhável é um token que não existe.
export function novoToken() {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  return "wa_noise_" + [...b].map((n) => n.toString(16).padStart(2, "0")).join("");
}
