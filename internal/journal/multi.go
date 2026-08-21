package journal

// NewMulti records to each sink in order. Nil sinks are skipped.
// Empty / all-nil returns nil (Finish is already a no-op on nil).
func NewMulti(sinks ...Journal) Journal {
	var out []Journal
	for _, s := range sinks {
		if s != nil {
			out = append(out, s)
		}
	}
	switch len(out) {
	case 0:
		return nil
	case 1:
		return out[0]
	default:
		return multi(out)
	}
}

type multi []Journal

func (m multi) Record(event Event) error {
	var first error
	for _, s := range m {
		if err := s.Record(event); err != nil && first == nil {
			first = err
		}
	}
	return first
}
