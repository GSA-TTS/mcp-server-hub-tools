package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLoginGovProfile(t *testing.T) {
	// Arrange: mock response struct
	mockProfile := LoginGovProfile{
		Subject:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Email:         "test@gsa.gov",
		EmailVerified: true,
		GivenName:     "Test",
		FamilyName:    "User",
	}

	// Arrange: mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedAuth := "Bearer mock_logingov_token"
		if got := r.Header.Get("Authorization"); got != expectedAuth {
			http.Error(w, fmt.Sprintf("unexpected auth header: got %s", got), http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockProfile)
	}))
	defer server.Close()

	// Act: call the function with the mock server URL
	ctx := context.Background()
	profile, err := FetchLoginGovProfile(ctx, "Bearer mock_logingov_token", server.URL)

	// Assert: no error
	if err != nil {
		t.Fatalf("FetchLoginGovProfile returned error: %v", err)
	}

	// Assert: check fields
	if profile.Subject != mockProfile.Subject {
		t.Errorf("unexpected Subject: got %s, want %s", profile.Subject, mockProfile.Subject)
	}
	if profile.Email != mockProfile.Email {
		t.Errorf("unexpected Email: got %s, want %s", profile.Email, mockProfile.Email)
	}
	if profile.EmailVerified != mockProfile.EmailVerified {
		t.Errorf("unexpected EmailVerified: got %v, want %v", profile.EmailVerified, mockProfile.EmailVerified)
	}
	if profile.GivenName != mockProfile.GivenName {
		t.Errorf("unexpected GivenName: got %s, want %s", profile.GivenName, mockProfile.GivenName)
	}
	if profile.FamilyName != mockProfile.FamilyName {
		t.Errorf("unexpected FamilyName: got %s, want %s", profile.FamilyName, mockProfile.FamilyName)
	}
}

func TestFetchLoginGovProfileErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := context.Background()
	if _, err := FetchLoginGovProfile(ctx, "Bearer bad_token", server.URL); err == nil {
		t.Fatalf("expected an error for non-200 status, got nil")
	}
}
