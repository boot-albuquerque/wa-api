# Armadilhas deste projeto

Catálogo de defeitos que **já passaram por revisão e por testes verdes** neste
repositório. Não é teoria: cada entrada aconteceu, tem a evidência medida e o
que a expôs.

Leia antes de mexer em identidade (LID/PN), rotas HTTP, junção de dados ou
qualquer coisa que grave no banco. O padrão comum de quase todas é o mesmo:
**o teste concordava com o defeito**.

---

## 1. Dublê mais permissivo que a produção esconde o defeito

O modo de falha mais frequente aqui. O teste passa, a rota real quebra.

**Aconteceu três vezes em 2026-08-08:**

| dublê | o que ele fazia | o que a produção faz | efeito |
|---|---|---|---|
| `ResolveQualifiedJID` fake | qualificava número nu | **rejeita** sem servidor | `/user/profile/5511999` dava erro só em produção |
| `spyPort.ResolveJID` | concatenava `@s.whatsapp.net` cego | preserva servidor existente | `...@s.whatsapp.net@s.whatsapp.net` |
| dublês de `list_chats` | chaveavam tudo no mesmo espaço | histórico mistura `@lid` e `@s.whatsapp.net` | 7.273 nomes no banco, 11 na lista |

**Regra**: quando um dublê imita uma REGRA (parsing, normalização, resolução),
ele tem de imitar a regra real, e o comentário do dublê deve dizer de onde ela
vem — com caminho de arquivo. Se a regra é `mapping/jid/parse.go:16-27`, o
dublê cita isso.

**Como pegar**: escreva um teste que force os dois lados a divergirem. Se o
dublê e a produção concordam sempre, o teste não está medindo a regra.

---

## 2. Testes que codificam o defeito como contrato

Testes verdes que provavam o comportamento errado.

- **F79** — `/session/disconnect` e `/session/logout` devolviam 200 sem
  encerrar nada. A suíte só exercitava a RECUSA da guarda; um use case que
  valida e não age passava com folga. **O caminho de sucesso não era testado
  por ninguém.**
- **F81** — `GET /user/lid/{jid}` dava 400 sempre. O teste servia o handler
  **cru**, sem padrão de rota, e mandava o corpo que o handler lia. Teste
  verde, rota quebrada.
- **F82** — o teste do walog rodava sem banco, e o comentário dizia "sem
  banco, sem HTTP". Aquilo só era verdade **por causa do defeito**: a guarda
  saía antes da escrita.

**Regra**: para todo use case, teste o caminho de SUCESSO, não só a recusa.
Para toda rota, teste pela rota REGISTRADA. Se um teste roda sem uma
dependência que a produção exige, pergunte por que — pode ser o bug.

---

## 3. Comentário que afirma comportamento inexistente

`nomesDoHistorico` dizia: *"a normalização acontece depois, em `montarResumo`,
que tenta as duas formas"*. **Não tentava.** O comentário foi escrito junto do
código, na mesma sessão, pelo mesmo autor.

**Regra**: comentário que descreve o que OUTRA função faz é dívida no
nascimento. Ou o teste prova a afirmação, ou ela não entra. Ao ler um
comentário assim, verifique antes de confiar.

---

## 4. Controle negativo que não compila não prova nada

Reintroduzir o defeito e ver o teste falhar é obrigatório (ver `CLAUDE.md`).
Mas várias tentativas hoje **não dispararam porque a edição quebrou o build** —
e um controle que não roda é indistinguível de um que não morde.

Casos reais:
- trocar embedding anônimo por campo nomeado → quebra promoção de campos →
  não compila. A variante que compila é manter anônimo e **adicionar tag
  JSON**.
- remover a única chamada a um pacote → import órfão → não compila. Remova o
  import junto.

**Regra**: se o controle negativo não produzir uma FALHA DE TESTE com
mensagem, ele não valeu. Ajuste a mutação até compilar e falhar.

---

## 5. Medir, não inferir — e conferir a própria medição

- Concluí que o fallback LID→PN "recuperava **zero**" a partir de um JOIN SQL
  que eu mesmo escrevi errado: `wanoise_lid_map` guarda LID e PN **sem
  sufixo** (`90937376170214`, não `90937376170214@lid`). Com o sufixo
  removido, recuperava 73. **Quase descartei o caminho por causa da minha
  consulta.**
