package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// LoginGovProfile represents the claims returned by the login.gov
// userinfo endpoint ({base}/api/openid_connect/userinfo).
//
// For the auth-only (AAL2) service level, the identity is the login.gov
// UUID carried in `sub`, and `email` is used for domain restriction. The
// IAL/AAL/verified_at claims are intentionally NOT surfaced here; enforcing
// identity proofing (IAL2) would require adding them to the /obot-get-state
// payload and writing obot access-control rules keyed on them.
type LoginGovProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
}

// FetchLoginGovProfile queries the login.gov userinfo endpoint using the
// supplied access token. The userInfoURL varies by environment (sandbox vs.
// production) and is provided by the caller.
func FetchLoginGovProfile(ctx context.Context, accessToken, userInfoURL string) (*LoginGovProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d: %s", resp.StatusCode, result)
	}

	var profile LoginGovProfile
	if err = json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}
