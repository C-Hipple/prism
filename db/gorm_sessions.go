package db

import (
	"gorm.io/gorm"
)

// sessionModelToSession converts a SessionModel to a Session interface type
func sessionModelToSession(m *SessionModel) *Session {
	if m == nil {
		return nil
	}

	return &Session{
		ID:        m.ID,
		UserID:    int(m.UserID),
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}
}

// sessionToSessionModel converts a Session interface type to a SessionModel
func sessionToSessionModel(s *Session) *SessionModel {
	if s == nil {
		return nil
	}

	return &SessionModel{
		ID:        s.ID,
		UserID:    uint(s.UserID),
		ExpiresAt: s.ExpiresAt,
		CreatedAt: s.CreatedAt,
	}
}

// CreateSession creates a new session
func (g *GormDB) CreateSession(session *Session) error {
	model := sessionToSessionModel(session)
	if err := g.db.Create(model).Error; err != nil {
		return err
	}
	// Update the session struct with the generated CreatedAt
	session.CreatedAt = model.CreatedAt
	return nil
}

// GetSession retrieves a session by ID
func (g *GormDB) GetSession(id string) (*Session, error) {
	var model SessionModel
	result := g.db.Where("id = ?", id).First(&model)

	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}

	return sessionModelToSession(&model), nil
}

// DeleteSession removes a session by ID
func (g *GormDB) DeleteSession(id string) error {
	return g.db.Where("id = ?", id).Delete(&SessionModel{}).Error
}
