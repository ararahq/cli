package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	oauthDeviceCodePath = "/oauth/device/code"
	// #nosec G101 -- HTTP path constant, not a credential.
	oauthDeviceTokenPath = "/oauth/device/token"
	oauthRefreshPath     = "/oauth/token/refresh"

	defaultOAuthClientID  = "arara-cli"
	defaultOAuthScope     = "messages templates campaigns"
	defaultPollInterval   = 5 * time.Second
	minimumPollInterval   = 1 * time.Second
	defaultDeviceLifetime = 10 * time.Minute

	oauthErrorAuthorizationPending = "authorization_pending"
	oauthErrorSlowDown             = "slow_down"
	oauthErrorAccessDenied         = "access_denied"
	oauthErrorExpiredToken         = "expired_token"
)

var ErrOAuthNotSupported = errors.New("OAuth device flow endpoint not found on this backend (older deploy?). Run 'arara login --key <api-key>' as fallback")

type DeviceCodeRequest struct {
	ClientID string `json:"clientId"`
	Scope    string `json:"scope,omitempty"`
}

type DeviceCodeResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete,omitempty"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type DeviceTokenRequest struct {
	ClientID   string `json:"clientId"`
	DeviceCode string `json:"deviceCode"`
}

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	TokenType    string `json:"tokenType,omitempty"`
	ExpiresIn    int    `json:"expiresIn,omitempty"`
}

type RefreshTokenRequest struct {
	ClientID     string `json:"clientId"`
	RefreshToken string `json:"refreshToken"`
}

func (client *Client) RequestDeviceCode(clientID, scope string) (*DeviceCodeResponse, error) {
	if clientID == "" {
		clientID = defaultOAuthClientID
	}
	if scope == "" {
		scope = defaultOAuthScope
	}

	requestPayload := DeviceCodeRequest{ClientID: clientID, Scope: scope}

	var response DeviceCodeResponse
	if postError := client.Post(oauthDeviceCodePath, requestPayload, &response); postError != nil {
		return nil, wrapOAuthError(postError, "failed to request device code")
	}

	return &response, nil
}

type PollOptions struct {
	ClientID   string
	DeviceCode string
	Interval   time.Duration
	ExpiresIn  time.Duration
	Sleep      func(time.Duration)
	Now        func() time.Time
	OnSlowDown func(newInterval time.Duration)
}

func (client *Client) PollDeviceToken(options PollOptions) (*TokenResponse, error) {
	if options.DeviceCode == "" {
		return nil, errors.New("device code cannot be empty")
	}
	if options.ClientID == "" {
		options.ClientID = defaultOAuthClientID
	}
	if options.Interval < minimumPollInterval {
		options.Interval = defaultPollInterval
	}
	if options.ExpiresIn <= 0 {
		options.ExpiresIn = defaultDeviceLifetime
	}
	if options.Sleep == nil {
		options.Sleep = time.Sleep
	}
	if options.Now == nil {
		options.Now = time.Now
	}

	deadline := options.Now().Add(options.ExpiresIn)
	currentInterval := options.Interval

	for {
		if options.Now().After(deadline) {
			return nil, errors.New("device authorization expired before user approved — please run login again")
		}

		options.Sleep(currentInterval)

		requestPayload := DeviceTokenRequest{ClientID: options.ClientID, DeviceCode: options.DeviceCode}

		var tokenResponse TokenResponse
		pollError := client.Post(oauthDeviceTokenPath, requestPayload, &tokenResponse)
		if pollError == nil {
			return &tokenResponse, nil
		}

		oauthErrorCode := extractOAuthErrorCode(pollError)
		switch oauthErrorCode {
		case oauthErrorAuthorizationPending:
			continue
		case oauthErrorSlowDown:
			currentInterval += minimumPollInterval
			if options.OnSlowDown != nil {
				options.OnSlowDown(currentInterval)
			}
			continue
		case oauthErrorAccessDenied:
			return nil, errors.New("user denied the device authorization request")
		case oauthErrorExpiredToken:
			return nil, errors.New("device code expired before user approved — please run login again")
		default:
			return nil, wrapOAuthError(pollError, "device token polling failed")
		}
	}
}

func (client *Client) RefreshDeviceToken(clientID, refreshToken string) (*TokenResponse, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token cannot be empty")
	}
	if clientID == "" {
		clientID = defaultOAuthClientID
	}

	requestPayload := RefreshTokenRequest{ClientID: clientID, RefreshToken: refreshToken}

	var response TokenResponse
	if postError := client.Post(oauthRefreshPath, requestPayload, &response); postError != nil {
		return nil, wrapOAuthError(postError, "failed to refresh access token")
	}

	return &response, nil
}

func extractOAuthErrorCode(err error) string {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return ""
	}
	return strings.TrimSpace(apiError.Code)
}

func wrapOAuthError(err error, prefix string) error {
	var apiError *APIError
	if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound && apiError.Code == "" {
		// 404 with no structured error code = endpoint missing entirely
		// (Spring's default not-found page). 404 WITH a code is a legitimate
		// domain error like "device_code_not_found" — surface it normally.
		return ErrOAuthNotSupported
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
