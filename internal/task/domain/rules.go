package domain

var allowedStatusTransitions = map[Status][]Status{
	StatusOpen: {
		StatusInProgress,
		StatusDone,
	},
	StatusInProgress: {
		StatusDone,
	},
}
