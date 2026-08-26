// Package middleware provides HTTP middleware handlers extracted from
// root wa-api/auth.go during Phase 13d of the Clean Architecture refactor.
// Functions accept all dependencies as parameters — no root imports.
package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
)

var errUnauthorized = &apperr.AppError{
	Code:     "unauthorized",
	Category: apperr.CategoryUnauthorized,
	Message:  "unauthorized",
}

// Values is a string map for user attributes carried through request context.
type Values struct {
	M map[string]string
}

// Get returns the value for key or empty string.
func (v Values) Get(key string) string {
	if v.M == nil {
		return ""
	}
	return v.M[key]
}

// NewValues creates a Values from a map.
func NewValues(m map[string]string) Values { return Values{M: m} }

// AuthAdmin returns middleware that validates the Authorization header.
func AuthAdmin(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			tokenHash := sha256.Sum256([]byte(token))
			adminHash := sha256.Sum256([]byte(adminToken))
			if subtle.ConstantTimeCompare(tokenHash[:], adminHash[:]) != 1 {
				hlog.FromRequest(r).Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Str("remote_addr", r.RemoteAddr).
					Bool("token_present", token != "").
					Msg("admin authentication rejected")
				customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// userCacheTTL limita por quanto tempo uma autenticação bem-sucedida sobrevive
// sem voltar ao banco. Antes era cache.NoExpiration, o que fazia revogar ou
// deletar um usuário não ter efeito nenhum enquanto o processo vivesse
// (sec/F11). 10 minutos é o teto de exposição pós-revogação; abaixo disso o
// custo por request de um SELECT indexado ainda é irrelevante frente ao ganho.
const userCacheTTL = 10 * time.Minute

// extractRequestToken lê o token do header e, se ausente, da query string.
//
// A query string continua aceita nesta release para não quebrar clientes que
// dependem dela, mas cada uso emite WARN com identificação do chamador. A
// remoção é a release seguinte. O token nunca é logado — só quem o mandou e
// para onde.
func extractRequestToken(r *http.Request) string {
	if token := r.Header.Get("token"); token != "" {
		return token
	}

	token := strings.Join(r.URL.Query()["token"], "")
	if token == "" {
		return ""
	}

	if !queryTokenAllowed(r.URL.Path) {
		log.Warn().
			Str("remote_addr", r.RemoteAddr).
			Str("path", r.URL.Path).
			Str("user_agent", r.UserAgent()).
			Msg("token por query string recusado nesta rota; use o header `token`")
		return ""
	}

	log.Debug().
		Str("path", r.URL.Path).
		Msg("token por query string aceito: a rota de WebSocket nao tem alternativa")
	return token
}

// wsPath é a única rota onde o token pode vir por query string.
const wsPath = "/session/ws"

// queryTokenAllowed decide se aquela rota aceita token na URL.
//
// Só o WebSocket, e a razão é da especificação, não nossa: a API `WebSocket` do
// navegador NÃO permite header customizado no handshake — não há `headers` em
// `new WebSocket(url, protocols)`. Um painel web não tem outra forma de
// autenticar ali, e recusar a query string nessa rota quebraria todo cliente de
// navegador sem oferecer saída (F75).
//
// Nas demais rotas o header sempre foi possível, e a query só sobrevivia por
// compatibilidade. Aqui ela deixa de ser aceita — e o token que viaja na URL da
// exceção é redigido no log (ver url_redaction.go), senão a exceção
// justificada viraria credencial registrada.
//
// O destino declarado é o subprotocolo (`new WebSocket(url, [token])`), que
// tira o token da URL de vez. Ele exige mudança em TODO cliente WebSocket, e
// por isso não cabe na mesma janela — ver ADR-0006.
func queryTokenAllowed(path string) bool {
	return path == wsPath
}

// AuthAlice returns middleware that looks up a user by token.
func AuthAlice(db *sql.DB, userCache *cache.Cache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var ctx context.Context
			txtid := ""
			name := ""
			webhook := ""
			jid := ""
			events := ""
			proxyURL := ""
			qrcode := ""
			var hasHmac bool

			token := extractRequestToken(r)

			// Sem token não se consulta o banco. A consulta casa por
			// `token = $1`, e aqui $1 seria "" — bastaria UMA linha com token
			// vazio para a requisição anônima autenticar como aquele usuário.
			// Medido na F100: a resposta caiu de 401 para 400 no instante em
			// que a coluna foi branqueada.
			//
			// Hoje nenhuma linha tem token vazio, então esta guarda não muda
			// nada observável. Ela existe para que a etapa 1 da F97 — que vai
			// branquear a coluna — não vire um acesso sem credencial.
			if token == "" {
				hlog.FromRequest(r).Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("authentication rejected: request carries no token")
				customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
				return
			}

			myuserinfo, found := userCache.Get(token)
			if !found {
				hlog.FromRequest(r).Debug().
					Str("path", r.URL.Path).
					Msg("token not in cache; looking up user in DB")
				rows, err := db.Query(
					"SELECT id,name,webhook,jid,events,proxy_url,qrcode,history,"+
						"hmac_key IS NOT NULL AND length(hmac_key) > 0,"+
						"CASE WHEN s3_enabled THEN 'true' ELSE 'false' END,"+
						"COALESCE(media_delivery, 'base64') "+
						"FROM users WHERE token_hash=$1 LIMIT 1",
					domain.HashToken(token),
				)
				if err != nil {
					hlog.FromRequest(r).Error().Err(err).
						Str("path", r.URL.Path).
						Msg("user lookup query failed")
					customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
					return
				}
				defer func() {
					if cerr := rows.Close(); cerr != nil {
						log.Warn().Err(cerr).Msg("error closing rows")
					}
				}()
				var history sql.NullInt64
				var s3Enabled, mediaDelivery string
				for rows.Next() {
					err = rows.Scan(&txtid, &name, &webhook, &jid, &events, &proxyURL, &qrcode, &history, &hasHmac, &s3Enabled, &mediaDelivery)
					if err != nil {
						hlog.FromRequest(r).Error().Err(err).
							Str("path", r.URL.Path).
							Msg("scanning user row failed")
						customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
						return
					}
					historyStr := "0"
					if history.Valid {
						historyStr = fmt.Sprintf("%d", history.Int64)
					}
					v := Values{M: map[string]string{
						"Id": txtid, "Name": name, "Jid": jid, "Webhook": webhook,
						"Token": token, "Proxy": proxyURL, "Events": events,
						"Qrcode": qrcode, "History": historyStr,
						"HasHmac":   strconv.FormatBool(hasHmac),
						"S3Enabled": s3Enabled, "MediaDelivery": mediaDelivery,
					}}
					userCache.Set(token, v, userCacheTTL)
					ctx = context.WithValue(r.Context(), appport.UserInfoKey, v)
				}
			} else {
				v := myuserinfo.(Values)
				ctx = context.WithValue(r.Context(), appport.UserInfoKey, v)
				txtid = v.Get("Id")
			}

			if txtid == "" {
				hlog.FromRequest(r).Warn().
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Bool("token_present", token != "").
					Bool("cache_hit", found).
					Msg("authentication rejected: no user matches the supplied token")
				customhttp.RespondJSON(w, http.StatusUnauthorized, nil, errUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveConnectEvents decides subscription string on reconnect.
func ResolveConnectEvents(findInSlice func([]string, string) bool, supported []string, subscribe []string, existing string) (string, bool) {
	if len(subscribe) < 1 {
		return existing, false
	}
	var subscribed []string
	for _, arg := range subscribe {
		if !findInSlice(supported, arg) {
			log.Warn().Str("Type", arg).Msg("Event type discarded")
			continue
		}
		if !findInSlice(subscribed, arg) {
			subscribed = append(subscribed, arg)
		}
	}
	resolved := strings.Join(subscribed, ",")
	return resolved, resolved != existing
}
