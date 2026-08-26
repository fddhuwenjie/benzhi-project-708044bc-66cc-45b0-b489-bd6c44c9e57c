package application

import "errors"

var ErrArchived = errors.New("archived_read_only")

func code(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
