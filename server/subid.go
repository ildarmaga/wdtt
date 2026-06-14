package main

import (
	"crypto/rand"
)

const subIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

func genSubID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 16)
	for i := range out {
		out[i] = subIDChars[int(b[i])%len(subIDChars)]
	}
	return string(out), nil
}
