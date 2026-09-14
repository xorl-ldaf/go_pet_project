package out

type RefreshTokenGenerator interface {
	Generate() (string, error)
	Hash(token string) string
}
