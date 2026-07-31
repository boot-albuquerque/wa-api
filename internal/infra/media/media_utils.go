package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/nfnt/resize"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

const (
	openGraphFetchTimeout    = 5 * time.Second
	openGraphPageMaxBytes    = 2 * 1024 * 1024  // 2MB
	openGraphImageMaxBytes   = 10 * 1024 * 1024 // 10MB
	openGraphThumbnailWidth  = 192
	openGraphThumbnailHeight = 192
	openGraphHQThumbnailDim  = 600 // Max dimension of the uploaded thumbnail used for the large preview card
	openGraphJpegQuality     = 80
	openGraphMaxImageDim     = 4000 // Max width or height for Open Graph images
	openGraphUserFetchLimit  = 20   // Limit concurrent Open Graph fetches per user
)

var (
	urlRegex = regexp.MustCompile(`https?://[^\s"']*[^\"'\s\.,!?()[\]{}]`)

	userSemaphoreManager = NewUserSemaphoreManager()
	openGraphGroup       singleflight.Group
	openGraphCache       = cache.New(5*time.Minute, 10*time.Minute) // Cache Open Graph data for 5 minutes, cleanup every 10 minutes
)

type OpenGraphResult struct {
	Title       string
	Description string
	ImageData   []byte // small inline thumbnail (JPEGThumbnail field)
	HQImageData []byte // larger thumbnail uploaded to WA media servers for the big preview card
	HQWidth     uint32
	HQHeight    uint32
}

// UserSemaphoreManager manages per-user semaphore pools for concurrency control.
type UserSemaphoreManager struct {
	pools sync.Map
}

// NewUserSemaphoreManager creates a new UserSemaphoreManager.
func NewUserSemaphoreManager() *UserSemaphoreManager {
	return &UserSemaphoreManager{}
}

// ForUser returns a semaphore pool for the given user ID.
// LoadOrStore provides an atomic way to get or create a semaphore.
func (usm *UserSemaphoreManager) ForUser(userID string) chan struct{} {
	pool, _ := usm.pools.LoadOrStore(userID, make(chan struct{}, openGraphUserFetchLimit))
	return pool.(chan struct{})
}

// IsHTTPURL checks if the input string is a valid HTTP or HTTPS URL.
func IsHTTPURL(input string) bool {
	parsed, err := url.ParseRequestURI(input)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// FetchURLBytes fetches bytes from a URL with a size limit.
func FetchURLBytes(ctx context.Context, resourceURL string, limit int64, httpClient *http.Client) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	if err != nil {
		return nil, "", err
	}

	// Sites with bot protection (e.g. Mercado Livre) return 403 to Go's
	// default "Go-http-client" agent; WhatsApp's own preview fetcher UA is
	// widely allowed since sites want their links previewed in WhatsApp.
	req.Header.Set("User-Agent", "WhatsApp/2.23.20.0")
	// Do not advertise image/avif: CDNs then serve AVIF, which Go's image
	// package cannot decode (gif/png/jpeg/webp decoders are registered).
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

// GetOpenGraphData fetches and caches Open Graph data for a URL.
func GetOpenGraphData(ctx context.Context, urlStr string, userID string, httpClient *http.Client) OpenGraphResult {
	// Check cache first
	if cachedData, found := openGraphCache.Get(urlStr); found {
		if data, ok := cachedData.(OpenGraphResult); ok {
			log.Debug().Str("url", urlStr).Msg("Open Graph data fetched from cache")
			return data
		}
	}

	v, err, _ := openGraphGroup.Do(urlStr, func() (res any, err error) {
		ctx, cancel := context.WithTimeout(ctx, openGraphFetchTimeout)
		defer cancel()

		// Acquire a token from the semaphore pool
		userPool := userSemaphoreManager.ForUser(userID)
		select {
		case userPool <- struct{}{}:
			defer func() { <-userPool }()
		case <-ctx.Done():
			log.Warn().Str("url", urlStr).Msg("Open Graph data fetch timed out while waiting for a worker")
			return nil, ctx.Err()
		}

		// Recover from panics and convert to error
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error().
					Interface("panic_info", r).
					Str("url", urlStr).
					Bytes("stack", stack).
					Msg("Panic recovered while fetching Open Graph data")
				err = fmt.Errorf("panic: %v", r)
			}
		}()

		// Fetch Open Graph data
		result := fetchOpenGraphDataInternal(ctx, urlStr, httpClient)

		// Store in cache
		openGraphCache.Set(urlStr, result, cache.DefaultExpiration)

		return result, nil
	})

	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Error fetching Open Graph data via singleflight")
		return OpenGraphResult{}
	}

	if v == nil {
		return OpenGraphResult{}
	}

	return v.(OpenGraphResult)
}

