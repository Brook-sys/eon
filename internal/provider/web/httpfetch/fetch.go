// Package httpfetch implements bounded HTTP(S) source acquisition. Callers
// should additionally apply deployment-level network egress controls.
package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"motor-autonomo/internal/port"
)

const defaultMaxBytes int64 = 1 << 20

type Config struct {
	Client               *http.Client
	MaxBytes             int64
	AllowedMediaTypes    []string
	AllowPrivateNetworks bool
	ResolveIP            func(context.Context, string) ([]net.IP, error)
}

type Fetcher struct {
	client       *http.Client
	maxBytes     int64
	allowed      map[string]struct{}
	allowPrivate bool
	resolveIP    func(context.Context, string) ([]net.IP, error)
}

type ErrorKind string

const (
	ErrorInvalidRequest    ErrorKind = "INVALID_REQUEST"
	ErrorUnsafeDestination ErrorKind = "UNSAFE_DESTINATION"
	ErrorTransport         ErrorKind = "TRANSPORT"
	ErrorHTTP              ErrorKind = "HTTP"
	ErrorTypeRejected      ErrorKind = "TYPE_REJECTED"
	ErrorBodyTooLarge      ErrorKind = "BODY_TOO_LARGE"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
}

func (e *Error) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("web fetch: %s (status %d)", e.Kind, e.StatusCode)
	}
	return "web fetch: " + string(e.Kind)
}

func New(config Config) (*Fetcher, error) {
	limit := config.MaxBytes
	if limit == 0 {
		limit = defaultMaxBytes
	}
	if limit < 1 {
		return nil, errors.New("max bytes must be positive")
	}
	allowed := make(map[string]struct{}, len(config.AllowedMediaTypes))
	for _, value := range config.AllowedMediaTypes {
		mediaType, _, err := mime.ParseMediaType(value)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed media type %q", value)
		}
		allowed[strings.ToLower(mediaType)] = struct{}{}
	}
	if len(allowed) == 0 {
		allowed["text/plain"] = struct{}{}
		allowed["text/html"] = struct{}{}
		allowed["application/json"] = struct{}{}
	}
	resolveIP := config.ResolveIP
	if resolveIP == nil {
		resolveIP = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	base := config.Client
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	client := *base
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if err := validateURL(req.Context(), req.URL, config.AllowPrivateNetworks, resolveIP); err != nil {
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		return nil
	}
	return &Fetcher{client: &client, maxBytes: limit, allowed: allowed, allowPrivate: config.AllowPrivateNetworks, resolveIP: resolveIP}, nil
}

func (f *Fetcher) Fetch(ctx context.Context, request port.FetchRequest) (port.FetchResult, error) {
	target, err := url.Parse(request.URL)
	if err != nil || validateURL(ctx, target, f.allowPrivate, f.resolveIP) != nil {
		return port.FetchResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return port.FetchResult{}, &Error{Kind: ErrorInvalidRequest}
	}
	req.Header.Set("Accept", strings.Join(sortedKeys(f.allowed), ", "))
	resp, err := f.client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "unsafe destination") {
			return port.FetchResult{}, &Error{Kind: ErrorUnsafeDestination}
		}
		return port.FetchResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, f.maxBytes+1))
		return port.FetchResult{}, &Error{Kind: ErrorHTTP, StatusCode: resp.StatusCode, Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500}
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return port.FetchResult{}, &Error{Kind: ErrorTypeRejected}
	}
	mediaType = strings.ToLower(mediaType)
	if _, ok := f.allowed[mediaType]; !ok {
		return port.FetchResult{}, &Error{Kind: ErrorTypeRejected}
	}
	if resp.ContentLength > f.maxBytes {
		return port.FetchResult{}, &Error{Kind: ErrorBodyTooLarge}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return port.FetchResult{}, &Error{Kind: ErrorTransport, Retryable: true}
	}
	if int64(len(content)) > f.maxBytes {
		return port.FetchResult{}, &Error{Kind: ErrorBodyTooLarge}
	}
	return port.FetchResult{FinalURL: resp.Request.URL.String(), MediaType: mediaType, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), Content: content}, nil
}

func validateURL(ctx context.Context, target *url.URL, allowPrivate bool, resolveIP func(context.Context, string) ([]net.IP, error)) error {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return errors.New("invalid HTTP(S) URL")
	}
	if allowPrivate {
		return nil
	}
	host := target.Hostname()
	if strings.EqualFold(host, "localhost") {
		return errors.New("unsafe destination")
	}
	if ip := net.ParseIP(host); ip != nil && !isPublic(ip) {
		return errors.New("unsafe destination")
	}
	lookupContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addresses, err := resolveIP(lookupContext, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("unsafe destination")
	}
	for _, address := range addresses {
		if !isPublic(address) {
			return errors.New("unsafe destination")
		}
	}
	return nil
}

func isPublic(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}
func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j] < result[j-1]; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result
}
