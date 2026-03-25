package infra

import "slices"

// SortByOrderThenID applies the shared canonical ordering rule used across modules.
func SortByOrderThenID[T any](items []T, orderOf func(T) int, idOf func(T) int64) {
	slices.SortStableFunc(items, func(left, right T) int {
		if leftOrder, rightOrder := orderOf(left), orderOf(right); leftOrder != rightOrder {
			return leftOrder - rightOrder
		}

		leftID := idOf(left)
		rightID := idOf(right)
		switch {
		case leftID < rightID:
			return -1
		case leftID > rightID:
			return 1
		default:
			return 0
		}
	})
}
