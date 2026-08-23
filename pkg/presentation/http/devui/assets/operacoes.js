// Catálogo das operações que um card oferece.
//
// É DADO, e não código, de propósito: acrescentar uma rota passa a ser uma
// linha nesta tabela em vez de um bloco de HTML e um handler novo. O painel
// tem hoje 14 rotas de envio e 7 de chat; escrevê-las à mão uma a uma seria
// garantir que a próxima ficasse de fora — que foi exatamente como a
// HOUSEKEEP F190 nasceu do lado do servidor.
//
// Cada campo declara: nome no JSON, rótulo, tipo do controlo, e se é
// obrigatório. `pre` transforma o valor antes de o enviar.

import { soDigitos } from "./devui.js";

const telefone = { nome: "Phone", rotulo: "telefone (só dígitos)", pre: soDigitos, req: true };
const legenda = { nome: "Caption", rotulo: "legenda (opcional)" };
const replyTo = { nome: "ReplyTo", rotulo: "reply-to (JSON, opcional)", tipo: "textarea",
  pre: (v) => { const s = String(v || "").trim(); return s ? JSON.parse(s) : undefined; } };

// ficheiro devolve um campo que lê um ficheiro local e o converte em data URI,
// que é o formato que as rotas de mídia aceitam além de URL.
const ficheiro = (nome, rotulo) => ({ nome, rotulo, tipo: "file", req: true });

export const ENVIO = [
  { id: "text", rotulo: "Texto", rota: "/chat/send/text",
    campos: [telefone, { nome: "Body", rotulo: "mensagem", tipo: "textarea", req: true }, replyTo] },

  { id: "image", rotulo: "Imagem", rota: "/chat/send/image",
    campos: [telefone, ficheiro("Image", "imagem"), legenda, replyTo] },

  { id: "video", rotulo: "Vídeo", rota: "/chat/send/video",
    campos: [telefone, ficheiro("Video", "vídeo"), legenda, replyTo] },

  // A legenda do áudio existe, mas NÃO é um campo do protocolo: o WhatsApp não
  // tem legenda em áudio, e a API envia-a como mensagem de texto SEPARADA, logo
  // a seguir (F116). São duas mensagens e dois ids — o painel diz isso no
  // rótulo para o operador não ser surpreendido pelo que vê no telemóvel.
  { id: "audio", rotulo: "Áudio", rota: "/chat/send/audio",
    campos: [telefone, ficheiro("Audio", "áudio"),
             { nome: "Caption", rotulo: "legenda (vai como mensagem separada)" }, replyTo] },

  { id: "document", rotulo: "Documento", rota: "/chat/send/document",
    campos: [telefone, ficheiro("Document", "ficheiro"),
             { nome: "FileName", rotulo: "nome do ficheiro" }, replyTo] },

  { id: "sticker", rotulo: "Sticker", rota: "/chat/send/sticker",
    campos: [telefone, ficheiro("Sticker", "webp 512×512"), replyTo] },

  { id: "location", rotulo: "Localização", rota: "/chat/send/location",
    campos: [telefone, { nome: "Name", rotulo: "nome do local" },
             { nome: "Latitude", rotulo: "latitude", tipo: "number", pre: Number, req: true },
             { nome: "Longitude", rotulo: "longitude", tipo: "number", pre: Number, req: true },
             replyTo] },

  { id: "contact", rotulo: "Contacto", rota: "/chat/send/contact",
    campos: [telefone, { nome: "Name", rotulo: "nome", req: true },
             { nome: "Vcard", rotulo: "vCard", tipo: "textarea", req: true,
               valor: "BEGIN:VCARD\nVERSION:3.0\nFN:Nome\nTEL;type=CELL:+5511999999999\nEND:VCARD" },
             replyTo] },

  { id: "poll", rotulo: "Enquete", rota: "/chat/send/poll",
    // A rota chama ao destinatário `Group`, e não `Phone`, apesar de aceitar
    // um número normal. É contrato existente — renomear aqui esconderia a
    // inconsistência do servidor em vez de a mostrar.
    campos: [{ nome: "Group", rotulo: "telefone ou grupo", pre: soDigitos, req: true },
             { nome: "Header", rotulo: "pergunta", req: true },
             { nome: "Options", rotulo: "opções (uma por linha)", tipo: "textarea", req: true,
               pre: (v) => String(v).split("\n").map((s) => s.trim()).filter(Boolean) },
             replyTo] },

  { id: "pollvote", rotulo: "Voto em Enquete", rota: "/chat/send/pollvote",
    campos: [telefone,
             { nome: "Sender", rotulo: "remetente da enquete (JID)", req: true },
             { nome: "PollMessageId", rotulo: "ID da enquete", req: true },
             { nome: "PollMessageTimestamp", rotulo: "timestamp da enquete (unix)", req: true,
               pre: (v) => Number(v) },
             { nome: "Options", rotulo: "opções escolhidas (uma por linha)", tipo: "textarea", req: true,
               pre: (v) => String(v).split("\n").map((s) => s.trim()).filter(Boolean) }] },

  { id: "buttons", rotulo: "Botões", rota: "/chat/send/buttons",
    campos: [telefone, { nome: "Title", rotulo: "título" },
             { nome: "Body", rotulo: "corpo", req: true },
             { nome: "Footer", rotulo: "rodapé" },
             // `reply` e não `quickreply`: os tipos aceites são reply,
             // cta_url, cta_call e copy, e um tipo fora deles faz o botão
             // SUMIR em silêncio (HOUSEKEEP F185). O valor de arranque evita
             // que quem experimenta caia nessa.
             { nome: "Buttons", rotulo: "botões (JSON)", tipo: "textarea", req: true,
               valor: '[{"type":"reply","title":"Sim"},{"type":"reply","title":"Não"}]',
               pre: JSON.parse },
             replyTo] },

  { id: "carousel", rotulo: "Carrossel", rota: "/chat/send/carousel",
    campos: [telefone, { nome: "Body", rotulo: "corpo do carrossel", req: true },
             { nome: "Footer", rotulo: "rodapé" },
             { nome: "Cards", rotulo: "cartões (JSON — Title só aparece no Android)", tipo: "textarea", req: true,
               valor: '[{"Title":"Cartão 1","Body":"Descrição","Buttons":[{"type":"reply","title":"Sim"}]},{"Title":"Cartão 2","Body":"Descrição","Buttons":[{"type":"reply","title":"Não"}]}]',
               pre: JSON.parse },
             replyTo] },

  { id: "template", rotulo: "Template", rota: "/chat/send/template",
    campos: [telefone, { nome: "Content", rotulo: "conteúdo", req: true },
             { nome: "Footer", rotulo: "rodapé" },
             { nome: "Buttons", rotulo: "botões (JSON)", tipo: "textarea",
               valor: '[{"DisplayText":"Ok","Type":"quickreply","Id":"t1"}]', pre: JSON.parse },
             replyTo] },

  { id: "list", rotulo: "Lista", rota: "/chat/send/list",
    campos: [telefone, { nome: "ButtonText", rotulo: "texto do botão", req: true },
             { nome: "TopText", rotulo: "título" },
             { nome: "Desc", rotulo: "descrição", req: true },
             { nome: "FooterText", rotulo: "rodapé" },
             { nome: "Sections", rotulo: "secções (JSON)", tipo: "textarea", req: true,
               valor: '[{"title":"Secção","rows":[{"title":"Item","desc":"d","rowid":"r1"}]}]',
               pre: JSON.parse },
             replyTo] },

  { id: "edit", rotulo: "Editar mensagem", rota: "/chat/send/edit",
    campos: [telefone, { nome: "Id", rotulo: "id da mensagem", req: true },
             { nome: "Body", rotulo: "novo texto", tipo: "textarea", req: true }] },
];

