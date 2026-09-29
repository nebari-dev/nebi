package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// authorizationCacheTTL bounds how long a verified token is served from the
// cache before its claims are applied again, so group and admin changes in
// the identity provider land within this window even for long-lived tokens.
const authorizationCacheTTL = 5 * time.Minute

// maxCachedTokens caps the verified-token cache.
const maxCachedTokens = 10000

var (
	errMissingAuthorization = errors.New("missing authorization")
	errInvalidToken         = errors.New("invalid or expired token")
	errProviderUnavailable  = errors.New("identity provider unavailable")
)

// OIDCConfig configures the OIDC resource server.
type OIDCConfig struct {
	IssuerURL    string
	DiscoveryURL string   // optional; see NewOIDCAuthenticator
	ClientID     string   // required "aud" of accepted tokens
	AdminGroups  []string // IdP groups whose members are nebi admins
}

// OIDCAuthenticator accepts access tokens issued by the configured OIDC
// provider. It never issues tokens itself: clients log in with the provider
// (authorization code + PKCE in the browser, device flow in the CLI and
// desktop app) and send the provider's access token as a bearer token.
//
// The first time a token is seen, its (iss, sub) is resolved to a local user
// (provisioned on first sight) and the user's groups and admin role are
// reconciled with its claims. Failed reconciliation rejects the request.
type OIDCAuthenticator struct {
	cfg      OIDCConfig
	db       *gorm.DB
	rbac     rbac.Provider
	verifier atomic.Pointer[oidc.IDTokenVerifier]

	cache  tokenCache
	flight singleflight.Group
	now    func() time.Time
}

// NewOIDCAuthenticator creates an authenticator. It verifies no token until
// Discover succeeds.
func NewOIDCAuthenticator(cfg OIDCConfig, db *gorm.DB, rbacProvider rbac.Provider) *OIDCAuthenticator {
	return &OIDCAuthenticator{
		cfg:   cfg,
		db:    db,
		rbac:  rbacProvider,
		cache: tokenCache{entries: map[[sha256.Size]byte]cachedToken{}},
		now:   time.Now,
	}
}

// accessTokenClaims are the claims nebi reads from an access token.
type accessTokenClaims struct {
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	Name              string   `json:"name"`
	Picture           string   `json:"picture"`
	Groups            []string `json:"groups"`
}

// Discover fetches the provider configuration and signing keys.
//
// In split-horizon deployments (e.g. Keycloak behind an external gateway
// while nebi reaches it via an internal hostname) the URL nebi uses to fetch
// .well-known/openid-configuration differs from the issuer that appears in
// the token's "iss" claim. When DiscoveryURL is set, discovery is fetched
// from it but "iss" is still validated against IssuerURL.
func (a *OIDCAuthenticator) Discover(ctx context.Context) error {
	discoveryURL := a.cfg.DiscoveryURL
	if discoveryURL != "" && discoveryURL != a.cfg.IssuerURL {
		ctx = oidc.InsecureIssuerURLContext(ctx, a.cfg.IssuerURL)
	} else {
		discoveryURL = a.cfg.IssuerURL
	}

	provider, err := oidc.NewProvider(ctx, discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to discover OIDC provider: %w", err)
	}
	// go-oidc keeps ctx for later key-set refreshes, so callers pass a
	// context that lives as long as the server.
	a.verifier.Store(provider.Verifier(&oidc.Config{ClientID: a.cfg.ClientID}))
	return nil
}

// DiscoverInBackground retries Discover until it succeeds or ctx is done.
// The provider (e.g. Keycloak) may not be ready when nebi starts; requests
// get 503 until discovery succeeds.
func (a *OIDCAuthenticator) DiscoverInBackground(ctx context.Context, logger *slog.Logger) {
	if err := a.Discover(ctx); err == nil {
		logger.Info("OIDC authentication enabled", "issuer", a.cfg.IssuerURL)
		return
	} else {
		logger.Error("OIDC discovery failed, will retry in background", "issuer", a.cfg.IssuerURL, "error", err)
	}

	go func() {
		backoff := 2 * time.Second
		const maxBackoff = 30 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if err := a.Discover(ctx); err != nil {
				logger.Warn("OIDC discovery retry failed", "error", err, "next_retry", backoff)
				backoff = min(backoff*2, maxBackoff)
				continue
			}
			logger.Info("OIDC authentication enabled", "issuer", a.cfg.IssuerURL)
			return
		}
	}()
}

// Middleware authenticates requests with an "Authorization: Bearer" access token.
func (a *OIDCAuthenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := a.authenticate(c.Request.Context(), c.GetHeader("Authorization"))
		if err != nil {
			status, message := http.StatusInternalServerError, "authentication failed"
			switch {
			case errors.Is(err, errMissingAuthorization):
				status, message = http.StatusUnauthorized, err.Error()
			case errors.Is(err, errInvalidToken):
				status, message = http.StatusUnauthorized, errInvalidToken.Error()
			case errors.Is(err, errProviderUnavailable):
				status, message = http.StatusServiceUnavailable, err.Error()
			}
			if status == http.StatusInternalServerError {
				slog.Error("Authentication failed", "error", err)
			} else {
				slog.Debug("Request not authenticated", "error", err)
			}
			c.AbortWithStatusJSON(status, gin.H{"error": message})
			return
		}
		c.Set(UserContextKey, user)
		c.Next()
	}
}

