package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/obot-platform/tools/auth-providers-common/pkg/env"
	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
	"github.com/obot-platform/tools/login.gov-auth-provider/pkg/profile"
)

const (
	// login.gov OIDC endpoint bases. logingov.go in the oauth2-proxy fork
	// hardcodes the production base via setProviderDefaults (which only fills
	// values when they are empty), so we set the endpoints explicitly below to
	// support the sandbox environment.
	sandboxBase    = "https://idp.int.identitysandbox.gov"
	productionBase = "https://secure.login.gov"

	// Relative OIDC paths, per login.gov documentation.
	authorizePath = "/openid_connect/authorize"
	tokenPath     = "/api/openid_connect/token"
	userInfoPath  = "/api/openid_connect/userinfo"
)

type Options struct {
	// ClientID is the login.gov issuer / service-provider (SP) string
	// registered in the Partner Portal. login.gov uses private_key_jwt client
	// authentication, so there is no client secret.
	ClientID string `env:"OBOT_LOGINGOV_AUTH_PROVIDER_CLIENT_ID"`
	// JWTKey is the RSA private key (PEM, 2048-bit+) used to sign the
	// client_assertion JWT. The corresponding public key is registered with
	// login.gov and published at PubJWKURL.
	JWTKey string `env:"OBOT_LOGINGOV_AUTH_PROVIDER_JWT_KEY"`
	// PubJWKURL is the publicly reachable JWKS URL exposing the public key.
	PubJWKURL string `env:"OBOT_LOGINGOV_AUTH_PROVIDER_PUBJWK_URL"`
	// AcrValues selects the login.gov service/assurance level. Defaults to
	// auth-only (AAL2, no identity proofing).
	AcrValues string `usage:"login.gov acr_values (service level)" optional:"true" default:"urn:acr.login.gov:auth-only" env:"OBOT_LOGINGOV_AUTH_PROVIDER_ACR_VALUES"`
	// Environment selects the login.gov endpoint base: "sandbox" or "production".
	Environment string `usage:"login.gov environment: sandbox or production" optional:"true" default:"sandbox" env:"OBOT_LOGINGOV_AUTH_PROVIDER_ENVIRONMENT"`

	ObotServerURL            string `env:"OBOT_SERVER_PUBLIC_URL,OBOT_SERVER_URL"`
	PostgresConnectionDSN    string `env:"OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN" optional:"true"`
	AuthCookieSecret         string `usage:"Secret used to encrypt cookie" env:"OBOT_AUTH_PROVIDER_COOKIE_SECRET"`
	AuthEmailDomains         string `usage:"Email domains allowed for authentication" default:"*" env:"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"`
	AuthTokenRefreshDuration string `usage:"Duration to refresh auth token after" optional:"true" default:"1h" env:"OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION"`
	LoggingEnabled           string `usage:"Enable oauth2-proxy logging" optional:"true" env:"OBOT_AUTH_PROVIDER_ENABLE_LOGGING"`
}

