// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package newsletter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beeper/argo-go/codec"
	"github.com/beeper/argo-go/pkg/buf"

	"wa-api/internal/wa-noise/protocol/argo"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// SendMexIQ manda uma consulta GraphQL/MEX e devolve o campo "data" da resposta.
//
// O guard de MACOS vem antes de qualquer uso de t: em um cliente cujo payload
// base declara a plataforma MACOS o caminho Argo seria necessario, e ele esta'
// desabilitado no fork (ver ErrArgoDecodingBroken). Isso torna este o unico
// caminho de SendMexIQ exercitavel sem sessao Noise aberta.
func SendMexIQ(ctx context.Context, t Transport, queryID string, variables any) (json.RawMessage, error) {
	if store.BaseClientPayload.GetUserAgent().GetPlatform() == waWa6.ClientPayload_UserAgent_MACOS {
		return nil, ErrArgoDecodingBroken
	}
	queryID = ConvertQueryID(t.ClientPayload(), queryID)
	payload, err := json.Marshal(map[string]any{
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: mexNamespace,
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: mexQueryTag,
			Attrs: waBinary.Attrs{
				mexQueryIDAttr: queryID,
			},
			Content: payload,
		}},
	})
	if err != nil {
		return nil, err
	}
	result, ok := resp.GetOptionalChildByTag(mexResultTag)
	if !ok {
		return nil, t.ElementMissing(mexResultTag, mexErrContext)
	}
	resultContent, ok := result.Content.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected content type %T in mex response", result.Content)
	}
	if result.AttrGetter().OptionalString(mexFormatAttr) == mexFormatArgo {
		return decodeArgoResult(t, queryID, resultContent)
	}
	return decodeGraphQLResult(resultContent)
}

// decodeArgoResult interpreta uma resposta MEX codificada em Argo.
//
// O caminho esta' DESABILITADO: o early return abaixo e' incondicional e todo o
// resto e' inalcancavel hoje. Fica no lugar porque e' a implementacao que sera'
// reabilitada quando o decoder Argo voltar a funcionar — ver PATCHES.md
// (Fase E, lote 2). Nao ha' como cobri-lo por teste sem alterar producao.
func decodeArgoResult(t Transport, queryID string, resultContent []byte) (json.RawMessage, error) {
	if true {
		return nil, ErrArgoDecodingBroken
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		return nil, err
	}
	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		return nil, err
	}
	wt := wireStore[queryIDMap[queryID]]

	decoder, err := codec.NewArgoDecoder(buf.NewBufReadonly(resultContent))
	if err != nil {
		return nil, err
	}
	data, err := decoder.ArgoToMap(wt)
	if err != nil {
		t.Log().Errorf("Failed to decode argo mex response for query %s: %v", queryID, err)
		return nil, fmt.Errorf("failed to decode argo mex response: %w", err)
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// decodeGraphQLResult interpreta uma resposta MEX em JSON puro.
//
// Quando o servidor responde com erros GraphQL, o "data" parcial e' devolvido
// JUNTO com o erro — os chamadores (GetInfo, GetSubscribed) dependem disso para
// preferir o erro do servidor ao erro de unmarshal.
func decodeGraphQLResult(resultContent []byte) (json.RawMessage, error) {
	var gqlResp types.GraphQLResponse
	err := json.Unmarshal(resultContent, &gqlResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal graphql response: %w", err)
	} else if len(gqlResp.Errors) > 0 {
		return gqlResp.Data, fmt.Errorf("graphql error: %w", gqlResp.Errors)
	}
	return gqlResp.Data, nil
}
