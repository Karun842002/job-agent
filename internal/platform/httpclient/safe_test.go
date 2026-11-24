package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeResolver struct{ addresses []netip.Addr }

func (r fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addresses, nil
}

func TestPublicAddress(t *testing.T) {
	for _, tc := range []struct {
		address string
		want    bool
	}{
		{"8.8.8.8", true}, {"2606:4700:4700::1111", true},
		{"127.0.0.1", false}, {"10.1.2.3", false}, {"169.254.169.254", false},
		{"100.64.0.1", false}, {"192.0.2.1", false}, {"198.18.0.1", false},
		{"::1", false}, {"::ffff:127.0.0.1", false}, {"fc00::1", false},
		{"fe80::1", false}, {"2001:db8::1", false},
	} {
		if got := publicAddress(netip.MustParseAddr(tc.address)); got != tc.want {
			t.Errorf("%s: got %v", tc.address, got)
		}
	}
}

func TestSafeFetch(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = io.WriteString(w, "User-agent: *\nDisallow: /private\n")
		case "/redirect":
			http.Redirect(w, r, "https://other.example/target", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "https://example.com/loop", http.StatusFound)
		case "/large":
			_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
		case "/slow":
			time.Sleep(150 * time.Millisecond)
		default:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	defer server.Close()
	var calls atomic.Int64
	authorize := func(_ context.Context, host string) error {
		calls.Add(1)
		if host == "other.example" {
			return errors.New("denied")
		}
		return nil
	}
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	client := newClient([]string{"example.com", "other.example"}, authorize, fakeResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}, dial)
	defer client.Close()
	client.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Local test server only.
	status, body, err := client.Get(context.Background(), "https://example.com/")
	if err != nil || status != 200 || string(body) != "ok" {
		t.Fatalf("fetch: %d %q %v", status, body, err)
	}
	if status, _, _, err := client.GetPage(context.Background(), "https://example.com/careers"); err != nil || status != 200 {
		t.Fatalf("robots allowed page: %d %v", status, err)
	}
	if _, _, _, err := client.GetPage(context.Background(), "https://example.com/private"); !errors.Is(err, ErrRobotsDenied) {
		t.Fatalf("robots denial: %v", err)
	}
	before := calls.Load()
	if _, _, err := client.Get(context.Background(), "https://example.com/redirect"); err == nil || calls.Load() != before+2 {
		t.Fatalf("redirect policy was not rechecked: %v", err)
	}
	for _, target := range []string{"http://example.com/", "https://unapproved.example/", "https://127.0.0.1/", "https://user@example.com/"} {
		if _, _, err := client.Get(context.Background(), target); err == nil {
			t.Errorf("accepted %s", target)
		}
	}
	for _, path := range []string{"/loop", "/large"} {
		if _, _, err := client.Get(context.Background(), "https://example.com"+path); err == nil {
			t.Errorf("accepted %s", path)
		}
	}
	client.http.Timeout = 50 * time.Millisecond
	if _, _, err := client.Get(context.Background(), "https://example.com/slow"); err == nil {
		t.Fatal("timeout was not enforced")
	}
}

func TestRejectsMixedPrivateDNS(t *testing.T) {
	var dialed atomic.Bool
	client := newClient([]string{"example.com"}, func(context.Context, string) error { return nil },
		fakeResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")}},
		func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unexpected dial")
		})
	defer client.Close()
	if _, _, err := client.Get(context.Background(), "https://example.com/"); err == nil || dialed.Load() {
		t.Fatal("mixed private DNS answer was not blocked")
	}
}

func TestRedirectChecksTargetRobots(t *testing.T) {
	var targetRequests atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			if r.Host == "other.example" {
				_, _ = io.WriteString(w, "User-agent: *\nDisallow: /private\n")
			} else {
				_, _ = io.WriteString(w, "User-agent: *\nAllow: /\n")
			}
			return
		}
		if r.Host == "example.com" {
			http.Redirect(w, r, "https://other.example/private", http.StatusFound)
			return
		}
		targetRequests.Add(1)
	}))
	defer server.Close()
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	client := newClient([]string{"example.com", "other.example"}, func(context.Context, string) error { return nil },
		fakeResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}, dial)
	defer client.Close()
	client.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Local fixture only.
	if _, _, _, err := client.GetPage(context.Background(), "https://example.com/careers"); !errors.Is(err, ErrRobotsDenied) {
		t.Fatalf("target robots denial: %v", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatal("redirect target page was fetched despite robots denial")
	}
}
