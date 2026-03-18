package services

import "os"

func (s *SingBoxService) PruneBackups(keep int) {
	if keep <= 0 {
		keep = 20
	}
	items, err := s.ListBackups()
	if err != nil || len(items) <= keep {
		return
	}
	for _, item := range items[keep:] {
		_ = os.Remove(item.Path)
	}
}
