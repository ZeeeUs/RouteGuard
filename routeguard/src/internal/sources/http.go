package sources

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"
)

func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:            (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			DisableKeepAlives:      true,
			DisableCompression:     true,
			MaxResponseHeaderBytes: 32 * 1024,
			TLSNextProto:           map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func download(ctx context.Context, client *http.Client, raw string) (DomainSet, int, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return nil, 0, fmt.Errorf("remote lists require a direct HTTPS URL on port 443")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid list URL")
	}

	resp, err := client.Do(request)
	if err != nil {
		var detail *url.Error
		if errors.As(err, &detail) {
			err = detail.Err
		}

		return nil, 0, fmt.Errorf("download from %s: %w", u.Hostname(), err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("download from %s: HTTP %d", u.Hostname(), resp.StatusCode)
	}

	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		return nil, 0, fmt.Errorf("list from %s is an HTML page; use the direct HTTPS link to the raw file", u.Hostname())
	}

	if resp.ContentLength > MaxSourceBytes {
		return nil, 0, fmt.Errorf("domain list exceeds 128 KiB")
	}

	domains, skipped, err := Parse(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("list from %s: %w", u.Hostname(), err)
	}

	return domains, skipped, nil
}