// ExtractFirstURL extracts the first URL from a text string.
func ExtractFirstURL(text string) string {
	match := urlRegex.FindString(text)
	if match == "" {
		return ""
	}
	return match
}

// FetchOpenGraphData fetches Open Graph metadata from a URL.
func fetchOpenGraphDataInternal(ctx context.Context, urlStr string, httpClient *http.Client) OpenGraphResult {
	pageData, _, err := FetchURLBytes(ctx, urlStr, openGraphPageMaxBytes, httpClient)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to fetch URL for Open Graph data")
		return OpenGraphResult{}
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(pageData))
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse HTML for Open Graph data")
		return OpenGraphResult{}
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
	selectors := []struct {
		selector string
		attr     string
	}{
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

	result := OpenGraphResult{Title: title, Description: description}

	pageURL, err := url.Parse(urlStr)
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to parse page URL for resolving image URL")
		return result
	}

	fetchOpenGraphImage(ctx, pageURL, imageURLStr, &result, httpClient)
	return result
}

func encodeJPEGThumbnail(img image.Image) []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: openGraphJpegQuality}); err != nil {
		log.Warn().Err(err).Msg("Failed to encode thumbnail to JPEG")
		return nil
	}
	return buf.Bytes()
}

func fetchOpenGraphImage(ctx context.Context, pageURL *url.URL, imageURLStr string, result *OpenGraphResult, httpClient *http.Client) {
	// No image found on the page; an empty string would resolve to the page
	// URL itself and we would try to decode HTML as an image.
	if imageURLStr == "" {
		return
	}

	imageURL, err := url.Parse(imageURLStr)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", imageURLStr).Msg("Failed to parse Open Graph image URL")
		return
	}

	resolvedImageURL := pageURL.ResolveReference(imageURL).String()
	imgBytes, _, err := FetchURLBytes(ctx, resolvedImageURL, openGraphImageMaxBytes, httpClient)
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to fetch Open Graph image")
		return
	}

	imgConfig, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image config")
		return
	}

	if imgConfig.Width > openGraphMaxImageDim || imgConfig.Height > openGraphMaxImageDim {
		log.Warn().
			Int("width", imgConfig.Width).
			Int("height", imgConfig.Height).
			Str("imageURL", resolvedImageURL).
			Msg("Open Graph image dimensions too large")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		log.Warn().Err(err).Str("imageURL", resolvedImageURL).Msg("Failed to decode Open Graph image")
		return
	}

	hqThumb := resize.Thumbnail(openGraphHQThumbnailDim, openGraphHQThumbnailDim, img, resize.Lanczos3)
	result.HQImageData = encodeJPEGThumbnail(hqThumb)
	bounds := hqThumb.Bounds()
	result.HQWidth = uint32(bounds.Dx())
	result.HQHeight = uint32(bounds.Dy())

	// Downscale the inline thumbnail from hqThumb (max 600px) instead of
	// resizing the original image (up to 4000px) a second time.
	result.ImageData = encodeJPEGThumbnail(resize.Thumbnail(openGraphThumbnailWidth, openGraphThumbnailHeight, hqThumb, resize.Lanczos3))
}