- Reportei "17 de 481" a partir de amostra da API; contra o banco inteiro era
  "261 de 1717".

**Regra**: antes de concluir a partir de uma consulta, valide-a num caso que
você SABE que deveria casar. Uma contagem zero é suspeita, não conclusiva.

---

## 6. LID e PN são o mesmo tipo Go

`types.JID` não distingue telefone de LID — só o `Server` em tempo de
execução (`@s.whatsapp.net` vs `@lid`). O compilador não ajuda.

- **F65** — `GetManyLIDsForPNs` devolvia o mapa INVERTIDO em produção. O
  dublê estava invertido do mesmo jeito, então os dois combinavam e o teste
  ficava verde.
- **F84 (2ª parte)** — tabela de consulta num espaço, chave de busca noutro.

**Regra**: toda tabela de consulta chaveada por JID precisa ser consultada nos
DOIS espaços, ou aliasada para um só (ver `aliasarParaLID`). E todo teste de
resolução tem de fixar a DIREÇÃO: LID de entrada não pode chamar
`GetLIDForPN`, e vice-versa.

---

## 7. `ON CONFLICT DO NOTHING` torna defeito de dado irrecuperável

`message_history` usava `DO NOTHING`. Como o logout não apaga a tabela, um
HistorySync novo traz as MESMAS `message_id` e o insert era descartado
inteiro: **nenhuma instalação existente se curaria ao atualizar**.

A cláusula certa preenche o que falta sem sobrescrever o que existe:

```sql
ON CONFLICT (...) DO UPDATE SET col = EXCLUDED.col
 WHERE EXCLUDED.col <> '' AND (tabela.col IS NULL OR tabela.col = '')
```

**Regra**: ao corrigir um bug que gravou dado errado, pergunte como o dado
JÁ GRAVADO se recupera. Se a resposta for "não recupera", isso é parte do
defeito, não uma limitação aceitável.

---

## 8. Nunca sobrescrever um valor com vazio

A Evolution API ([#2426](https://github.com/EvolutionAPI/evolution-api/issues/2426))
zerava `Contact.pushName` a cada mensagem enviada porque o upsert não tinha
guarda contra vazio — enquanto o caminho de `Chat.name` tinha.

**Regra**: em qualquer upsert de nome/rótulo, a guarda `<> ''` é obrigatória.

---

## 9. Rota se testa pelo roteador, e o roteador aqui é gorilla/mux

`r.PathValue` **não funciona** neste projeto — é do `ServeMux` nativo, e o
router é `gorilla/mux` (`pkg/bootstrap/router.go:237`). Use `mux.Vars(r)`.

A correção óbvia da F81 (`r.PathValue`) estaria igualmente quebrada; foi pega
por um controle negativo dedicado.

**Regra**: teste de rota monta `mux.NewRouter()` com o MESMO padrão de
`wiring_routes.go`.

---

## 10. Endpoints de rede não se chamam em laço

Um laço de **seis** chamadas a `/user/profile` disparou
`429: rate-overlimit` do usync do WhatsApp.

**Regra**: rota que faz chamada de rede por consulta não entra em laço sobre
uma lista. Para enriquecer N itens, ou existe chamada em LOTE (como
`GetJoinedGroups`, que traz todos os nomes de grupo de uma vez) ou o
enriquecimento é sob demanda com cache.

---

## 11. Verificação em produção não é opcional

Os dois defeitos mais graves de 2026-08-08 passaram por revisão, testes
verdes e `make check` — e só apareceram **medindo contra dados reais**:

- 7.273 nomes no banco, 11 na lista.
- 0 de 23.836 mensagens com nome depois de um "fix" que os testes aprovavam.

**Regra**: para defeito de forma de dado ou de junção, o teste unitário prova
que o mecanismo funciona; só a medição em produção prova que ele funciona
**nos dados que existem**. Registre o ANTES antes de mexer — sem linha de
base, o depois não significa nada.

---

## 12. Não sobrescrever arquivo sem olhar o diff

`cp CLAUDE.md AGENTS.md` depois de o próprio script informar que os dois
**diferiam**. A perda foi uma linha (o título) e o git recuperou, mas a
decisão estava errada independentemente do tamanho do estrago.

**Regra**: antes de sobrescrever, `diff`. O aviso de que os arquivos diferem
é o momento de parar, não de continuar.
