package checkers

type Checker interface {
	Enabled() bool
	Start()
}
