package command

type RegisterCommand struct {
	Email    string
	Username string
	Password string
	Timezone string
}

type RegisterResult struct {
	User UserResult
}
