package query

import "github.com/google/uuid"

type ListAssignableUsersQuery struct {
	ActorID uuid.UUID
}

type ListAssignableUsersResult struct {
	Users []GetMeResult
}
