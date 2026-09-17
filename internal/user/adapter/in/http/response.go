package httpadapter

import (
	"time"

	"go_pet_project/internal/user/application/query"
)

type GetMeResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AssignableUsersResponse struct {
	Items []GetMeResponse `json:"items"`
}

func newGetMeResponse(result query.GetMeResult) GetMeResponse {
	return GetMeResponse{
		ID:        result.ID.String(),
		Email:     result.Email,
		Username:  result.Username,
		Timezone:  result.Timezone,
		CreatedAt: result.CreatedAt,
		UpdatedAt: result.UpdatedAt,
	}
}

func newAssignableUsersResponse(result query.ListAssignableUsersResult) AssignableUsersResponse {
	items := make([]GetMeResponse, 0, len(result.Users))
	for _, user := range result.Users {
		items = append(items, newGetMeResponse(user))
	}

	return AssignableUsersResponse{Items: items}
}
