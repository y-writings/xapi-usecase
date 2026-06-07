package xoauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const randomTokenBytes = 32

func GenerateCodeVerifier() (string, error) {
	return generateRandomToken()
}

func GenerateState() (string, error) {
	return generateRandomToken()
}

func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func generateRandomToken() (string, error) {
	b := make([]byte, randomTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
