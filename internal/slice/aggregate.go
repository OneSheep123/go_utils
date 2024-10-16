package slice

// Sum 求和
// 在使用 float32 或者 float64 的时候要小心精度问题
func Sum[T Number](ts []T) T {
	var res T
	for _, n := range ts {
		res += n
	}
	return res
}
