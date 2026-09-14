package postgres

import authout "go_pet_project/internal/auth/application/port/out"

var _ authout.UserRepository = (*Repository)(nil)
