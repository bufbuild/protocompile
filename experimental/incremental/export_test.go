package incremental

// Exported symbols for test use only. Placing such symbols in a _test.go
// file avoids them being exported "for real".

// Abort forces an abort on the given task.
func Abort(t *Task, err error) { t.abort(err) }
