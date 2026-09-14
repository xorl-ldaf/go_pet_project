package out

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(password string, encodedHash string) error
}
