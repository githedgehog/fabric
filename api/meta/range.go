package meta

type Range[T comparable] interface {
	From() T
	To() T
}
