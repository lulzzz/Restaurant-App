package oidc

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/patrickmn/go-cache"
)

// TokenVerifier provides for token validation
type TokenVerifier interface {
	ValidateToken(bearerToken string) (bool, error)
}

// JwtTokenVerifier provides oidc server information
type JwtTokenVerifier struct {
	HTTPClient       jwk.HTTPClient
	Cache            *cache.Cache
	Authority        string
	ClaimsToValidate map[string]interface{}
}

// ValidateToken validates claims with given token
func (j *JwtTokenVerifier) ValidateToken(bearerToken string) (bool, error) {

	token, err := jwt.Parse(bearerToken, func(token *jwt.Token) (interface{}, error) {
		if err, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			fmt.Println(err)
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		set, err := j.fetchAndCacheJWKS()
		if err != nil {
			return nil, err
		}

		keyID, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("expecting JWT header to have string kid")
		}
		if key := set.LookupKeyID(keyID); len(key) == 1 {
			return key[0].Materialize()
		}

		return nil, errors.New("unable to find key")
	})

	if err != nil {
		return false, err
	}

	return j.validateTokenByClaims(token)
}

func (j *JwtTokenVerifier) validateTokenByClaims(token *jwt.Token) (bool, error) {
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		for k, v := range j.ClaimsToValidate {
			claim := claims[k]
			switch reflect.TypeOf(claim).Kind() {
			case reflect.String:
				if claim != v {
					return false, fmt.Errorf("claims validate failed, invalid claim: %v", k)
				}
			case reflect.Slice:
				var itemFound bool
				for _, c := range claim.([]interface{}) {
					if c == v {
						itemFound = true
						break
					}
				}
				if !itemFound {
					return false, fmt.Errorf("claims validate failed, invalid claim: %v", k)
				}
			}
		}
		return true, nil
	}

	return false, fmt.Errorf("invalid token")
}

func (j *JwtTokenVerifier) fetchAndCacheJWKS() (*jwk.Set, error) {
	var result *jwk.Set
	if cachedSet, found := j.Cache.Get(j.getJwkURL()); found {
		if set, ok := cachedSet.(*jwk.Set); !ok {
			result = set
		} else {
			return nil, fmt.Errorf("cannot convert the type %v into %v", cachedSet, reflect.TypeOf(cachedSet))
		}
	} else {
		if response, err := jwk.FetchHTTP(j.getJwkURL(), jwk.WithHTTPClient(j.HTTPClient)); err != nil {
			return nil, err
		} else {
			result = response
			j.Cache.Set(j.getJwkURL(), &response, cache.NoExpiration)
		}
	}
	return result, nil
}

func (j *JwtTokenVerifier) getJwkURL() string {
	if !strings.HasSuffix(j.Authority, "/") {
		j.Authority += "/"
	}
	return j.Authority + ".well-known/openid-configuration/jwks"
}
