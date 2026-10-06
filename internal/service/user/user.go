// Package user holds the user domain type, its state machine, and the
// sqlc-backed repository. Behavior mirrors reference/app/Models/User.php
// and reference/app/Enums/UserState.php.
package user

import "time"

// User is the domain representation of a registered user.
type User struct {
	ID                     int64
	Name                   string
	Email                  string
	Phone                  *string
	PasswordHash           string
	State                  UserState
	EmailVerifiedAt        *time.Time
	TwoFactorSecret        *string
	TwoFactorRecoveryCodes *string
	TwoFactorConfirmedAt   *time.Time
	RememberToken          *string
}

// UserState is a member state. Mirrors the PHP backed enum value for value.
type UserState string

const (
	StateUnverified UserState = "unverified"
	StatePending    UserState = "pending"
	StateActive     UserState = "active"
	StateSuspended  UserState = "suspended"
	StateRejected   UserState = "rejected"
)

// Transitions returns the states this state may move to. Exact mirror of
// UserState::transitions() in the reference.
func (s UserState) Transitions() []UserState {
	switch s {
	case StateUnverified:
		return nil
	case StatePending:
		return []UserState{StateActive, StateRejected}
	case StateActive:
		return []UserState{StateSuspended}
	case StateSuspended:
		return []UserState{StateActive}
	case StateRejected:
		return []UserState{StatePending}
	default:
		return nil
	}
}

// CanTransitionTo reports whether target is an allowed transition.
func (s UserState) CanTransitionTo(target UserState) bool {
	for _, t := range s.Transitions() {
		if t == target {
			return true
		}
	}
	return false
}
