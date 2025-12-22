package auth

import (
    "testing"
    "time"
    
    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
)

func TestMakeJWT_ValidateJWT(t *testing.T) {
    secret := "test-secret"
    userID := uuid.New()
    expiresIn := time.Minute
    
    token, err := MakeJWT(userID, secret, expiresIn)
    assert.NoError(t, err)
    assert.NotEmpty(t, token)
    
    validatedID, err := ValidateJWT(token, secret)
    assert.NoError(t, err)
    assert.Equal(t, userID, validatedID)
}

func TestValidateJWT_InvalidToken(t *testing.T) {
    _, err := ValidateJWT("invalid.token", "secret")
    assert.Error(t, err)
}
