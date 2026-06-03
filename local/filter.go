package local

type RouteAction int

const (
	RouteProxy RouteAction = iota
	RouteDirect
	RouteBlock
)

type Filter interface {
	Filter(target string) RouteAction
}
