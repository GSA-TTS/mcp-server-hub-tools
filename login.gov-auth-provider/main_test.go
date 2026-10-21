package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
)

func TestNormalizeMultilineSecret(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"escaped newlines": {
			input: `-----BEGIN PRIVATE KEY-----\nkey material\n-----END PRIVATE KEY-----\n`,
			want:  "-----BEGIN PRIVATE KEY-----\nkey material\n-----END PRIVATE KEY-----\n",
		},
		"real newlines": {
			input: "line one\nline two\n",
			want:  "line one\nline two\n",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeMultilineSecret(tt.input); got != tt.want {
				t.Fatalf("normalizeMultilineSecret() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetLoginGovIdentityUsesVerifiedUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q, want bearer session token", got)
		}
		_, _ = w.Write([]byte("{\"sub\":\"login-gov-subject\",\"email\":\"User@GSA.GOV \",\"email_verified\":true}"))
	}))
	defer server.Close()

	ss := state.SerializableState{AccessToken: "session-token", User: "wrong-shared-user", Email: "wrong@example.com"}
	if err := setLoginGovIdentity(t.Context(), &ss, server.URL); err != nil {
		t.Fatalf("setLoginGovIdentity() error = %v", err)
	}
	if ss.User != "login-gov-subject" || ss.PreferredUsername != "login-gov-subject" {
		t.Fatalf("state identity = (%q, %q), want Login.gov subject", ss.User, ss.PreferredUsername)
	}
	if ss.Email != "user@gsa.gov" {
		t.Fatalf("state email = %q, want normalized verified email", ss.Email)
	}
}

func TestSetLoginGovIdentityFailsClosed(t *testing.T) {
	t.Run("missing access token", func(t *testing.T) {
		if err := setLoginGovIdentity(t.Context(), &state.SerializableState{}, "http://unused"); err == nil {
			t.Fatal("setLoginGovIdentity() succeeded without an access token")
		}
	})

	t.Run("unverified userinfo", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{\"sub\":\"subject\",\"email\":\"user@gsa.gov\",\"email_verified\":false}"))
		}))
		defer server.Close()

		err := setLoginGovIdentity(t.Context(), &state.SerializableState{AccessToken: "token"}, server.URL)
		if err == nil {
			t.Fatal("setLoginGovIdentity() succeeded with an unverified email")
		}
	})
}

func TestSerializeLoginGovSessionDoesNotRefresh(t *testing.T) {
	future := time.Now().Add(time.Hour)
	ss, err := serializeLoginGovSession(&sessions.SessionState{
		ExpiresOn:   &future,
		AccessToken: "access-token",
		User:        "subject",
		Email:       "user@gsa.gov",
	})
	if err != nil {
		t.Fatalf("serializeLoginGovSession() error = %v", err)
	}
	if ss.AccessToken != "access-token" || len(ss.SetCookies) != 0 {
		t.Fatalf("serialized state unexpectedly refreshed: token=%q cookies=%d", ss.AccessToken, len(ss.SetCookies))
	}
}

func TestSerializeLoginGovSessionRejectsInvalidSession(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	for name, session := range map[string]*sessions.SessionState{
		"missing": nil,
		"expired": {ExpiresOn: &past},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := serializeLoginGovSession(session); err == nil {
				t.Fatal("serializeLoginGovSession() succeeded for an invalid session")
			}
		})
	}
}