export const CHAT = [
  { id: "list", rotulo: "Listar conversas", rota: "/chat/list", metodo: "GET",
    query: [{ nome: "limit", rotulo: "limite", valor: "30" }] },

  { id: "history", rotulo: "Histórico", rota: "/chat/history", metodo: "GET",
    query: [{ nome: "chat_jid", rotulo: "chat_jid", req: true },
            { nome: "limit", rotulo: "limite", valor: "20" }] },

  { id: "react", rotulo: "Reagir", rota: "/chat/react",
    campos: [telefone, { nome: "Id", rotulo: "id da mensagem", req: true },
             { nome: "Body", rotulo: "emoji (vazio remove)", valor: "👍" }] },

  { id: "markread", rotulo: "Marcar lido", rota: "/chat/markread",
    campos: [{ nome: "ChatPhone", rotulo: "telefone do chat", pre: soDigitos, req: true },
             { nome: "SenderPhone", rotulo: "telefone do remetente", pre: soDigitos, req: true },
             { nome: "Id", rotulo: "ids (um por linha)", tipo: "textarea", req: true,
               pre: (v) => String(v).split("\n").map((s) => s.trim()).filter(Boolean) }] },

  { id: "presence", rotulo: "Presença", rota: "/chat/presence",
    campos: [telefone, { nome: "State", rotulo: "estado", tipo: "select", req: true,
                         opcoes: ["composing", "paused", "recording"] }] },

  { id: "delete", rotulo: "Apagar mensagem", rota: "/chat/delete/message", perigo: true,
    // IRREVERSÍVEL do lado de quem recebeu. O aviso está no `perigo`, que faz
    // o painel pedir confirmação antes de executar.
    campos: [telefone, { nome: "Id", rotulo: "id da mensagem", req: true }] },
];

// Os downloads ficam de fora do menu por agora: exigem os campos de cifragem
// do evento recebido (Url, DirectPath, MediaKey, FileEncSHA256…), que só
// existem no webhook, e um formulário com seis campos opacos ensinaria menos
// do que esconde. Registado assim, e não esquecido.
