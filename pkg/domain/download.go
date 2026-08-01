// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// DownloadRequest representa o payload de download de mídia.
type DownloadRequest struct {
	URL           string `json:"Url"`
	DirectPath    string `json:"DirectPath"`
	MediaKey      []byte `json:"MediaKey"`
	Mimetype      string `json:"Mimetype"`
	FileEncSHA256 []byte `json:"FileEncSHA256"`
	FileSHA256    []byte `json:"FileSHA256"`
	FileLength    uint64 `json:"FileLength"`
}

// DownloadResult representa o resultado do download de mídia.
type DownloadResult struct {
	Mimetype string `json:"Mimetype"`
	Data     string `json:"Data"` // base64 data URL
}
