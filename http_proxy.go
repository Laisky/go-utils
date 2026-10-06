package utils

import "net/http"

// WithHTTPClientProxyFromEnvironment uses the standard HTTP_PROXY, HTTPS_PROXY,
// and NO_PROXY policy, including uppercase precedence, loopback bypass, and CGI
// refusal of unsafe HTTP_PROXY. Go caches environment discovery on first use;
// configure process variables before issuing requests. Explicit proxy options
// remain separate, and the last proxy option supplied wins.
func WithHTTPClientProxyFromEnvironment() HTTPClientOptFunc {
	return func(opt *httpClientOption) error {
		opt.proxy = http.ProxyFromEnvironment
		return nil
	}
}
