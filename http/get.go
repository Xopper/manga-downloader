// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"io"
)

// Get is a helper method for obtaining online files via GET call
func Get(params Params) (body io.ReadCloser, err error) {
	return request("GET", params)
}

// GetText is a helper method for obtaining online files as string via GET call
func GetText(params Params) (text string, err error) {
	body, err := Get(params)
	if err != nil {
		return
	}
	defer body.Close()

	// a body cut short (a connection dropped mid-transfer, a timeout) is an
	// error, not a shorter page: a cut-off JSON or HTML reads as broken
	buff := new(bytes.Buffer)
	if _, err = io.Copy(buff, body); err != nil {
		return "", err
	}

	return buff.String(), nil
}
