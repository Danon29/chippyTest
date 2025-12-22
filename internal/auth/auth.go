package auth

import (
    "time"
	"strings"
	"errors"
    "net/http"
	
    "github.com/alexedwards/argon2id"
    "github.com/google/uuid"
    "github.com/golang-jwt/jwt/v5"
)

func HashPassword(password string) (string, error) {
    hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
    if err != nil {
        return "", err
    }
    return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
    match, err := argon2id.ComparePasswordAndHash(password, hash)
    if err != nil {
        return false, err
    }
    return match, nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
    now := time.Now().UTC()

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
        Issuer:    "chirpy",
        IssuedAt:  jwt.NewNumericDate(now), 
        ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)), 
        Subject:   userID.String(), 
    }) 

    signedToken, err := token.SignedString([]byte(tokenSecret))
    if err != nil {
        return "", err  
    }

    return signedToken, nil 
}

func ValidateJWT(tokenString string, tokenSecret string) (uuid.UUID, error) {
    token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, 
        func(token *jwt.Token) (interface{}, error) {
            return []byte(tokenSecret), nil
        })
    if err != nil {
        return uuid.Nil, err
    }
    
    if !token.Valid {
        return uuid.Nil, jwt.ErrTokenInvalidClaims
    }
    
    claims := token.Claims.(*jwt.RegisteredClaims)
    userID, err := uuid.Parse(claims.Subject)
    if err != nil {
        return uuid.Nil, err
    }
    
    return userID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("Authorization header is missing")
	}

	parts := strings.Fields(authHeader)
    if len(parts) != 2 || parts[0] != "Bearer" {
        return "", errors.New("Authorization header must be 'Bearer <token>'")
    }

    token := parts[1] 
    return token, nil 
}

