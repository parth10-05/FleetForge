package controlhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type targetCursor struct {
	Version  int    `json:"v"`
	Route    string `json:"r"`
	ID       string `json:"id"`
	PageSize int    `json:"s"`
}

func encodeTargetCursor(key []byte, id string, pageSize int) (string, error) {
	if len(key) < 32 || id == "" || pageSize < 1 || pageSize > 100 {
		return "", fmt.Errorf("invalid cursor")
	}
	payload, err := json.Marshal(targetCursor{Version: 1, Route: "/v1/targets", ID: id, PageSize: pageSize})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func decodeTargetCursor(key []byte, value string, pageSize int) (string, error) {
	if len(key) < 32 || pageSize < 1 || pageSize > 100 {
		return "", fmt.Errorf("invalid cursor")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid cursor")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", fmt.Errorf("invalid cursor")
	}
	var cursor targetCursor
	if json.Unmarshal(payload, &cursor) != nil || cursor.Version != 1 || cursor.Route != "/v1/targets" || cursor.ID == "" || cursor.PageSize != pageSize {
		return "", fmt.Errorf("invalid cursor")
	}
	return cursor.ID, nil
}
