package paniccheck

func Safe() {
	// no panic here — should not be reported
	_ = 1 + 1
}

func Unsafe() {
	panic("something went wrong") // want `use of built-in panic is forbidden`
}

func UnsafeWithValue(v interface{}) {
	panic(v) // want `use of built-in panic is forbidden`
}
