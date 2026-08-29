package session

import "wa-api/pkg/domain"

// Presenters are hand-written, one function per type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.GetStatusResult and
// this file stops building. A reflection-based mapper would keep building and
// would silently drop the key from every response, which is the exact failure
// the DTO layer exists to prevent.

// PresentConnect maps the connect result onto the route's `data`.
//
// It ignores domain.ConnectResult entirely, and that is not an oversight: the
// use case answers an empty value because the WhatsApp client is started in a
// goroutine AFTER the response is written, so there is nothing true to report
// but the state.
func PresentConnect(_ *domain.ConnectResult) ConnectResponse {
	return ConnectResponse{Status: ConnectingStatus}
}

// PresentDisconnect maps the disconnect result.
func PresentDisconnect(r *domain.DisconnectResult) DisconnectResponse {
	if r == nil {
		return DisconnectResponse{}
	}
	return DisconnectResponse{Details: r.Details}
}

// PresentGetQR maps the QR read.
func PresentGetQR(r *domain.GetQRResult) GetQRResponse {
	if r == nil {
		return GetQRResponse{}
	}
	return GetQRResponse{QRCode: r.QRCode, CodeAgeSeconds: r.CodeAgeSeconds}
}

// PresentLogout maps the logout result.
func PresentLogout(r *domain.LogoutResult) LogoutResponse {
	if r == nil {
		return LogoutResponse{}
	}
	return LogoutResponse{Details: r.Details}
}

// PresentPairPhone maps the phone-pairing result.
func PresentPairPhone(r *domain.PairPhoneResult) PairPhoneResponse {
	if r == nil {
		return PairPhoneResponse{}
	}
	return PairPhoneResponse{LinkingCode: r.LinkingCode}
}

// PresentProxySummary maps the proxy summary embedded in the status.
func PresentProxySummary(p domain.ProxySummary) ProxyConfigResponse {
	return ProxyConfigResponse{
		Enabled:  p.Enabled,
		ProxyURL: p.ProxyURL,
	}
}

// PresentS3Summary maps the S3 summary embedded in the status.
func PresentS3Summary(s domain.S3Summary) S3ConfigSummaryResponse {
	return S3ConfigSummaryResponse{
		Enabled:       s.Enabled,
		Endpoint:      s.Endpoint,
		Region:        s.Region,
		Bucket:        s.Bucket,
		PathStyle:     s.PathStyle,
		PublicURL:     s.PublicURL,
		MediaDelivery: s.MediaDelivery,
		RetentionDays: s.RetentionDays,
	}
}

// PresentGetStatus maps the session status.
//
// domain.GetStatusResult.Token is NOT presented, and the omission is the point:
// the field exists so the record can be read in one query, not so the session
// token is echoed back on every status poll.
func PresentGetStatus(r *domain.GetStatusResult) GetStatusResponse {
	if r == nil {
		return GetStatusResponse{}
	}
	return GetStatusResponse{
		ID:        r.ID,
		Name:      r.Name,
		Connected: r.Connected,
		LoggedIn:  r.LoggedIn,
		JID:       r.Jid,
		Webhook:   r.Webhook,
		Events:    r.Events,
		ProxyURL:  r.ProxyURL,
		QRCode:    r.Qrcode,
		History:   r.History,

		ProxyConfig:    PresentProxySummary(r.ProxyConfig),
		S3Config:       PresentS3Summary(r.S3Config),
		HMACConfigured: r.HMACConfigured,
	}
}

// PresentSetStatusMessage maps the status-message write.
func PresentSetStatusMessage(r *domain.SetStatusMessageResult) SetStatusMessageResponse {
	if r == nil {
		return SetStatusMessageResponse{}
	}
	return SetStatusMessageResponse{Details: r.Details}
}

// PresentRequestHistorySync maps the history-sync request result.
func PresentRequestHistorySync(r *domain.RequestHistorySyncResult) RequestHistorySyncResponse {
	if r == nil {
		return RequestHistorySyncResponse{}
	}
	return RequestHistorySyncResponse{
		Details:            r.Details,
		Timestamp:          r.Timestamp,
		Count:              r.Count,
		ChatJID:            r.ChatJid,
		OldestMsgID:        r.OldestMsgID,
		OldestMsgFromMe:    r.OldestMsgFromMe,
		OldestMsgTimestamp: r.OldestMsgTimestamp,
	}
}

// PresentSyncContactRoster maps the contact-roster sync result.
func PresentSyncContactRoster(r *domain.SyncContactRosterResult) SyncContactRosterResponse {
	if r == nil {
		return SyncContactRosterResponse{}
	}
	return SyncContactRosterResponse{Details: r.Details, Mode: r.Mode}
}

// PresentPublishStatusImage maps the image status publication.
func PresentPublishStatusImage(r *domain.PublishStatusImageResult) PublishStatusResponse {
	if r == nil {
		return PublishStatusResponse{}
	}
	return PublishStatusResponse{
		MessageID: r.MessageID,
		Timestamp: r.Timestamp,
		Status:    r.Status,
	}
}

// PresentPublishStatusVideo maps the video status publication.
func PresentPublishStatusVideo(r *domain.PublishStatusVideoResult) PublishStatusResponse {
	if r == nil {
		return PublishStatusResponse{}
	}
	return PublishStatusResponse{
		MessageID: r.MessageID,
		Timestamp: r.Timestamp,
		Status:    r.Status,
	}
}

// PresentPublishStatusAudio maps the audio status publication.
func PresentPublishStatusAudio(r *domain.PublishStatusAudioResult) PublishStatusResponse {
	if r == nil {
		return PublishStatusResponse{}
	}
	return PublishStatusResponse{
		MessageID: r.MessageID,
		Timestamp: r.Timestamp,
		Status:    r.Status,
	}
}
