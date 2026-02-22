package proxy

import "net/url"

func buildTargetURL(baseURL, path, rawQuery string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		u = &url.URL{Scheme: "https", Host: "api.anthropic.com"}
	}
	u.Path = path
	u.RawQuery = rawQuery
	return u.String()
}
