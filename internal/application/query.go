package application

import "seedvault/internal/domain"

func (a *App) List() []*domain.RejuvenationBatch { return a.Store.List() }
func (a *App) Events(id string) any              { return a.Store.Events(id) }
