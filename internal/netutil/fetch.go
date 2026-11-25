// Package netutil implements the SSRF-hardened public-web fetch used by the fetch_page tool.
package netutil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxBytes     = 512 * 1024
	maxHops      = 5
	totalTimeout = 15 * time.Second
)

var blockedV4 = []*net.IPNet{
	cidr4("0.0.0.0", 8), cidr4("10.0.0.0", 8), cidr4("100.64.0.0", 10), cidr4("127.0.0.0", 8),
	cidr4("169.254.0.0", 16), cidr4("172.16.0.0", 12), cidr4("192.0.0.0", 24), cidr4("192.168.0.0", 16),
	cidr4("198.18.0.0", 15), cidr4("224.0.0.0", 3),
}

var blockedV6 = []*net.IPNet{
	cidr6("::1", 128), cidr6("64:ff9b::", 96), cidr6("fc00::", 7), cidr6("fe80::", 10), cidr6("ff00::", 8),
}

func cidr4(s string, bits int) *net.IPNet {
	_, n, err := net.ParseCIDR(s + "/" + itoa(bits))
	if err != nil {
		panic(err)
	}
	return n
}

func cidr6(s string, bits int) *net.IPNet {
	_, n, err := net.ParseCIDR(s + "/" + itoa(bits))
	if err != nil {
		panic(err)
	}
	return n
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func public(ip net.IP) bool {
	v4 := ip.To4()
	if v4 != nil {
		for _, n := range blockedV4 {
			if n.Contains(v4) {
				return false
			}
		}
		return true
	}
	if ip.To16() == nil || ip.IsUnspecified() {
		return false
	}
	for _, n := range blockedV6 {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// FetchPublicPage resolves and fetches a public HTTP(S) page, pinning the connection to the checked IP.
func FetchPublicPage(ctx context.Context, rawURL string, abort <-chan struct{}) (status int, body string, err error) {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	select {
	case <-abort:
		return 0, "", context.Canceled
	default:
	}

	for hop := 0; hop < maxHops; hop++ {
		u, parseErr := url.Parse(rawURL)
		if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return 0, "", errors.New("只支持 http 或 https 地址")
		}
		host := u.Hostname()
		if host == "" {
			return 0, "", errors.New("地址缺少主机名")
		}
		addresses, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
		if lookupErr != nil || len(addresses) == 0 {
			return 0, "", errors.New("无法解析主机")
		}
		pinned := addresses[0].IP
		for _, candidate := range addresses {
			if !public(candidate.IP) {
				return 0, "", errors.New("不允许访问内网地址")
			}
		}
		if pinned.To4() == nil && len(addresses) > 1 {
			for _, candidate := range addresses {
				if candidate.IP.To4() != nil {
					pinned = candidate.IP
					break
				}
			}
		}
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		transport := &http.Transport{
			DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(dialCtx, "tcp", net.JoinHostPort(pinned.String(), port))
			},
			TLSHandshakeTimeout: 5 * time.Second,
			DisableKeepAlives:   true,
		}
		client := &http.Client{Transport: transport, Timeout: totalTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if reqErr != nil {
			return 0, "", reqErr
		}
		req.Header.Set("User-Agent", "pi-durable-assistant/1.0 (+fetch_page)")
		resp, doErr := client.Do(req)
		if doErr != nil {
			return 0, "", doErr
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return resp.StatusCode, "", nil
			}
			next, resolveErr := u.Parse(location)
			if resolveErr != nil {
				return 0, "", resolveErr
			}
			rawURL = next.String()
			continue
		}
		reader := io.LimitReader(resp.Body, maxBytes+1)
		data, readErr := io.ReadAll(reader)
		resp.Body.Close()
		if readErr != nil {
			return 0, "", readErr
		}
		text := string(data)
		if len(data) > maxBytes {
			text = text[:maxBytes]
		}
		if strings.TrimSpace(text) == "" {
			return resp.StatusCode, "", nil
		}
		return resp.StatusCode, text, nil
	}
	return 0, "", errors.New("重定向次数过多")
}
