package audit

func FirstBroken(events []Event) int {
	prev := ""
	for i, e := range events {
		if e.Revision != i+1 || HashEvent(prev, e) != e.Hash {
			return i + 1
		}
		prev = e.Hash
	}
	return 0
}
