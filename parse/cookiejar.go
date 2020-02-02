package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type cookieJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func LoadCookieJarFromJSON(r io.Reader) ([]*http.Cookie, error) {
	var data []*cookieJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	cookies := make([]*http.Cookie, len(data))
	for i, c := range data {
		cookies[i] = &http.Cookie{
			Name:  c.Name,
			Value: c.Value,
		}
	}
	return cookies, nil
}

func LoadCookieJarFromText(r io.Reader) ([]*http.Cookie, error) {
	d, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	var cookies []*http.Cookie
	for _, c := range bytes.Split(d, []byte(";")) {
		c = bytes.TrimSpace(c)
		parts := bytes.SplitN(c, []byte("="), 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid cookie: %q", c)
		}
		cookies = append(cookies, &http.Cookie{
			Name:  string(parts[0]),
			Value: string(parts[1]),
		})
	}
	return cookies, nil
}
