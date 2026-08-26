package domain

func Viability(germinated, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(germinated) * 100 / float64(total)
}
func (b *RejuvenationBatch) Accession(id string) (AccessionItem, bool) {
	for _, x := range b.Items {
		if x.AccessionID == id {
			return x, true
		}
	}
	return AccessionItem{}, false
}
