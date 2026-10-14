package api

import "time"

// Sign-in follows the device authorization flow (RFC 8628, backend plan D1).
// The app starts it, shows UserCode, and opens VerificationURLComplete; the
// person signs in there with Google or GitHub. Meanwhile the app polls
// RoutePollDeviceSignIn every IntervalSeconds until it gets a Token or an
// error other than CodeAuthorizationPending and CodeSlowDown.
//
// Every other /v1 route needs the token in an Authorization header:
// "Bearer <token>". Without a valid one it answers CodeUnauthorized.
const (
	RouteStartDeviceSignIn = "POST /v1/auth/device"
	RoutePollDeviceSignIn  = "POST /v1/auth/token"
	RouteSession           = "GET /v1/auth/session"
	RouteSignOut           = "POST /v1/auth/sign-out"
)

// DeviceSignIn answers RouteStartDeviceSignIn.
type DeviceSignIn struct {
	DeviceCode              string    `json:"deviceCode"`
	UserCode                string    `json:"userCode"`
	VerificationURL         string    `json:"verificationUrl"`
	VerificationURLComplete string    `json:"verificationUrlComplete"`
	ExpiresAt               time.Time `json:"expiresAt"`
	IntervalSeconds         int       `json:"intervalSeconds"`
}

// PollDeviceSignInRequest is the body of RoutePollDeviceSignIn.
type PollDeviceSignInRequest struct {
	DeviceCode string `json:"deviceCode"`
}

// Token answers RoutePollDeviceSignIn once the person has signed in. The
// token is shown once; the server keeps only its hash.
type Token struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Email     string    `json:"email"`
}

// Session answers RouteSession with the signed-in account. RouteSignOut
// revokes the token it is called with and answers with no body.
type Session struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Error codes for RoutePollDeviceSignIn, named as in RFC 8628.
const (
	// CodeAuthorizationPending: the person has not finished signing in.
	CodeAuthorizationPending = "authorization_pending"
	// CodeSlowDown: the app polled sooner than IntervalSeconds.
	CodeSlowDown = "slow_down"
	// CodeAccessDenied: the account may not use this server.
	CodeAccessDenied = "access_denied"
	// CodeExpiredToken: the device code expired or is unknown; start again.
	CodeExpiredToken = "expired_token"
)
