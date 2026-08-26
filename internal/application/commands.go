package application

import "seedvault/internal/domain"

func (a *App) State(id string) (domain.State, error) {
	b, e := a.Get(id)
	if e != nil {
		return "", e
	}
	return b.State, nil
}
