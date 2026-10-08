package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		http.Error(w, "sensitive upstream response", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := FetchLoginGovProfile(ctx, "Bearer bad_token", server.URL)
	if err == nil {
		t.Fatalf("expected an error for non-200 status, got nil")
	}
	if strings.Contains(err.Error(), "sensitive upstream response") {
		t.Fatalf("error exposed the upstream response body: %v", err)
	}
}

func TestFetchLoginGovProfileRejectsInvalidResponses(t *testing.T) {
	tests := map[string]struct {
		response string
		want     string
	}{
		"malformed JSON": {
			response: `{`,
			want:     "decode login.gov userinfo",
		},
		"missing subject": {
			response: `{"email":"test@gsa.gov","email_verified":true}`,
			want:     "missing required claims",
		},
		"missing email": {
			response: `{"sub":"subject","email_verified":true}`,
			want:     "missing required claims",
		},
		"unverified email": {
			response: `{"sub":"subject","email":"test@gsa.gov","email_verified":false}`,
			want:     "email is not verified",
		},
		"oversized response": {
			response: `{"sub":"subject","email":"test@gsa.gov","email_verified":true,"given_name":"` + strings.Repeat("a", maxUserInfoResponseBytes) + `"}`,
			want:     "request body too large",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			_, err := FetchLoginGovProfile(context.Background(), "Bearer token", server.URL)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("FetchLoginGovProfile() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestFetchLoginGovProfileHonorsContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := FetchLoginGovProfile(ctx, "Bearer token", server.URL)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("FetchLoginGovProfile() error = %v, want context deadline exceeded", err)
	}
}