func (a *OIDCAuthenticator) authenticate(ctx context.Context, header string) (*models.User, error) {
	scheme, raw, ok := strings.Cut(header, " ")
	if header == "" {
		return nil, errMissingAuthorization
	}
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return nil, fmt.Errorf("%w: malformed authorization header", errInvalidToken)
	}

	key := sha256.Sum256([]byte(raw))
	if userID, ok := a.cache.get(key, a.now()); ok {
		return a.loadUser(userID)
	}

	// Parallel requests with the same new token (e.g. the web UI's first
	// page load) reconcile once.
	v, err, _ := a.flight.Do(string(key[:]), func() (any, error) {
		if userID, ok := a.cache.get(key, a.now()); ok {
			return userID, nil
		}
		// Waiters share this call, so one caller's canceled request must not
		// fail it for the rest.
		userID, expiry, err := a.verifyAndReconcile(context.WithoutCancel(ctx), raw)
		if err != nil {
			return nil, err
		}
		now := a.now()
		until := now.Add(authorizationCacheTTL)
		if expiry.Before(until) {
			until = expiry
		}
		a.cache.put(key, userID, until, now)
		return userID, nil
	})
	if err != nil {
		return nil, err
	}
	return a.loadUser(v.(uuid.UUID))
}

func (a *OIDCAuthenticator) verifyAndReconcile(ctx context.Context, raw string) (uuid.UUID, time.Time, error) {
	verifier := a.verifier.Load()
	if verifier == nil {
		return uuid.Nil, time.Time{}, errProviderUnavailable
	}
	token, err := verifier.Verify(ctx, raw)
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("%w: %v", errInvalidToken, err)
	}
	var claims accessTokenClaims
	if err := token.Claims(&claims); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("%w: parse claims: %v", errInvalidToken, err)
	}

	user, err := findOrCreateFederatedUser(a.db, federatedUserClaims{
		Issuer:            token.Issuer,
		Subject:           token.Subject,
		PreferredUsername: claims.PreferredUsername,
		Email:             claims.Email,
		EmailVerified:     claims.EmailVerified,
		Name:              claims.Name,
		AvatarURL:         claims.Picture,
	})
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("resolve federated user: %w", err)
	}
	if err := syncOIDCGroups(a.db, user.ID, claims.Groups, a.rbac); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("sync groups for user %s: %w", user.ID, err)
	}
	if err := syncAdminRole(user.ID, shouldBeAdminFromGroups(claims.Groups, a.cfg.AdminGroups), a.rbac); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("sync admin role for user %s: %w", user.ID, err)
	}
	return user.ID, token.Expiry, nil
}

func (a *OIDCAuthenticator) loadUser(userID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := a.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("load user %s: %w", userID, err)
	}
	return &user, nil
}

func shouldBeAdminFromGroups(groups []string, adminGroups []string) bool {
	adminGroupSet := make(map[string]bool, len(adminGroups))
	for _, g := range adminGroups {
		if g = strings.TrimPrefix(strings.TrimSpace(g), "/"); g != "" {
			adminGroupSet[g] = true
		}
	}
	for _, g := range groups {
		// Keycloak's full-path group mapper adds a leading "/".
		if adminGroupSet[strings.TrimPrefix(g, "/")] {
			return true
		}
	}
	return false
}

// syncAdminRole grants or revokes the user's admin role to match the IdP groups.
func syncAdminRole(userID uuid.UUID, shouldBeAdmin bool, rbacProvider rbac.Provider) error {
	if rbacProvider == nil {
		return errors.New("rbac provider is not configured")
	}
	isAdmin, err := rbacProvider.IsAdmin(userID)
	if err != nil {
		return fmt.Errorf("check admin status: %w", err)
	}
	switch {
	case shouldBeAdmin && !isAdmin:
		if err := rbacProvider.MakeAdmin(userID); err != nil {
			return fmt.Errorf("grant admin from IdP groups: %w", err)
		}
		slog.Info("Granted admin via IdP group membership", "user_id", userID)
	case !shouldBeAdmin && isAdmin:
		if err := rbacProvider.RevokeAdmin(userID); err != nil {
			return fmt.Errorf("revoke admin from IdP groups: %w", err)
		}
		slog.Info("Revoked admin via IdP group membership", "user_id", userID)
	}
	return nil
}

type cachedToken struct {
	userID uuid.UUID
	until  time.Time
}

// tokenCache remembers tokens whose claims were applied recently, keyed by
// the SHA-256 of the raw token so raw tokens are never kept in memory.
type tokenCache struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]cachedToken
}

func (c *tokenCache) get(key [sha256.Size]byte, now time.Time) (uuid.UUID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !now.Before(e.until) {
		return uuid.Nil, false
	}
	return e.userID, true
}

func (c *tokenCache) put(key [sha256.Size]byte, userID uuid.UUID, until, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxCachedTokens {
		for k, e := range c.entries {
			if !now.Before(e.until) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= maxCachedTokens {
			clear(c.entries)
		}
	}
	c.entries[key] = cachedToken{userID: userID, until: until}
}
