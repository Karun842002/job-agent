// Package httpclient provides bounded, policy-gated HTTPS reads.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/temoto/robotstxt"
)

const maxResponseBytes = 8 << 20

var specialNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func publicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range specialNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type safeTransport struct {
	base      *http.Transport
	hosts     map[string]bool
	authorize func(context.Context, string) error
}

func (t *safeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	host, err := validateURL(request.URL, t.hosts)
	if err != nil {
		return nil, err
	}
	if err := t.authorize(request.Context(), host); err != nil {
		return nil, fmt.Errorf("source permission denied: %w", err)
	}
	return t.base.RoundTrip(request)
}

func validateURL(value *url.URL, hosts map[string]bool) (string, error) {
	if value == nil || value.Scheme != "https" || value.User != nil || value.Hostname() == "" {
		return "", errors.New("URL must use HTTPS with an approved hostname")
	}
	host := strings.ToLower(strings.TrimSuffix(value.Hostname(), "."))
	if _, err := netip.ParseAddr(host); err == nil || !hosts[host] {
		return "", errors.New("hostname is not approved")
	}
	if port := value.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("invalid port")
		}
	}
	return host, nil
}

func newClient(hosts []string, authorize func(context.Context, string) error, lookup resolver, dial func(context.Context, string, string) (net.Conn, error)) *Client {
	allowed := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allowed[strings.ToLower(strings.TrimSuffix(host, "."))] = true
	}
	transport := &http.Transport{
		Proxy:             nil,
		ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			host = strings.ToLower(strings.TrimSuffix(host, "."))
			if !allowed[host] {
				return nil, errors.New("hostname is not approved")
			}
			addresses, err := lookup.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("hostname could not be resolved")
			}
			for _, addr := range addresses {
				if !publicAddress(addr) {
					return nil, errors.New("hostname resolved to a non-public address")
				}
			}
			var lastErr error
			for _, addr := range addresses {
				conn, err := dial(ctx, network, net.JoinHostPort(addr.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
	wrapped := &safeTransport{base: transport, hosts: allowed, authorize: authorize}
	return &Client{http: &http.Client{
		Transport: wrapped,
		Timeout:   10 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("redirect limit exceeded")
			}
			return nil
		},
	}, transport: transport}
}

// Client performs HTTPS GET requests with hostname, policy and IP checks.
type Client struct {
	http      *http.Client
	transport *http.Transport
}

// New creates a client restricted to exact approved hostnames. The authorizer
// is called for every request, including each redirect.
func New(hosts []string, authorize func(context.Context, string) error) (*Client, error) {
	if len(hosts) == 0 || authorize == nil {
		return nil, errors.New("approved hosts and authorizer are required")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return newClient(hosts, authorize, net.DefaultResolver, dialer.DialContext), nil
}

// Get fetches one bounded response, rechecking restrictions on redirects.
func (c *Client) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, errors.New("invalid URL")
	}
	request.Header.Set("User-Agent", "job-agent")
	response, err := c.http.Do(request)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			return 0, nil, fmt.Errorf("fetch failed: %w", requestError.Err)
		}
		return 0, nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return 0, nil, errors.New("response exceeds size limit")
	}
	return response.StatusCode, data, nil
}

// Close releases idle connections.
func (c *Client) Close() { c.transport.CloseIdleConnections() }

// ErrRobotsDenied indicates a robots policy prevents fetching a page.
var ErrRobotsDenied = errors.New("robots policy denies the page")

// GetPage checks robots.txt for every redirect target before fetching the page.
func (c *Client) GetPage(ctx context.Context, rawURL string) (int, []byte, string, error) {
	current, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, "", errors.New("invalid URL")
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for redirects := 0; redirects < 4; redirects++ {
		robotsURL := &url.URL{Scheme: current.Scheme, Host: current.Host, Path: "/robots.txt"}
		status, data, err := c.Get(ctx, robotsURL.String())
		if err != nil {
			return 0, nil, "", err
		}
		switch {
		case status == http.StatusNotFound || status == http.StatusGone:
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return 0, nil, "", ErrRobotsDenied
		case status >= 200 && status < 300:
			robots, err := robotstxt.FromBytes(data)
			if err != nil {
				return 0, nil, "", errors.New("invalid robots policy")
			}
			path := current.EscapedPath()
			if path == "" {
				path = "/"
			}
			if current.RawQuery != "" {
				path += "?" + current.RawQuery
			}
			if !robots.TestAgent(path, "job-agent") {
				return 0, nil, "", ErrRobotsDenied
			}
		default:
			return 0, nil, "", errors.New("robots policy unavailable")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return 0, nil, "", errors.New("invalid URL")
		}
		request.Header.Set("User-Agent", "job-agent")
		response, err := client.Do(request)
		if err != nil {
			var requestError *url.Error
			if errors.As(err, &requestError) {
				return 0, nil, "", fmt.Errorf("fetch failed: %w", requestError.Err)
			}
			return 0, nil, "", fmt.Errorf("fetch failed: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		response.Body.Close()
		if readErr != nil {
			return 0, nil, "", fmt.Errorf("read response: %w", readErr)
		}
		if len(body) > maxResponseBytes {
			return 0, nil, "", errors.New("response exceeds size limit")
		}
		if response.StatusCode < 300 || response.StatusCode >= 400 {
			return response.StatusCode, body, current.String(), nil
		}
		location := response.Header.Get("Location")
		next, err := current.Parse(location)
		if err != nil || location == "" {
			return 0, nil, "", errors.New("invalid redirect")
		}
		current = next
	}
	return 0, nil, "", errors.New("redirect limit exceeded")
}
