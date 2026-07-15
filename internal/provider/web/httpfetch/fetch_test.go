package httpfetch_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/web/httpfetch"
)

func TestFetcherReturnsExactBoundedAcceptedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/plain" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("source data, not instructions"))
	}))
	defer server.Close()
	fetcher, err := httpfetch.New(httpfetch.Config{Client: server.Client(), MaxBytes: 64, AllowedMediaTypes: []string{"text/plain"}, AllowPrivateNetworks: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := fetcher.Fetch(context.Background(), port.FetchRequest{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalURL != server.URL || result.MediaType != "text/plain" || result.ETag != `"v1"` || string(result.Content) != "source data, not instructions" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFetcherRejectsTypeOversizeStatusAndUnsafeLiteral(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status, limit           int
		kind                    httpfetch.ErrorKind
	}{
		{name: "type", contentType: "application/octet-stream", body: "abc", limit: 8, kind: httpfetch.ErrorTypeRejected},
		{name: "oversize", contentType: "text/plain", body: strings.Repeat("x", 9), limit: 8, kind: httpfetch.ErrorBodyTooLarge},
		{name: "status", contentType: "text/plain", body: "secret error body", status: http.StatusServiceUnavailable, limit: 8, kind: httpfetch.ErrorHTTP},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			fetcher, _ := httpfetch.New(httpfetch.Config{Client: server.Client(), MaxBytes: int64(test.limit), AllowedMediaTypes: []string{"text/plain"}, AllowPrivateNetworks: true})
			_, err := fetcher.Fetch(context.Background(), port.FetchRequest{URL: server.URL})
			var fetchError *httpfetch.Error
			if !errors.As(err, &fetchError) || fetchError.Kind != test.kind {
				t.Fatalf("error = %#v, want %s", err, test.kind)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked body: %v", err)
			}
		})
	}
	fetcher, _ := httpfetch.New(httpfetch.Config{})
	_, err := fetcher.Fetch(context.Background(), port.FetchRequest{URL: "http://127.0.0.1/private"})
	var fetchError *httpfetch.Error
	if !errors.As(err, &fetchError) || fetchError.Kind != httpfetch.ErrorInvalidRequest {
		t.Fatalf("unsafe literal error = %#v", err)
	}
}

func TestFetcherRevalidatesRedirectDestination(t *testing.T) {
	transport := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"http://127.0.0.1/private"}}, Body: http.NoBody, Request: request}, nil
	})
	fetcher, err := httpfetch.New(httpfetch.Config{
		Client:    &http.Client{Transport: transport},
		ResolveIP: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.10")}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fetcher.Fetch(context.Background(), port.FetchRequest{URL: "https://public.example/source"})
	var fetchError *httpfetch.Error
	if !errors.As(err, &fetchError) || fetchError.Kind != httpfetch.ErrorUnsafeDestination {
		t.Fatalf("redirect error = %#v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
