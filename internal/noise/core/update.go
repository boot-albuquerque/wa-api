package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/socket"
)

var clientVersionRegex = regexp.MustCompile(`"client_revision":(\d+),`)

const (
	// latestVersionUserAgent finge ser um Chrome de desktop: web.whatsapp.com
	// so' devolve o bundle com client_revision para navegadores que reconhece.
	latestVersionUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
	latestVersionAccept    = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"

	// Os dois primeiros componentes da versao do WhatsApp Web sao fixos; so' o
	// terceiro (client_revision) e lido da pagina.
	latestVersionMajor = 2
	latestVersionMinor = 3000
)

// GetLatestVersion returns the latest version number from web.whatsapp.com.
//
// After fetching, you can update the version to use using store.SetWAVersion, e.g.
//
//	latestVer, err := GetLatestVersion(nil)
//	if err != nil {
//		return err
//	}
//	store.SetWAVersion(*latestVer)
func GetLatestVersion(ctx context.Context, httpClient *http.Client) (*store.WAVersionContainer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, socket.Origin, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", latestVersionUserAgent)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Accept", latestVersionAccept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response with status %d: %s", resp.StatusCode, data)
	} else if match := clientVersionRegex.FindSubmatch(data); len(match) == 0 {
		return nil, fmt.Errorf("version number not found")
	} else if parsedVer, err := strconv.ParseInt(string(match[1]), 10, 64); err != nil {
		return nil, fmt.Errorf("failed to parse version number: %w", err)
	} else {
		return &store.WAVersionContainer{latestVersionMajor, latestVersionMinor, uint32(parsedVer)}, nil
	}
}
