package pointer

func To[T any](val T) *T {
	return &val
}
