package service

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
)

type protectedFetchPolicyError struct{ error }

// IsProtectedFetchPolicyError lets delivery code distinguish a deterministic
// policy rejection from an outbound transport error without logging a URL.
func IsProtectedFetchPolicyError(err error) bool {
	var rejected protectedFetchPolicyError
	return errors.As(err, &rejected)
}

// A local tunnel can return synthetic addresses from the system resolver.
// An operator may select a DNS server that returns addresses the protected
// fetcher can validate and dial. The same policy applies to every answer.
func protectedFetchResolver(server string) (ssrfResolver, error) {
	if server == "" {
		return net.DefaultResolver, nil
	}
	address, err := netip.ParseAddrPort(server)
	if err != nil || address.Port() == 0 {
		return nil, errors.New("PROTECTED_FETCH_DNS_SERVER must be an IP address and port")
	}
	dialer := &net.Dialer{}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address.String())
	}}, nil
}

func (t *ssrfProtectedRoundTripper) validateTarget(ctx context.Context, target *url.URL) error {
	protection, enabled, err := t.getProtection()
	if err != nil {
		return protectedFetchPolicyError{err}
	}
	if !enabled {
		return nil
	}
	port := 80
	switch target.Scheme {
	case "http":
	case "https":
		port = 443
	default:
		return protectedFetchPolicyError{errors.New("unsupported protected fetch protocol")}
	}
	if target.Port() != "" {
		port, err = strconv.Atoi(target.Port())
		if err != nil {
			return protectedFetchPolicyError{errors.New("invalid protected fetch port")}
		}
	}
	host := target.Hostname()
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return protectedFetchPolicyError{err}
	}
	if net.ParseIP(host) != nil || !protection.ApplyIPFilterForDomain {
		return nil
	}
	addresses, err := t.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errors.New("protected fetch DNS returned no addresses")
	}
	for _, address := range addresses {
		if err := protection.ValidateResolvedIP(host, address.IP); err != nil {
			return protectedFetchPolicyError{err}
		}
	}
	return nil
}
