package domain

import "github.com/google/uuid"

// NewIDIfEmpty — общее тело BeforeCreate-хука для всех domain-моделей.
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
