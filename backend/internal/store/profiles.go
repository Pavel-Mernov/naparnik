package store

// Профиль сотрудника: что можно поменять в Напарнике поверх оргструктуры. ФИО, должность, отдел и почта —
// из оргструктуры (в Авито — HR-система), здесь не правятся. Телефон и фото — правит сам сотрудник.

// Photo — фото профиля. Хранится в состоянии, поэтому размер ограничен (MaxPhotoBytes в tools).
type Photo struct {
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
	UpdatedAt   string `json:"updated_at"`
}

type ProfileEdit struct {
	Phone     string `json:"phone,omitempty"`
	Photo     *Photo `json:"photo,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *Store) Profile(userID string) (ProfileEdit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.profiles[userID]
	if ok && p.Photo != nil {
		cp := *p.Photo
		cp.Data = append([]byte(nil), p.Photo.Data...)
		p.Photo = &cp
	}
	return p, ok
}

func (s *Store) UpdateProfile(userID string, fn func(*ProfileEdit)) ProfileEdit {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.profiles[userID]
	fn(&p)
	s.profiles[userID] = p
	return p
}