func main() {
	var opts Options
	if err := env.LoadEnvForStruct(&opts); err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to load options: %v\n", err)
		os.Exit(1)
	}
	opts.JWTKey = normalizeMultilineSecret(opts.JWTKey)

	base := productionBase
	switch strings.ToLower(strings.TrimSpace(opts.Environment)) {
	case "", "sandbox":
		base = sandboxBase
	case "production", "prod":
		base = productionBase
	default:
		fmt.Printf("ERROR: login.gov-auth-provider: invalid environment %q (expected \"sandbox\" or \"production\")\n", opts.Environment)
		os.Exit(1)
	}

	refreshDuration, err := time.ParseDuration(opts.AuthTokenRefreshDuration)
	if err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to parse token refresh duration: %v\n", err)
		os.Exit(1)
	}

	if refreshDuration < 0 {
		fmt.Printf("ERROR: login.gov-auth-provider: token refresh duration must be greater than 0\n")
		os.Exit(1)
	}

	cookieSecret, err := base64.StdEncoding.DecodeString(opts.AuthCookieSecret)
	if err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to decode cookie secret: %v\n", err)
		os.Exit(1)
	}

	legacyOpts := options.NewLegacyOptions()
	legacyOpts.LegacyProvider.ProviderType = "login.gov"
	legacyOpts.LegacyProvider.ProviderName = "login.gov"
	legacyOpts.LegacyProvider.ClientID = opts.ClientID
	legacyOpts.LegacyProvider.Scope = "openid email"

	// login.gov private_key_jwt configuration.
	legacyOpts.LegacyProvider.JWTKey = opts.JWTKey
	legacyOpts.LegacyProvider.PubJWKURL = opts.PubJWKURL
	legacyOpts.LegacyProvider.AcrValues = opts.AcrValues

	// Explicitly set the OIDC endpoints for the selected environment.
	// logingov.go only fills these when empty, and its defaults point at
	// production, so the sandbox base must be set here.
	legacyOpts.LegacyProvider.LoginURL = base + authorizePath
	legacyOpts.LegacyProvider.RedeemURL = base + tokenPath
	legacyOpts.LegacyProvider.ProfileURL = base + userInfoPath
	legacyOpts.LegacyProvider.ValidateURL = base + userInfoPath

	oauthProxyOpts, err := legacyOpts.ToOptions()
	if err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to convert legacy options to new options: %v\n", err)
		os.Exit(1)
	}

	oauthProxyOpts.Server.BindAddress = ""
	oauthProxyOpts.MetricsServer.BindAddress = ""
	if opts.PostgresConnectionDSN != "" {
		oauthProxyOpts.Session.Type = options.PostgresSessionStoreType
		oauthProxyOpts.Session.Postgres.ConnectionDSN = opts.PostgresConnectionDSN
		oauthProxyOpts.Session.Postgres.TableNamePrefix = "logingov_"
	}
	oauthProxyOpts.Cookie.Refresh = refreshDuration
	oauthProxyOpts.Cookie.Name = "obot_access_token"
	oauthProxyOpts.Cookie.Secret = string(bytes.TrimSpace(cookieSecret))
	oauthProxyOpts.Cookie.Secure = strings.HasPrefix(opts.ObotServerURL, "https://")
	oauthProxyOpts.Cookie.CSRFExpire = 30 * time.Minute
	oauthProxyOpts.Templates.Path = os.Getenv("GPTSCRIPT_TOOL_DIR") + "/../auth-providers-common/templates"
	oauthProxyOpts.RawRedirectURL = opts.ObotServerURL + "/"
	if opts.AuthEmailDomains != "" {
		emailDomains := strings.Split(opts.AuthEmailDomains, ",")
		for i := range emailDomains {
			emailDomains[i] = strings.TrimSpace(emailDomains[i])
		}
		oauthProxyOpts.EmailDomains = emailDomains
	}

	loggingEnabled := strings.EqualFold(opts.LoggingEnabled, "true")
	oauthProxyOpts.Logging.RequestEnabled = loggingEnabled
	oauthProxyOpts.Logging.AuthEnabled = loggingEnabled
	oauthProxyOpts.Logging.StandardEnabled = loggingEnabled

	if err = validation.Validate(oauthProxyOpts); err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to validate options: %v\n", err)
		os.Exit(1)
	}

	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to create oauth2 proxy: %v\n", err)
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	userInfoURL := base + userInfoPath

	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("http://127.0.0.1:%s", port)))
	})
	mux.HandleFunc("/obot-get-state", loginGovState(oauthProxy, userInfoURL))
	mux.HandleFunc("/obot-get-user-info", func(w http.ResponseWriter, r *http.Request) {
		userInfo, err := profile.FetchLoginGovProfile(r.Context(), r.Header.Get("Authorization"), userInfoURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch user info: %v", err), http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(userInfo)
	})
	mux.HandleFunc("/obot-list-user-auth-groups", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/", oauthProxy.ServeHTTP)

	fmt.Printf("listening on 127.0.0.1:%s\n", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("ERROR: login.gov-auth-provider: failed to listen and serve: %v\n", err)
		os.Exit(1)
	}
}

func loginGovState(p *oauth2proxy.OAuthProxy, userInfoURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sr state.SerializableRequest
		if err := json.NewDecoder(r.Body).Decode(&sr); err != nil {
			http.Error(w, "failed to decode request", http.StatusBadRequest)
			return
		}

		reqObj, err := http.NewRequest(sr.Method, sr.URL, nil)
		if err != nil {
			http.Error(w, "failed to create request", http.StatusBadRequest)
			return
		}
		reqObj.Header = sr.Header

		ss, err := state.GetSerializableState(p, reqObj)
		if err != nil {
			http.Error(w, "failed to get authentication state", http.StatusUnauthorized)
			return
		}
		if err = setLoginGovIdentity(r.Context(), &ss, userInfoURL); err != nil {
			http.Error(w, "failed to validate Login.gov identity", http.StatusUnauthorized)
			return
		}

		if err = json.NewEncoder(w).Encode(ss); err != nil {
			http.Error(w, "failed to encode authentication state", http.StatusInternalServerError)
		}
	}
}

func setLoginGovIdentity(ctx context.Context, ss *state.SerializableState, userInfoURL string) error {
	if ss.AccessToken == "" {
		return errors.New("authentication state has no access token")
	}

	userInfo, err := profile.FetchLoginGovProfile(ctx, "Bearer "+ss.AccessToken, userInfoURL)
	if err != nil {
		return err
	}

	ss.User = userInfo.Subject
	ss.PreferredUsername = userInfo.Subject
	ss.Email = strings.ToLower(strings.TrimSpace(userInfo.Email))
	return nil
}

func normalizeMultilineSecret(value string) string {
	return strings.ReplaceAll(value, `\n`, "\n")
}
