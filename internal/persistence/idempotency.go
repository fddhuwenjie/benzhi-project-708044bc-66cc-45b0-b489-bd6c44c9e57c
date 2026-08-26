package persistence

func (s *Store) Remember(request string, response any) bool {
	if request == "" {
		return false
	}
	if _, ok := s.Idem(request); ok {
		return true
	}
	s.PutIdem(request, response)
	return false
}
