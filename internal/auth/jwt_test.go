package auth_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marcelblijleven/bifrost/internal/auth"
)

const testSecret = "super-secret-test-key"

func TestGenerateAndValidateToken(t *testing.T) {
	userID := uuid.New()
	email := "test@example.com"

	token, err := auth.GenerateToken(userID, email, false, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ValidateToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
	if claims.Email != email {
		t.Errorf("Email = %q, want %q", claims.Email, email)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := auth.GenerateToken(uuid.New(), "user@example.com", false, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = auth.ValidateToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	token, err := auth.GenerateToken(uuid.New(), "user@example.com", false, testSecret, -time.Second)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	_, err = auth.ValidateToken(token, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := auth.ValidateToken("not.a.valid.jwt.at.all", testSecret)
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

func TestValidateToken_EmptyString(t *testing.T) {
	_, err := auth.ValidateToken("", testSecret)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

func TestWithClaimsAndClaimsFromContext(t *testing.T) {
	userID := uuid.New()
	want := &auth.Claims{UserID: userID, Email: "ctx@example.com"}

	ctx := auth.WithClaims(t.Context(), want)

	got, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		t.Fatal("ClaimsFromContext: expected ok=true")
	}
	if got.UserID != want.UserID {
		t.Errorf("UserID = %v, want %v", got.UserID, want.UserID)
	}
}

func TestClaimsFromContext_Missing(t *testing.T) {
	_, ok := auth.ClaimsFromContext(t.Context())
	if ok {
		t.Fatal("expected ok=false when no claims in context")
	}
}
