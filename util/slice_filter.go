package util

// FilterSlice generic function for filtering a slice. Keeps elements where filterFn returns true.
func FilterSlice[T any](src []*T, filterFn func(*T) (bool, error)) ([]*T, error) {
	var res []*T
	for _, b := range src {
		ok, err := filterFn(b)
		if err != nil {
			return nil, err
		}
		if ok {
			res = append(res, b)
		}
	}
	return res, nil
}
