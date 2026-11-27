package auth_test

import (
	"chatnip/internal/kit/auth"
	"chatnip/internal/kit/database"
	"context"
	"os"
	"testing"
)

func TestAPIKeyLifecycle(t *testing.T) {
	dbPath := "/tmp/test_apikeys.db"
	_ = os.Remove(dbPath)
	defer os.Remove(dbPath)

	db, err := database.NewSQLiteConnection(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite connection: %v", err)
	}
	defer db.Close()

	store := auth.NewSQLStore(db)
	ctx := context.Background()

	rawKey, key, err := store.Create(ctx, "ERP Integration")
	if err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}
	if rawKey == "" {
		t.Fatal("expected non-empty raw key")
	}
	if key.Name != "ERP Integration" {
		t.Fatalf("expected name to be 'ERP Integration', got %s", key.Name)
	}
	if key.MaskedKey == "" || len(key.MaskedKey) < 10 {
		t.Fatalf("expected valid masked key, got %s", key.MaskedKey)
	}

	valid, err := store.Validate(ctx, rawKey)
	if err != nil {
		t.Fatalf("unexpected error validating key: %v", err)
	}
	if !valid {
		t.Fatal("expected rawKey to be valid")
	}

	invalid, err := store.Validate(ctx, "cn_live_invalidkey123")
	if err != nil {
		t.Fatalf("unexpected error validating invalid key: %v", err)
	}
	if invalid {
		t.Fatal("expected invalid key to return false")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list keys: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 key in list, got %d", len(list))
	}
	if list[0].MaskedKey != key.MaskedKey {
		t.Fatalf("expected masked key in list %s, got %s", key.MaskedKey, list[0].MaskedKey)
	}
	if list[0].RevokedAt != nil {
		t.Fatal("expected key to not be revoked")
	}

	if err := store.Revoke(ctx, key.ID); err != nil {
		t.Fatalf("failed to revoke key: %v", err)
	}

	validAfterRevoke, err := store.Validate(ctx, rawKey)
	if err != nil {
		t.Fatalf("unexpected error validating revoked key: %v", err)
	}
	if validAfterRevoke {
		t.Fatal("expected revoked key to not be valid")
	}
}
