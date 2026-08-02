// Package chat reúne os casos de uso das operações avulsas sobre uma
// conversa — arquivar, rejeitar chamada, pedir reenvio de mensagem não
// decifrada — que não pertencem nem ao envio de mensagens (usecase/message)
// nem à administração de grupos (usecase/group).
//
// É exatamente o recorte da porta port.ChatOperations, e não uma sobra:
// o pacote nasceu como "misc" e foi renomeado na Fase 7 justamente porque
// aquele nome não dizia o que havia dentro.
//
// Cada usecase recebe suas dependências via constructor (ports) e não
// referencia variáveis globais do package main.
package chat
