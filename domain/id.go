package domain

import "github.com/google/uuid"

// NewIDIfEmpty assigns a fresh UUIDv7 to *id if it's still the zero value —
// the shared body of every domain model's GORM BeforeCreate hook.
func NewIDIfEmpty(id *uuid.UUID) error {
	if *id != uuid.Nil {
		return nil
	}
	newID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	*id = newID
	return nil
}
