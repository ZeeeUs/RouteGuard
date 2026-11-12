package sources

import (
	"context"
	_ "embed"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ZeeeUs/RouteGuard/internal/sources/mock"
)

//go:embed testdata/domains.txt
var sourceFixture string

func TestParseSource(t *testing.T) {
	domains, skipped, err := Parse(strings.NewReader(sourceFixture))
	if err != nil || skipped != 3 || len(domains) != 2 {
		t.Fatalf("Parse = %v, %d, %v", domains, skipped, err)
	}

	for _, name := range []string{"example.org", "api.example.org"} {
		if _, exists := domains[name]; !exists {
			t.Errorf("missing domain %s", name)
		}
	}

	for _, bad := range []string{"", "<html>error</html>", "include:child", "foo.example.org trailing",
		"*.example.org", "127.0.0.1", "unknown:foo.example.org",
		strings.Repeat("# comment padding\n", MaxSourceBytes/10)} {
		if _, _, err := Parse(strings.NewReader(bad)); err == nil {
			t.Errorf("accepted invalid list (%d bytes)", len(bad))
		}
	}
}

func TestDownloadRejectsRedirect(t *testing.T) {
	client := newHTTPClient()
	transport := mock.NewMockRoundTripper(gomock.NewController(t))
	client.Transport = transport
	transport.EXPECT().RoundTrip(gomock.Cond(func(req *http.Request) bool {
		return req.Method == http.MethodGet && req.URL.String() == "https://example.org/list"
	})).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 302,
			Header:     http.Header{"Location": {"http://other.example/list"}},
			Body:       io.NopCloser(strings.NewReader("example.org")),
			Request:    req,
		}, nil
	})
	if _, _, err := download(context.Background(), client, "https://example.org/list"); err == nil {
		t.Fatal("accepted redirect")
	}
}

func TestDownloadRejectsHTMLPage(t *testing.T) {
	client := newHTTPClient()
	transport := mock.NewMockRoundTripper(gomock.NewController(t))
	client.Transport = transport
	transport.EXPECT().RoundTrip(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(sourceFixture)),
	}, nil)

	_, _, err := download(context.Background(), client, "https://example.org/list")
	if err == nil || !strings.Contains(err.Error(), "use the direct HTTPS link to the raw file") {
		t.Fatalf("expected an actionable HTML error, got %v", err)
	}
}

func TestDNSAddresses(t *testing.T) {
	v4 := netip.MustParseAddr("160.79.104.10")
	got, err := resolvedPrefixes("example.org", []netip.Addr{
		v4, netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("224.0.0.1"),
		netip.MustParseAddr("100.64.1.1"),
	}, "ip4")
	if err != nil || !slices.Equal(got, []netip.Prefix{netip.PrefixFrom(v4, 32)}) {
		t.Fatalf("resolved = %v, %v", got, err)
	}

	if _, err := resolvedPrefixes("example.org", []netip.Addr{
		v4, netip.MustParseAddr("198.18.1.1"),
	}, "ip4"); err == nil {
		t.Fatal("accepted mixed DNS response with a synthetic address")
	}
}
