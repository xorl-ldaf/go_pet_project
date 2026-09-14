package postgres

import "go_pet_project/internal/user/domain"

func toModel(user domain.User) userModel {
	return userModel{
		ID:           user.ID,
		Email:        user.Email,
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Timezone:     user.Timezone,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

func toDomain(model userModel) domain.User {
	return domain.User{
		ID:           model.ID,
		Email:        model.Email,
		Username:     model.Username,
		PasswordHash: model.PasswordHash,
		Timezone:     model.Timezone,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
