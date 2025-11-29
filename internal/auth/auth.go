package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type AuthContext struct {
	UserID      string
	OrgID       *string
	Roles       []string
	Permissions []string
	Email       *string
	Name        *string
}

type Config struct {
	JWKSUrl      string
	Issuer       string
	Audience     string
	JWKSCacheTTL int
}

type cachedJWKS struct {
	set       jwk.Set
	expiresAt time.Time
}

type JWKSClient struct {
	url        string
	cache      *cachedJWKS
	cacheTTL   time.Duration
	mu         sync.RWMutex
	httpClient *http.Client
}

func NewJWKSClient(url string, cacheTTLSeconds int) *JWKSClient {
	ttl := time.Duration(cacheTTLSeconds) * time.Second
	if ttl == 0 {
		ttl = 15 * time.Minute
	}

	return &JWKSClient{
		url:        url,
		cacheTTL:   ttl,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *JWKSClient) GetKeySet(ctx context.Context) (jwk.Set, error) {
	c.mu.RLock()
	if c.cache != nil && time.Now().Before(c.cache.expiresAt) {
		set := c.cache.set
		c.mu.RUnlock()
		return set, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cache != nil && time.Now().Before(c.cache.expiresAt) {
		return c.cache.set, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.cache != nil {
			return c.cache.set, nil
		}
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.cache != nil {
			return c.cache.set, nil
		}
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	set, err := jwk.ParseReader(resp.Body, jwk.WithPEM(true))
	if err != nil {
		if c.cache != nil {
			return c.cache.set, nil
		}
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	c.cache = &cachedJWKS{
		set:       set,
		expiresAt: time.Now().Add(c.cacheTTL),
	}

	return set, nil
}

func VerifyToken(ctx context.Context, tokenString string, jwksClient *JWKSClient, config Config) (*AuthContext, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode token header: %w", err)
	}

	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse token header: %w", err)
	}

	kid, ok := header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing kid in header")
	}

	keySet, err := jwksClient.GetKeySet(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get JWKS: %w", err)
	}

	key, found := keySet.LookupKeyID(kid)
	if !found {
		return nil, fmt.Errorf("key not found for kid: %s", kid)
	}

	var publicKey interface{}
	if err := key.Raw(&publicKey); err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	verifiedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	claims, ok := verifiedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	if iss, ok := claims["iss"].(string); !ok || iss != config.Issuer {
		return nil, fmt.Errorf("invalid issuer")
	}

	if aud, ok := claims["aud"].(string); ok && aud != config.Audience {
		return nil, fmt.Errorf("invalid audience")
	} else if audArr, ok := claims["aud"].([]interface{}); ok {
		found := false
		for _, a := range audArr {
			if aStr, ok := a.(string); ok && aStr == config.Audience {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid audience")
		}
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return nil, fmt.Errorf("token missing sub claim")
	}

	var orgIDPtr *string
	if orgID, ok := claims["org_id"].(string); ok && orgID != "" {
		orgIDPtr = &orgID
	}

	var rolesList []string
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, r := range roles {
			if rStr, ok := r.(string); ok {
				rolesList = append(rolesList, rStr)
			}
		}
	}

	var permissionsList []string
	if permissions, ok := claims["permissions"].([]interface{}); ok {
		for _, p := range permissions {
			if pStr, ok := p.(string); ok {
				permissionsList = append(permissionsList, pStr)
			}
		}
	}

	var emailPtr *string
	if email, ok := claims["email"].(string); ok && email != "" {
		emailPtr = &email
	}

	var namePtr *string
	if name, ok := claims["name"].(string); ok && name != "" {
		namePtr = &name
	}

	return &AuthContext{
		UserID:      sub,
		OrgID:       orgIDPtr,
		Roles:       rolesList,
		Permissions: permissionsList,
		Email:       emailPtr,
		Name:        namePtr,
	}, nil
}

func AuthMiddleware(jwksClient *JWKSClient, config Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid authorization header"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		authContext, err := VerifyToken(c.Request.Context(), token, jwksClient, config)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token", "details": err.Error()})
			c.Abort()
			return
		}

		c.Set("auth", authContext)
		c.Next()
	}
}

func RequirePermissions(requiredPermissions []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authContext, exists := c.Get("auth")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
			c.Abort()
			return
		}

		ctx, ok := authContext.(*AuthContext)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid auth context"})
			c.Abort()
			return
		}

		hasAll := true
		for _, required := range requiredPermissions {
			found := false
			for _, perm := range ctx.Permissions {
				if perm == required {
					found = true
					break
				}
			}
			if !found {
				hasAll = false
				break
			}
		}

		if !hasAll {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient permissions",
				"required": requiredPermissions,
				"has":      ctx.Permissions,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func GetAuthContext(c *gin.Context) (*AuthContext, bool) {
	authContext, exists := c.Get("auth")
	if !exists {
		return nil, false
	}

	ctx, ok := authContext.(*AuthContext)
	return ctx, ok
}
