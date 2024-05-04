//go:build !nofido2

package twofactor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/keys-pub/go-libfido2"
	"github.com/quexten/goldwarden/cli/agent/config"
	"github.com/quexten/goldwarden/cli/agent/systemauth/pinentry"
)

const isFido2Enabled = true

type Fido2Response struct {
	Id         string `json:"id"`
	RawId      string `json:"rawId"`
	Type_      string `json:"type"`
	Extensions struct {
		Appid bool `json:"appid"`
	} `json:"extensions"`
	Response struct {
		AuthenticatorData string `json:"authenticatorData"`
		ClientDataJSON    string `json:"clientDataJson"`
		Signature         string `json:"signature"`
	} `json:"response"`
}

func Fido2TwoFactor(challengeB64 string, credentials []string, config *config.Config) (string, error) {
	url, err := url.Parse(config.ConfigFile.ApiUrl)
	if err != nil {
		return "", err
	}
	rpid := url.Host

	locs, err := libfido2.DeviceLocations()
	if err != nil {
		return "", err
	}
	if len(locs) == 0 {
		return "", errors.New("no devices found")
	}

	path := locs[0].Path
	device, err := libfido2.NewDevice(path)
	if err != nil {
		return "", err
	}

	creds := make([][]byte, len(credentials))
	for i, cred := range credentials {
		decodedPublicKey, err := base64.RawURLEncoding.DecodeString(cred)
		if err != nil {
			return "", err
		}
		creds[i] = decodedPublicKey
	}

	clientDataJson := "{\"type\":\"webauthn.get\",\"challenge\":\"" + challengeB64 + "\",\"origin\":\"https://" + rpid + "\",\"crossOrigin\":false}"
	clientDataHash := sha256.Sum256([]byte(clientDataJson))
	clientDataJson = base64.URLEncoding.EncodeToString([]byte(clientDataJson))

	info, err := device.Info()
	if err != nil {
		return "", err
	}
	hasPin := false
	for _, option := range info.Options {
		if option.Name == "clientPin" && option.Value == "true" {
			hasPin = true
		}
	}

	var assertion *libfido2.Assertion
	if hasPin {
		pin, err := pinentry.GetPassword("Fido2 PIN", "Enter your token's PIN")
		if err != nil {
			return "", err
		}

		assertion, err = device.Assertion(
			rpid,
			clientDataHash[:],
			creds,
			pin,
			&libfido2.AssertionOpts{
				Extensions: []libfido2.Extension{},
				UV:         libfido2.False,
			},
		)
		if err != nil {
			return "", err
		}
	} else {
		assertion, err = device.Assertion(
			rpid,
			clientDataHash[:],
			creds,
			"",
			&libfido2.AssertionOpts{
				Extensions: []libfido2.Extension{},
			},
		)
		if err != nil {
			return "", err
		}
	}

	authDataRaw := assertion.AuthDataCBOR[2:] // first 2 bytes seem to be from cbor, don't have a proper decoder ATM but this works
	authData := base64.URLEncoding.EncodeToString(authDataRaw)
	sig := base64.URLEncoding.EncodeToString(assertion.Sig)
	credential := base64.URLEncoding.EncodeToString(assertion.CredentialID)

	resp := Fido2Response{
		Id:    credential,
		RawId: credential,
		Type_: "public-key",
		Extensions: struct {
			Appid bool `json:"appid"`
		}{Appid: false},
		Response: struct {
			AuthenticatorData string `json:"authenticatorData"`
			ClientDataJSON    string `json:"clientDataJson"`
			Signature         string `json:"signature"`
		}{
			AuthenticatorData: authData,
			ClientDataJSON:    clientDataJson,
			Signature:         sig,
		},
	}

	respjson, err := json.Marshal(resp)
	return string(respjson), err
}
