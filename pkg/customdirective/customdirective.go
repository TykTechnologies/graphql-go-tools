package customdirective

type CustomDirective interface {
	Name() string
	DataType() string
	Execute([]byte) ([]byte, error)
}
