package proxy

import "net/http"

// Hop-by-hop headers that must not be forwarded.
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func stripHopByHop(h http.Header) {
	for _, key := range hopByHopHeaders {
		h.Del(key)
	}
}

func prepareUpstreamHeaders(original http.Header, apiKey string) http.Header {
	h := make(http.Header)
	copyHeaders(h, original)
	stripHopByHop(h)

	h.Del("Host")

	// Inject auth if API key provided and no existing auth
	if apiKey != "" && h.Get("Authorization") == "" {
		h.Set("Authorization", "Bearer "+apiKey)
	}

	// Remove accept-encoding to get uncompressed responses for SSE parsing
	h.Del("Accept-Encoding")

	return h
}

func prepareClientHeaders(upstream http.Header) http.Header {
	h := make(http.Header)
	copyHeaders(h, upstream)
	stripHopByHop(h)
	// Remove content-encoding since we stripped accept-encoding upstream
	h.Del("Content-Encoding")
	h.Del("Content-Length") // will be set by http.ResponseWriter
	return h
}
