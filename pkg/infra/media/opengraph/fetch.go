// Package opengraph provides Open Graph link-preview fetching,
// extracted from root wa-api/helpers.go (Phase 12b).
package opengraph

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/nfnt/resize"
	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/media/sticker"
)

// Result holds Open Graph metadata extracted from a URL.
type Result struct {
	Title       string
	Description string
	ImageData   []byte
	HQImageData []byte
	HQWidth     uint32
	HQHeight    uint32
}

// Default constants.
const (
	FetchTimeout    = 5 * time.Second
	PageMaxBytes    = 2 * 1024 * 1024
	ImageMaxBytes   = 10 * 1024 * 1024
	ThumbnailWidth  = 192
	ThumbnailHeight = 192
	HQThumbnailDim  = 600
	MaxImageDim     = 4000
)

// FetchURLBytes fetches a URL and returns its body bytes and content type.
// limit enforces a maximum response size. httpClient is the SSRF-safe HTTP
// client injected by root.
func FetchURLBytes(ctx context.Context, httpClient *http.Client, resourceURL string, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "WhatsApp/2.23.20.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, limit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("response exceeds allowed size (%d bytes)", limit)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// FetchOpenGraphData fetches and parses Open Graph metadata from a URL.
func FetchOpenGraphData(ctx context.Context, httpClient *http.Client, urlStr string) Result {
	pageData, _, err := FetchURLBytes(ctx, httpClient, urlStr, PageMaxBytes)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to fetch URL for Open Graph data")
		return Result{}
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageData))
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse HTML for Open Graph data")
		return Result{}
	}

	title := doc.Find(`meta[property="og:title"]`).AttrOr("content", "")
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").Text())
	}

	description := doc.Find(`meta[property="og:description"]`).AttrOr("content", "")
	if description == "" {
		description = doc.Find(`meta[name="description"]`).AttrOr("content", "")
	}

	var imageURLStr string
	selectors := []struct{ selector, attr string }{
		{`meta[property="og:image"]`, "content"},
		{`meta[property="twitter:image"]`, "content"},
		{`link[rel="apple-touch-icon"]`, "href"},
		{`link[rel="icon"]`, "href"},
	}
	for _, s := range selectors {
		imageURLStr, _ = doc.Find(s.selector).Attr(s.attr)
		if imageURLStr != "" {
			break
		}
	}

	result := Result{Title: title, Description: description}
	pageURL, err := url.Parse(urlStr)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse page URL for resolving image URL")
		return result
	}
	FetchOpenGraphImage(ctx, httpClient, pageURL, imageURLStr, &result)
	return result
}

// FetchOpenGraphImage downloads, decodes and thumbnails the OG image.
func FetchOpenGraphImage(ctx context.Context, httpClient *http.Client, pageURL *url.URL, imageURLStr string, result *Result) {
	if imageURLStr == "" {
		return
	}
	imageURL, err := url.Parse(imageURLStr)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", imageURLStr).Msg("Failed to parse Open Graph image URL")
		return
	}
	resolvedImageURL := pageURL.ResolveReference(imageURL).String()
	imgBytes, _, err := FetchURLBytes(ctx, httpClient, resolvedImageURL, ImageMaxBytes)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to fetch Open Graph image")
		return
	}

	imgConfig, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image config")
		return
	}
	if imgConfig.Width > MaxImageDim || imgConfig.Height > MaxImageDim {
		log.Warn().Int("width", imgConfig.Width).Int("height", imgConfig.Height).
			Str("imageURL", resolvedImageURL).Msg("Open Graph image dimensions too large")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image")
		return
	}

	hqThumb := resize.Thumbnail(HQThumbnailDim, HQThumbnailDim, img, resize.Lanczos3)
	result.HQImageData = sticker.EncodeJPEGThumbnail(hqThumb)
	bounds := hqThumb.Bounds()
	result.HQWidth = uint32(bounds.Dx())
	result.HQHeight = uint32(bounds.Dy())
	result.ImageData = sticker.EncodeJPEGThumbnail(
		resize.Thumbnail(ThumbnailWidth, ThumbnailHeight, hqThumb, resize.Lanczos3))
}
