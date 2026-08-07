// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/beeper/argo-go/codec"
	"github.com/beeper/argo-go/pkg/buf"

	"wa-api/internal/wa-noise/argo"
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waWa6"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

const (
	queryFetchNewsletter           = "6563316087068696"
	queryFetchNewsletterDehydrated = "7272540469429201"
	queryRecommendedNewsletters    = "7263823273662354" // variables -> input -> {limit: 20, country_codes: [string]}, output: xwa2_newsletters_recommended
	queryNewslettersDirectory      = "6190824427689257" // variables -> input -> {view: "RECOMMENDED", limit: 50, start_cursor: base64, filters: {country_codes: [string]}}
	querySubscribedNewsletters     = "6388546374527196" // variables -> empty, output: xwa2_newsletter_subscribed
	queryNewsletterSubscribers     = "9800646650009898" // variables -> input -> {newsletter_id, count}, output: xwa2_newsletter_subscribers -> subscribers -> edges
	mutationMuteNewsletter         = "6274038279359549" // variables -> {newsletter_id, updates->{description, settings}}, output: xwa2_newsletter_update -> NewsletterMetadata without viewer meta
	mutationUnmuteNewsletter       = "6068417879924485"
	mutationUpdateNewsletter       = "7150902998257522"
	mutationCreateNewsletter       = "6234210096708695"
	mutationUnfollowNewsletter     = "6392786840836363"
	mutationFollowNewsletter       = "9926858900719341"

	// desktop & mobile
	queryFetchNewsletterDesktop        = "9779843322044422"
	queryRecommendedNewslettersDesktop = "27256776790637714"
	querySubscribedNewslettersDesktop  = "8621797084555037"
	queryNewsletterSubscribersDesktop  = "25403502652570342"
	mutationMuteNewsletterDesktop      = "5971669009605755" // variables -> {newsletter_id, updates->{description, settings}}, output: xwa2_newsletter_update -> NewsletterMetadata without viewer meta
	mutationUnmuteNewsletterDesktop    = "6104029483058502"
	mutationUpdateNewsletterDesktop    = "7839742399440946"
	mutationCreateNewsletterDesktop    = "27527996220149684"
	mutationUnfollowNewsletterDesktop  = "8782612271820087"
	mutationFollowNewsletterDesktop    = "8621797084555037"
)

const (
	// mexNamespace é o namespace <iq> das consultas GraphQL/MEX do WhatsApp.
	mexNamespace = "w:mex"
	// mexQueryTag / mexQueryIDAttr / mexResultTag nomeiam o nó de requisição e
	// o de resposta do MEX, que aparecem tanto na construção quanto na leitura.
	mexQueryTag    = "query"
	mexQueryIDAttr = "query_id"
	mexResultTag   = "result"
	// mexFormatAttr / mexFormatArgo identificam a resposta codificada em Argo
	// (em vez de JSON puro).
	mexFormatAttr = "format"
	mexFormatArgo = "argo"
)

// errArgoDecodingBroken é devolvido enquanto o caminho de decodificação Argo
// estiver desabilitado no fork. Ver PATCHES.md (Fase E, lote 2).
var errArgoDecodingBroken = errors.New("argo decoding is currently broken")

func convertQueryID(cli *Client, queryID string) string {
	if payload := cli.Store.GetClientPayload(); payload.GetUserAgent().Platform == waWa6.ClientPayload_UserAgent_MACOS.Enum() || payload.GetWebInfo() == nil {
		switch queryID {
		case queryFetchNewsletter:
			return queryFetchNewsletterDesktop
		case queryRecommendedNewsletters:
			return queryRecommendedNewslettersDesktop
		case querySubscribedNewsletters:
			return querySubscribedNewslettersDesktop
		case queryNewsletterSubscribers:
			return queryNewsletterSubscribersDesktop
		case mutationMuteNewsletter:
			return mutationMuteNewsletterDesktop
		case mutationUnmuteNewsletter:
			return mutationUnmuteNewsletterDesktop
		case mutationUpdateNewsletter:
			return mutationUpdateNewsletterDesktop
		case mutationCreateNewsletter:
			return mutationCreateNewsletterDesktop
		case mutationUnfollowNewsletter:
			return mutationUnfollowNewsletterDesktop
		case mutationFollowNewsletter:
			return mutationFollowNewsletterDesktop
		default:
			return queryID
		}
	} else {
		return queryID
	}
}

func (cli *Client) sendMexIQ(ctx context.Context, queryID string, variables any) (json.RawMessage, error) {
	if store.BaseClientPayload.GetUserAgent().GetPlatform() == waWa6.ClientPayload_UserAgent_MACOS {
		return nil, errArgoDecodingBroken
	}
	queryID = convertQueryID(cli, queryID)
	payload, err := json.Marshal(map[string]any{
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: mexNamespace,
		Type:      iqGet,
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
		return nil, &ElementMissingError{Tag: mexResultTag, In: "mex response"}
	}
	resultContent, ok := result.Content.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected content type %T in mex response", result.Content)
	}
	if result.AttrGetter().OptionalString(mexFormatAttr) == mexFormatArgo {
		if true {
			return nil, errArgoDecodingBroken
		}
		store, err := argo.GetStore()
		if err != nil {
			return nil, err
		}
		queryIDMap, err := argo.GetQueryIDToMessageName()
		if err != nil {
			return nil, err
		}
		wt := store[queryIDMap[queryID]]

		decoder, err := codec.NewArgoDecoder(buf.NewBufReadonly(resultContent))
		if err != nil {
			return nil, err
		}
		data, err := decoder.ArgoToMap(wt)
		if err != nil {
			cli.Log.Errorf("Failed to decode argo mex response for query %s: %v", queryID, err)
			return nil, fmt.Errorf("failed to decode argo mex response: %w", err)
		}
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return b, nil
	} else {
		var gqlResp types.GraphQLResponse
		err = json.Unmarshal(resultContent, &gqlResp)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal graphql response: %w", err)
		} else if len(gqlResp.Errors) > 0 {
			return gqlResp.Data, fmt.Errorf("graphql error: %w", gqlResp.Errors)
		}
		return gqlResp.Data, nil
	}
}
