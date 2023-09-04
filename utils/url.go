package utils

import (
	"net/url"
	"strings"
)

func DropUtmMarkers(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr // return original URL in case of error
	}

	queryParams := u.Query()
	for key := range queryParams {
		if strings.HasPrefix(key, "utm_") {
			delete(queryParams, key)
		}
	}

	u.RawQuery = queryParams.Encode()

	return u.String()
}

