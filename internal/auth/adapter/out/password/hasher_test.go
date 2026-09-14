package password

import "testing"

func TestHasher(t *testing.T) {
	hasher, err := NewHasher(4)
	if err != nil {
		t.Fatalf("new hasher: %v", err)
	}

	hash, err := hasher.Hash("password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "password" {
		t.Fatalf("hash must not equal plaintext")
	}
	if err := hasher.Compare("password", hash); err != nil {
		t.Fatalf("compare correct password: %v", err)
	}
	if err := hasher.Compare("wrong-password", hash); err == nil {
		t.Fatalf("expected wrong password to fail")
	}

	emptyHash, err := hasher.Hash("")
	if err != nil {
		t.Fatalf("hash empty password: %v", err)
	}
	if emptyHash == "" {
		t.Fatalf("empty password hash must not be empty")
	}
	if err := hasher.Compare("", emptyHash); err != nil {
		t.Fatalf("compare empty password: %v", err)
	}
}
